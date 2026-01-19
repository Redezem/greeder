package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppFiltersAndRefreshErrors(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}

	app.articles = []Article{{ID: 1, Title: "A", IsRead: false}, {ID: 2, Title: "B", IsRead: true, IsStarred: true}}
	app.filter = FilterUnread
	if got := len(app.FilteredArticles()); got != 1 {
		t.Fatalf("expected unread filter")
	}
	app.ToggleFilter()
	if got := len(app.FilteredArticles()); got != 1 {
		t.Fatalf("expected starred filter")
	}
	app.ToggleFilter()
	if got := len(app.FilteredArticles()); got != 2 {
		t.Fatalf("expected all filter")
	}
	app.ToggleFilter()
	if app.filter != FilterUnread {
		t.Fatalf("expected unread filter reset")
	}

	feed, err := app.store.InsertFeed(Feed{Title: "Bad", URL: "http://example.test/bad"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	app.fetcher = &FeedFetcher{client: clientForResponse(http.StatusInternalServerError, "", nil)}
	app.feeds = []Feed{{ID: feed.ID, Title: feed.Title, URL: "http://example.test/bad"}}
	if err := app.RefreshFeeds(); err != nil {
		t.Fatalf("RefreshFeeds error: %v", err)
	}
	if !strings.Contains(app.status, "refreshed") {
		t.Fatalf("expected refreshed status")
	}
}

func TestAppSelectionClearsSummary(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{
		{GUID: "1", Title: "One", URL: "u1"},
		{GUID: "2", Title: "Two", URL: "u2"},
	})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	_, err = app.store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "Summary"})
	if err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.syncSummaryForSelection()
	if app.summaryStatus != SummaryGenerated {
		t.Fatalf("expected summary generated")
	}
	app.MoveSelection(1)
	if app.summaryStatus != SummaryNotGenerated || app.current.Content != "" {
		t.Fatalf("expected summary cleared")
	}
	app.MoveSelection(-1)
	if app.summaryStatus != SummaryGenerated {
		t.Fatalf("expected summary restored")
	}
}

func TestAppGenerateSummaryExistingAndError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}

	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "https://example.com/1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	storedSummary, err := app.store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "Done", Model: "m", GeneratedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.summarizer = &Summarizer{baseURL: "http://example.com", model: "m", client: http.DefaultClient}
	if err := app.GenerateSummary(); err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
	if app.current.Content != storedSummary.Content || app.summaryStatus != SummaryGenerated {
		t.Fatalf("expected cached summary")
	}

	app.summarizer = &Summarizer{baseURL: "http://example.test/v1", model: "m", client: clientForResponse(http.StatusBadRequest, "", nil)}
	newArticles, err := app.store.InsertArticles(feed, []Article{{GUID: "2", Title: "Next", URL: "https://example.com/2"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = append(app.articles, newArticles...)
	app.selectedIndex = len(app.FilteredArticles()) - 1
	app.current = Summary{}
	if err := app.GenerateSummary(); err == nil {
		t.Fatalf("expected summary error")
	}
	if app.summaryStatus != SummaryFailed {
		t.Fatalf("expected summary failed status")
	}
}

func TestAppAddFeedDuplicateAndImportExport(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}

	app.fetcher = &FeedFetcher{client: clientForResponse(http.StatusOK, rssSample, map[string]string{"content-type": "application/rss+xml"})}

	if err := app.AddFeed("http://example.test/rss"); err != nil {
		t.Fatalf("AddFeed error: %v", err)
	}
	if err := app.AddFeed("http://example.test/rss"); err == nil {
		t.Fatalf("expected duplicate add error")
	}
	if err := app.AddFeed("example.com"); err != nil {
		t.Fatalf("expected scheme default success: %v", err)
	}

	opmlPath := filepath.Join(root, "feeds.opml")
	if err := ExportOPML(opmlPath, app.feeds); err != nil {
		t.Fatalf("ExportOPML error: %v", err)
	}
	app.feeds = nil
	if err := app.ImportOPML(opmlPath); err != nil {
		t.Fatalf("ImportOPML error: %v", err)
	}

	exportPath := filepath.Join(root, "out.opml")
	if err := app.ExportOPML(exportPath); err != nil {
		t.Fatalf("ExportOPML error: %v", err)
	}
}

func TestAppSaveToRaindropWithoutSummary(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "g1", Title: "T", URL: "https://example.com"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0

	app.raindrop = &RaindropClient{
		baseURL: "http://example.test",
		token:   "token",
		client:  clientForResponse(http.StatusOK, `{"item":{"_id":5}}`, map[string]string{"content-type": "application/json"}),
	}

	if err := app.SaveToRaindrop([]string{"t"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
	if app.SelectedArticle().ID != articles[0].ID {
		t.Fatalf("expected article selection")
	}
}

func TestAppToggleReadStarNoArticle(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if err := app.ToggleRead(); err != nil {
		t.Fatalf("ToggleRead error: %v", err)
	}
	if err := app.ToggleStar(); err != nil {
		t.Fatalf("ToggleStar error: %v", err)
	}
}

func TestAppToggleReadStarStoreError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 99, Title: "Ghost"}}
	app.selectedIndex = 0
	if err := app.ToggleRead(); err == nil {
		t.Fatalf("expected toggle read error")
	}
	if err := app.ToggleStar(); err == nil {
		t.Fatalf("expected toggle star error")
	}
}

func TestAppSyncSummaryPending(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.summaryPending[articles[0].ID] = true
	app.syncSummaryForSelection()
	if app.summaryStatus != SummaryGenerating {
		t.Fatalf("expected generating status")
	}
}

func TestAppDeleteSelectionAdjust(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{
		{GUID: "1", Title: "One", URL: "u1"},
		{GUID: "2", Title: "Two", URL: "u2"},
	})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = len(app.articles) - 1
	if err := app.DeleteSelected(); err != nil {
		t.Fatalf("DeleteSelected error: %v", err)
	}
	if app.selectedIndex != len(app.FilteredArticles())-1 {
		t.Fatalf("expected selection adjustment")
	}
}

func TestAppDeleteSelectionClamp(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Only", URL: "u1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	if err := app.DeleteSelected(); err != nil {
		t.Fatalf("DeleteSelected error: %v", err)
	}
	if app.selectedIndex != 0 {
		t.Fatalf("expected selection clamped to 0")
	}
}

func TestAppUndeleteSuccess(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Only", URL: "u1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	if err := app.DeleteSelected(); err != nil {
		t.Fatalf("DeleteSelected error: %v", err)
	}
	if err := app.Undelete(); err != nil {
		t.Fatalf("Undelete error: %v", err)
	}
	if !strings.Contains(app.status, "restored") {
		t.Fatalf("expected restore status")
	}
}

func TestAppUndeleteByPublishedDays(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if err := app.UndeleteByPublishedDays(0); err != nil {
		t.Fatalf("expected nil error on invalid days")
	}
	if !strings.Contains(app.status, "undelete failed") {
		t.Fatalf("expected invalid days status")
	}
	if _, err := app.store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (1, 'g1', 't', 'u', '', '', '', '', NULL, 0, 0, 0, 'f', 0)`); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	if err := app.UndeleteByPublishedDays(3); err != nil {
		t.Fatalf("expected nil error on empty restore")
	}
	if !strings.Contains(app.status, "no deleted articles") {
		t.Fatalf("expected empty restore status")
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Only", URL: "u1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	if err := app.DeleteSelected(); err != nil {
		t.Fatalf("DeleteSelected error: %v", err)
	}
	if err := app.UndeleteByPublishedDays(3); err != nil {
		t.Fatalf("UndeleteByPublishedDays error: %v", err)
	}
	if !strings.Contains(app.status, "restored") {
		t.Fatalf("expected restore status")
	}
}

func TestAppSaveToRaindropWithSummary(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "g1", Title: "T", URL: "https://example.com"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.current = Summary{ArticleID: articles[0].ID, Content: "Summary"}

	app.raindrop = &RaindropClient{
		baseURL: "http://example.test",
		token:   "token",
		client:  clientForResponse(http.StatusOK, `{"item":{"_id":9}}`, map[string]string{"content-type": "application/json"}),
	}

	if err := app.SaveToRaindrop([]string{"t"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
}

func TestNewAppWithServices(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	cfg.RaindropToken = "token"
	t.Setenv("LM_BASE_URL", "http://example.com")
	t.Setenv("LM_API_KEY", "key")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if app.summarizer == nil || app.raindrop == nil {
		t.Fatalf("expected summarizer and raindrop")
	}
}

func TestAppImportOPMLError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if err := app.ImportOPML(filepath.Join(root, "missing.opml")); err == nil {
		t.Fatalf("expected import error")
	}
}

func TestAppCopyURL(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "T", URL: "https://example.com"}}
	app.selectedIndex = 0
	orig := clipboardRun
	clipboardRun = func(cmd string, args []string, input string) error { return nil }
	t.Cleanup(func() { clipboardRun = orig })
	if err := app.CopySelectedURL(); err != nil {
		t.Fatalf("CopySelectedURL error: %v", err)
	}
	if !strings.Contains(app.status, "copied") {
		t.Fatalf("expected status update")
	}
}

func TestAppCopyURLNoArticle(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if err := app.CopySelectedURL(); err != nil {
		t.Fatalf("expected nil error")
	}
}

func TestAppGenerateMissingSummaries(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{
		{GUID: "1", Title: "One", URL: "u1"},
		{GUID: "2", Title: "Two", URL: "u2"},
	})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	_, err = app.store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "Existing"})
	if err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	app.summarizer = &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}),
	}
	app.articles = app.store.SortedArticles()
	if err := app.GenerateMissingSummaries(); err != nil {
		t.Fatalf("GenerateMissingSummaries error: %v", err)
	}
	if _, ok := app.store.FindSummary(articles[1].ID); !ok {
		t.Fatalf("expected summary for missing article")
	}
}

func TestAppGenerateMissingSummariesFailure(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "One", URL: "u1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.summarizer = &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusBadRequest, "bad", nil),
	}
	if err := app.GenerateMissingSummaries(); err == nil {
		t.Fatalf("expected batch summary error")
	}
	if !strings.Contains(app.status, "Batch summary failed") {
		t.Fatalf("expected batch summary status")
	}
}

func TestAppGenerateMissingSummariesNoConfig(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.summarizer = nil
	if err := app.GenerateMissingSummaries(); err == nil {
		t.Fatalf("expected summarizer error")
	}
}

func TestAppGenerateMissingSummariesSaveError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "One", URL: "u1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.summarizer = &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}),
	}
	_ = app.store.db.Close()
	if err := app.GenerateMissingSummaries(); err == nil {
		t.Fatalf("expected save error")
	}
}

func TestNewAppStoreError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = root
	if _, err := NewApp(cfg); err == nil {
		t.Fatalf("expected NewApp error")
	}
}

func TestAppGenerateSummaryStoreError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	_, err = app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "u1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0

	app.summarizer = &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}),
	}
	_ = app.store.db.Close()

	if err := app.GenerateSummary(); err == nil {
		t.Fatalf("expected save error")
	}
}

func TestAppDeleteSelectedStoreError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 99, Title: "T", URL: "u"}}
	app.selectedIndex = 0
	if err := app.DeleteSelected(); err == nil {
		t.Fatalf("expected delete error")
	}
}

func TestAppSaveToRaindropNoArticle(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.raindrop = &RaindropClient{baseURL: "http://example.com", token: "token", client: http.DefaultClient}
	if err := app.SaveToRaindrop(nil); err != nil {
		t.Fatalf("expected no error for empty selection")
	}
}

func TestAppSaveToRaindropError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.db")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "T", URL: "https://example.com"}}
	app.selectedIndex = 0

	app.raindrop = &RaindropClient{
		baseURL: "http://example.test",
		token:   "token",
		client:  clientForResponse(http.StatusBadRequest, "", nil),
	}

	if err := app.SaveToRaindrop(nil); err == nil {
		t.Fatalf("expected save error")
	}
}
