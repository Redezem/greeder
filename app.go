package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type SummaryStatus string

const (
	SummaryNotGenerated SummaryStatus = "not_generated"
	SummaryGenerating   SummaryStatus = "generating"
	SummaryGenerated    SummaryStatus = "generated"
	SummaryFailed       SummaryStatus = "failed"
	SummaryNoConfig     SummaryStatus = "no_config"
)

type FilterMode string

const (
	FilterAll     FilterMode = "all"
	FilterUnread  FilterMode = "unread"
	FilterStarred FilterMode = "starred"
)

type App struct {
	config        Config
	store         *Store
	fetcher       *FeedFetcher
	summarizer    *Summarizer
	raindrop      *RaindropClient
	feeds         []Feed
	articles      []Article
	current       Summary
	summaryStatus SummaryStatus
	selectedIndex int
	filter        FilterMode
	status        string
	lastDeleted   *Article
	openURL       func(string) error
	emailSender   func(string) error
}

func NewApp(cfg Config) (*App, error) {
	store, err := NewStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	app := &App{
		config:        cfg,
		store:         store,
		fetcher:       NewFeedFetcher(),
		summarizer:    NewSummarizerFromEnv(),
		raindrop:      NewRaindropClient(cfg.RaindropToken),
		feeds:         store.Feeds(),
		articles:      store.SortedArticles(),
		summaryStatus: SummaryNotGenerated,
		filter:        FilterUnread,
		openURL:       defaultOpenURL,
		emailSender:   defaultSendEmail,
	}
	app.store.DeleteOldArticles(7)
	app.articles = app.store.SortedArticles()
	app.status = fmt.Sprintf("%d feeds loaded", len(app.feeds))
	return app, nil
}

func (a *App) SelectedArticle() *Article {
	articles := a.FilteredArticles()
	if len(articles) == 0 || a.selectedIndex < 0 || a.selectedIndex >= len(articles) {
		return nil
	}
	article := articles[a.selectedIndex]
	return &article
}

func (a *App) FilteredArticles() []Article {
	if a.filter == FilterAll {
		return a.articles
	}
	filtered := make([]Article, 0, len(a.articles))
	for _, article := range a.articles {
		switch a.filter {
		case FilterUnread:
			if !article.IsRead {
				filtered = append(filtered, article)
			}
		case FilterStarred:
			if article.IsStarred {
				filtered = append(filtered, article)
			}
		}
	}
	return filtered
}

func (a *App) MoveSelection(delta int) {
	articles := a.FilteredArticles()
	if len(articles) == 0 {
		a.selectedIndex = 0
		return
	}
	idx := a.selectedIndex + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(articles) {
		idx = len(articles) - 1
	}
	a.selectedIndex = idx
	a.syncSummaryForSelection()
}

func (a *App) ToggleFilter() {
	switch a.filter {
	case FilterUnread:
		a.filter = FilterStarred
	case FilterStarred:
		a.filter = FilterAll
	default:
		a.filter = FilterUnread
	}
	a.selectedIndex = 0
	a.syncSummaryForSelection()
}

func (a *App) RefreshFeeds() error {
	if len(a.feeds) == 0 {
		a.status = "no feeds to refresh"
		return nil
	}
	for _, feed := range a.feeds {
		parsed, err := a.fetcher.FetchFeed(feed.URL)
		if err != nil {
			a.status = fmt.Sprintf("refresh failed: %v", err)
			continue
		}
		_, _ = a.store.InsertArticles(feed, parsed.Articles)
	}
	a.feeds = a.store.Feeds()
	a.articles = a.store.SortedArticles()
	a.status = fmt.Sprintf("refreshed %d feeds", len(a.feeds))
	return nil
}

func (a *App) AddFeed(input string) error {
	input = strings.TrimSpace(input)
	if input == "" {
		return errors.New("empty feed url")
	}
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	parsed, err := a.fetcher.DiscoverFeed(input)
	if err != nil {
		return err
	}
	feed := Feed{
		Title:       parsed.Title,
		URL:         parsed.URL,
		SiteURL:     parsed.SiteURL,
		Description: parsed.Description,
	}
	if _, err := a.store.InsertFeed(feed); err != nil {
		return err
	}
	a.feeds = a.store.Feeds()
	_, _ = a.store.InsertArticles(a.feeds[len(a.feeds)-1], parsed.Articles)
	a.articles = a.store.SortedArticles()
	a.status = "feed added"
	return nil
}

func (a *App) GenerateSummary() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	if a.summarizer == nil {
		a.summaryStatus = SummaryNoConfig
		return nil
	}
	if existing, ok := a.store.FindSummary(article.ID); ok {
		a.current = existing
		a.summaryStatus = SummaryGenerated
		return nil
	}
	a.summaryStatus = SummaryGenerating
	summaryText, model, err := a.summarizer.GenerateSummary(article.Title, firstNonEmpty(article.ContentText, article.Content))
	if err != nil {
		a.summaryStatus = SummaryFailed
		return err
	}
	summary := Summary{
		ArticleID:   article.ID,
		Content:     summaryText,
		Model:       model,
		GeneratedAt: time.Now().UTC(),
	}
	stored, err := a.store.UpsertSummary(summary)
	if err != nil {
		return err
	}
	a.current = stored
	a.summaryStatus = SummaryGenerated
	return nil
}

func (a *App) ToggleRead() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	article.IsRead = !article.IsRead
	if err := a.store.UpdateArticle(*article); err != nil {
		return err
	}
	a.updateArticleInList(*article)
	return nil
}

func (a *App) ToggleStar() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	article.IsStarred = !article.IsStarred
	if err := a.store.UpdateArticle(*article); err != nil {
		return err
	}
	a.updateArticleInList(*article)
	return nil
}

func (a *App) DeleteSelected() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	deleted, err := a.store.DeleteArticle(article.ID)
	if err != nil {
		return err
	}
	a.lastDeleted = &deleted
	a.articles = a.store.SortedArticles()
	if a.selectedIndex >= len(a.FilteredArticles()) {
		a.selectedIndex = len(a.FilteredArticles()) - 1
		if a.selectedIndex < 0 {
			a.selectedIndex = 0
		}
	}
	a.status = "article deleted"
	return nil
}

func (a *App) Undelete() error {
	_, err := a.store.UndeleteLast()
	if err != nil {
		a.status = "nothing to undelete"
		return nil
	}
	a.articles = a.store.SortedArticles()
	a.status = "article restored"
	return nil
}

func (a *App) OpenSelected() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	return a.openURL(article.URL)
}

func (a *App) EmailSelected() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	mailURL := buildMailto(article, a.current)
	return a.emailSender(mailURL)
}

func (a *App) SaveToRaindrop(tags []string) error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	if a.raindrop == nil {
		return errors.New("raindrop not configured")
	}
	summary := ""
	if a.current.ArticleID == article.ID {
		summary = a.current.Content
	}
	payload := RaindropItem{
		Link:  article.URL,
		Title: article.Title,
		Tags:  tags,
		Note:  summary,
	}
	raindropID, err := a.raindrop.Save(payload)
	if err != nil {
		return err
	}
	return a.store.SaveToRaindrop(article.ID, raindropID, tags)
}

func (a *App) CopySelectedURL() error {
	article := a.SelectedArticle()
	if article == nil {
		return nil
	}
	if err := copyToClipboard(article.URL); err != nil {
		return err
	}
	a.status = "URL copied to clipboard"
	return nil
}

func (a *App) GenerateMissingSummaries() error {
	if a.summarizer == nil {
		a.status = "Summarizer not configured"
		return errors.New("summarizer not configured")
	}
	existing := map[int]bool{}
	for _, summary := range a.store.Summaries() {
		existing[summary.ArticleID] = true
	}
	for _, article := range a.articles {
		if existing[article.ID] {
			continue
		}
		summaryText, model, err := a.summarizer.GenerateSummary(article.Title, firstNonEmpty(article.ContentText, article.Content))
		if err != nil {
			a.status = "Batch summary failed: " + err.Error()
			return err
		}
		summary := Summary{
			ArticleID:   article.ID,
			Content:     summaryText,
			Model:       model,
			GeneratedAt: time.Now().UTC(),
		}
		if _, err := a.store.UpsertSummary(summary); err != nil {
			return err
		}
	}
	a.status = "Batch summaries complete"
	a.syncSummaryForSelection()
	return nil
}

func (a *App) syncSummaryForSelection() {
	article := a.SelectedArticle()
	if article == nil {
		a.current = Summary{}
		a.summaryStatus = SummaryNotGenerated
		return
	}
	if summary, ok := a.store.FindSummary(article.ID); ok {
		a.current = summary
		a.summaryStatus = SummaryGenerated
		return
	}
	a.current = Summary{}
	a.summaryStatus = SummaryNotGenerated
}

func (a *App) updateArticleInList(article Article) {
	for i := range a.articles {
		if a.articles[i].ID == article.ID {
			a.articles[i] = article
			return
		}
	}
}

func (a *App) ImportOPML(path string) error {
	feeds, err := ParseOPML(path)
	if err != nil {
		return err
	}
	for _, feed := range feeds {
		if _, err := a.store.InsertFeed(feed); err != nil {
			continue
		}
	}
	a.feeds = a.store.Feeds()
	return a.RefreshFeeds()
}

func (a *App) ExportOPML(path string) error {
	return ExportOPML(path, a.feeds)
}

func buildMailto(article *Article, summary Summary) string {
	params := url.Values{}
	params.Set("subject", article.Title)
	body := []string{"Title: " + article.Title, "", "URL: " + article.URL}
	if summary.ArticleID == article.ID && summary.Content != "" {
		body = append(body, "", "AI Summary:", summary.Content)
	}
	if article.ContentText != "" {
		body = append(body, "", "Article Content:", article.ContentText)
	}
	params.Set("body", strings.Join(body, "\n"))
	return "mailto:?" + params.Encode()
}
