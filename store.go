package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.Mutex
	data storeData
}

type storeData struct {
	NextFeedID    int       `json:"next_feed_id"`
	NextArticleID int       `json:"next_article_id"`
	NextSummaryID int       `json:"next_summary_id"`
	Feeds         []Feed    `json:"feeds"`
	Articles      []Article `json:"articles"`
	Summaries     []Summary `json:"summaries"`
	Saved         []Saved   `json:"saved"`
	Deleted       []Deleted `json:"deleted"`
}

var storeJSONMarshal = func(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func NewStore(path string) (*Store, error) {
	store := &Store{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = storeData{NextFeedID: 1, NextArticleID: 1, NextSummaryID: 1}
			return s.saveLocked()
		}
		return err
	}
	if len(data) == 0 {
		s.data = storeData{NextFeedID: 1, NextArticleID: 1, NextSummaryID: 1}
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}
	if s.data.NextFeedID == 0 {
		s.data.NextFeedID = 1
	}
	if s.data.NextArticleID == 0 {
		s.data.NextArticleID = 1
	}
	if s.data.NextSummaryID == 0 {
		s.data.NextSummaryID = 1
	}
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	blob, err := storeJSONMarshal(s.data)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, blob, 0o600)
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) Feeds() []Feed {
	s.mu.Lock()
	defer s.mu.Unlock()
	feeds := append([]Feed(nil), s.data.Feeds...)
	return feeds
}

func (s *Store) Articles() []Article {
	s.mu.Lock()
	defer s.mu.Unlock()
	articles := append([]Article(nil), s.data.Articles...)
	return articles
}

func (s *Store) Summaries() []Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]Summary(nil), s.data.Summaries...)
	return items
}

func (s *Store) InsertFeed(feed Feed) (Feed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.data.Feeds {
		if existing.URL == feed.URL {
			return Feed{}, errors.New("feed already exists")
		}
	}
	feed.ID = s.data.NextFeedID
	s.data.NextFeedID++
	if feed.CreatedAt.IsZero() {
		now := time.Now().UTC()
		feed.CreatedAt = now
		feed.UpdatedAt = now
	}
	s.data.Feeds = append(s.data.Feeds, feed)
	return feed, s.saveLocked()
}

func (s *Store) UpdateFeed(fetch Feed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, feed := range s.data.Feeds {
		if feed.ID == fetch.ID {
			fetch.UpdatedAt = time.Now().UTC()
			s.data.Feeds[i] = fetch
			return s.saveLocked()
		}
	}
	return errors.New("feed not found")
}

func (s *Store) DeleteFeed(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	feeds := s.data.Feeds[:0]
	for _, feed := range s.data.Feeds {
		if feed.ID != id {
			feeds = append(feeds, feed)
		}
	}
	s.data.Feeds = feeds
	articles := s.data.Articles[:0]
	for _, article := range s.data.Articles {
		if article.FeedID != id {
			articles = append(articles, article)
		}
	}
	s.data.Articles = articles
	return s.saveLocked()
}

func (s *Store) InsertArticles(feed Feed, incoming []Article) ([]Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byKey := map[string]bool{}
	for _, article := range s.data.Articles {
		key := articleKey(article.FeedID, article.GUID)
		byKey[key] = true
	}
	for _, deleted := range s.data.Deleted {
		key := articleKey(deleted.FeedID, deleted.GUID)
		byKey[key] = true
	}

	added := []Article{}
	for _, article := range incoming {
		if article.GUID == "" {
			article.GUID = article.URL
		}
		key := articleKey(feed.ID, article.GUID)
		if byKey[key] {
			continue
		}
		byKey[key] = true
		article.ID = s.data.NextArticleID
		s.data.NextArticleID++
		article.FeedID = feed.ID
		article.FeedTitle = feed.Title
		if article.FetchedAt.IsZero() {
			article.FetchedAt = time.Now().UTC()
		}
		added = append(added, article)
		s.data.Articles = append(s.data.Articles, article)
	}

	feed.LastFetched = time.Now().UTC()
	feed.UpdatedAt = time.Now().UTC()
	for i := range s.data.Feeds {
		if s.data.Feeds[i].ID == feed.ID {
			s.data.Feeds[i] = feed
			break
		}
	}

	return added, s.saveLocked()
}

func (s *Store) FindSummary(articleID int) (Summary, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, summary := range s.data.Summaries {
		if summary.ArticleID == articleID {
			return summary, true
		}
	}
	return Summary{}, false
}

func (s *Store) UpsertSummary(summary Summary) (Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.data.Summaries {
		if existing.ArticleID == summary.ArticleID {
			summary.ID = existing.ID
			s.data.Summaries[i] = summary
			return summary, s.saveLocked()
		}
	}
	summary.ID = s.data.NextSummaryID
	s.data.NextSummaryID++
	s.data.Summaries = append(s.data.Summaries, summary)
	return summary, s.saveLocked()
}

func (s *Store) UpdateArticle(article Article) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.data.Articles {
		if existing.ID == article.ID {
			s.data.Articles[i] = article
			return s.saveLocked()
		}
	}
	return errors.New("article not found")
}

func (s *Store) DeleteArticle(id int) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, article := range s.data.Articles {
		if article.ID == id {
			s.data.Articles = append(s.data.Articles[:i], s.data.Articles[i+1:]...)
			deleted := Deleted{FeedID: article.FeedID, GUID: article.GUID, DeletedAt: time.Now().UTC(), Article: article}
			s.data.Deleted = append(s.data.Deleted, deleted)
			return article, s.saveLocked()
		}
	}
	return Article{}, errors.New("article not found")
}

func (s *Store) UndeleteLast() (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data.Deleted) == 0 {
		return Article{}, errors.New("no deleted article")
	}
	deleted := s.data.Deleted[len(s.data.Deleted)-1]
	s.data.Deleted = s.data.Deleted[:len(s.data.Deleted)-1]
	article := deleted.Article
	article.ID = s.data.NextArticleID
	s.data.NextArticleID++
	s.data.Articles = append(s.data.Articles, article)
	return article, s.saveLocked()
}

func (s *Store) DeleteOldArticles(days int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	kept := s.data.Articles[:0]
	removed := 0
	for _, article := range s.data.Articles {
		if article.FetchedAt.Before(cutoff) {
			removed++
			continue
		}
		kept = append(kept, article)
	}
	s.data.Articles = kept
	_ = s.saveLocked()
	return removed
}

func (s *Store) Compact(days int) int {
	return s.DeleteOldArticles(days)
}

func (s *Store) SaveToRaindrop(articleID int, raindropID int, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, saved := range s.data.Saved {
		if saved.ArticleID == articleID {
			s.data.Saved[i].RaindropID = raindropID
			s.data.Saved[i].Tags = tags
			return s.saveLocked()
		}
	}
	s.data.Saved = append(s.data.Saved, Saved{ArticleID: articleID, RaindropID: raindropID, Tags: tags, SavedAt: time.Now().UTC()})
	return s.saveLocked()
}

func (s *Store) SavedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Saved)
}

func (s *Store) SortedArticles() []Article {
	articles := s.Articles()
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})
	return articles
}

func articleKey(feedID int, guid string) string {
	return strconv.Itoa(feedID) + "::" + guid
}
