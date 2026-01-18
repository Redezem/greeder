package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCRUD(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	feed, err := store.InsertFeed(Feed{Title: "Test", URL: "https://example.com/feed"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertFeed(feed); err == nil {
		t.Fatalf("expected duplicate feed error")
	}
	feed.Description = "desc"
	if err := store.UpdateFeed(feed); err != nil {
		t.Fatalf("UpdateFeed error: %v", err)
	}
	if err := store.UpdateFeed(Feed{ID: 999}); err == nil {
		t.Fatalf("expected missing feed error")
	}

	articles, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "One", URL: "https://example.com/1"}})
	if err != nil || len(articles) != 1 {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "Dup", URL: "https://example.com/1"}}); err != nil {
		t.Fatalf("InsertArticles duplicate error: %v", err)
	}
	article := articles[0]
	article.IsRead = true
	if err := store.UpdateArticle(article); err != nil {
		t.Fatalf("UpdateArticle error: %v", err)
	}
	if err := store.UpdateArticle(Article{ID: 999}); err == nil {
		t.Fatalf("expected missing article error")
	}

	if _, err := store.DeleteArticle(999); err == nil {
		t.Fatalf("expected delete missing error")
	}
	deleted, err := store.DeleteArticle(article.ID)
	if err != nil || deleted.ID != article.ID {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	if _, err := store.UndeleteLast(); err != nil {
		t.Fatalf("UndeleteLast error: %v", err)
	}
	if _, err := store.UndeleteLast(); err == nil {
		t.Fatalf("expected undelete error")
	}

	if err := store.SaveToRaindrop(article.ID, 10, []string{"tag"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
	if err := store.SaveToRaindrop(article.ID, 11, []string{"tag2"}); err != nil {
		t.Fatalf("SaveToRaindrop update error: %v", err)
	}
	if store.SavedCount() != 1 {
		t.Fatalf("expected saved count 1")
	}

	oldArticle := Article{GUID: "old", Title: "Old", URL: "https://example.com/old", FetchedAt: time.Now().Add(-10 * 24 * time.Hour)}
	if _, err := store.InsertArticles(feed, []Article{oldArticle}); err != nil {
		t.Fatalf("InsertArticles old error: %v", err)
	}
	removed := store.DeleteOldArticles(7)
	if removed == 0 {
		t.Fatalf("expected old article removal")
	}
	_ = store.Compact(7)

	if err := store.DeleteFeed(feed.ID); err != nil {
		t.Fatalf("DeleteFeed error: %v", err)
	}
}

func TestStoreLoadEmptyFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty.json")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if len(store.Feeds()) != 0 {
		t.Fatalf("expected empty feeds")
	}
}

func TestStoreInvalidJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := NewStore(path); err == nil {
		t.Fatalf("expected json error")
	}
}

func TestStoreSummariesAndSave(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(store.Summaries()) != 0 {
		t.Fatalf("expected empty summaries")
	}
	summary, err := store.UpsertSummary(Summary{ArticleID: 1, Content: "A", Model: "m"})
	if err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	if found, ok := store.FindSummary(1); !ok || found.Content != "A" {
		t.Fatalf("expected summary lookup")
	}
	summary.Content = "B"
	if _, err := store.UpsertSummary(summary); err != nil {
		t.Fatalf("UpsertSummary update error: %v", err)
	}
	if _, ok := store.FindSummary(999); ok {
		t.Fatalf("expected no summary")
	}
}

func TestStoreSaveError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	store := &Store{path: filepath.Join(blocker, "child.json")}
	store.data = storeData{NextFeedID: 1, NextArticleID: 1, NextSummaryID: 1}
	if err := store.Save(); err == nil {
		t.Fatalf("expected save error")
	}
}

func TestStoreSortedArticles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	feed, err := store.InsertFeed(Feed{Title: "Test", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	now := time.Now().UTC()
	_, err = store.InsertArticles(feed, []Article{
		{GUID: "1", Title: "A", URL: "u1", PublishedAt: now.Add(-time.Hour)},
		{GUID: "2", Title: "B", URL: "u2", PublishedAt: now},
	})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	sorted := store.SortedArticles()
	if len(sorted) < 2 || sorted[0].GUID != "2" {
		t.Fatalf("unexpected sort order")
	}
}

func TestStoreLoadDefaultsFromJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	if err := os.WriteFile(path, []byte(`{"feeds":[],"articles":[],"summaries":[],"saved":[],"deleted":[]}`), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if store.data.NextFeedID != 1 || store.data.NextArticleID != 1 || store.data.NextSummaryID != 1 {
		t.Fatalf("expected default counters")
	}
}

func TestStoreSaveMarshalError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	orig := storeJSONMarshal
	storeJSONMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("marshal fail")
	}
	t.Cleanup(func() { storeJSONMarshal = orig })

	if err := store.Save(); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestStoreInsertArticlesGuidsAndDeleted(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	feed, err := store.InsertFeed(Feed{Title: "Test", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "", Title: "A", URL: "u1"}})
	if err != nil || len(articles) != 1 || articles[0].GUID != "u1" {
		t.Fatalf("expected guid fallback")
	}
	if _, err := store.DeleteArticle(articles[0].ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "u1", Title: "A", URL: "u1"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
}

func TestStoreNewStoreSaveError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := NewStore(filepath.Join(blocker, "store.json")); err == nil {
		t.Fatalf("expected new store error")
	}
}

func TestStoreDeleteFeedNoMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if _, err := store.InsertFeed(Feed{Title: "Test", URL: "https://example.com/rss"}); err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if err := store.DeleteFeed(999); err != nil {
		t.Fatalf("DeleteFeed error: %v", err)
	}
}

func TestStoreDeleteFeedKeepsOtherArticles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	feed1, err := store.InsertFeed(Feed{Title: "One", URL: "https://example.com/1"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	feed2, err := store.InsertFeed(Feed{Title: "Two", URL: "https://example.com/2"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feed2, []Article{{GUID: "a", Title: "A", URL: "u"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.DeleteFeed(feed1.ID); err != nil {
		t.Fatalf("DeleteFeed error: %v", err)
	}
	if len(store.Articles()) != 1 {
		t.Fatalf("expected other articles preserved")
	}
}
