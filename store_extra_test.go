package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreErrorPathsWithClosedDB(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if feeds := store.Feeds(); feeds != nil {
		t.Fatalf("expected nil feeds")
	}
	if articles := store.Articles(); articles != nil {
		t.Fatalf("expected nil articles")
	}
	if summaries := store.Summaries(); summaries != nil {
		t.Fatalf("expected nil summaries")
	}
	if _, err := store.InsertFeed(Feed{Title: "A", URL: "u"}); err == nil {
		t.Fatalf("expected insert feed error")
	}
	if err := store.UpdateFeed(Feed{ID: 1}); err == nil {
		t.Fatalf("expected update feed error")
	}
	if err := store.DeleteFeed(1); err == nil {
		t.Fatalf("expected delete feed error")
	}
	if _, err := store.InsertArticles(Feed{ID: 1}, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected insert articles error")
	}
	if err := store.UpdateArticle(Article{ID: 1}); err == nil {
		t.Fatalf("expected update article error")
	}
	if _, err := store.UpsertSummary(Summary{ArticleID: 1, Content: "A"}); err == nil {
		t.Fatalf("expected upsert summary error")
	}
	if _, err := store.DeleteArticle(1); err == nil {
		t.Fatalf("expected delete article error")
	}
	if _, err := store.UndeleteLast(); err == nil {
		t.Fatalf("expected undelete error")
	}
	if count := store.DeleteOldArticles(7); count != 0 {
		t.Fatalf("expected delete old count 0")
	}
	if err := store.SaveToRaindrop(1, 2, []string{"t"}); err == nil {
		t.Fatalf("expected save to raindrop error")
	}
	if count := store.SavedCount(); count != 0 {
		t.Fatalf("expected saved count 0")
	}
	if sorted := store.SortedArticles(); sorted != nil {
		t.Fatalf("expected nil sorted articles")
	}
}

func TestInitSchemaError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if err := initSchema(db); err == nil {
		t.Fatalf("expected schema error")
	}
}

func TestNewStoreDirMismatch(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := NewStore(filepath.Join(blocker, "store.db")); err == nil {
		t.Fatalf("expected new store error")
	}
}

func TestStoreSaveToRaindropInsert(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.SaveToRaindrop(articles[0].ID, 8, []string{"a"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
	if store.SavedCount() != 1 {
		t.Fatalf("expected saved count 1")
	}
}

func newWritableStore(t *testing.T) (*Store, string) {
	path := filepath.Join(t.TempDir(), "store.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	return store, path
}

func openReadOnlyStore(t *testing.T, path string) *Store {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open read-only error: %v", err)
	}
	return &Store{path: path, db: db}
}

func TestNewStoreErrorBranches(t *testing.T) {
	if _, err := NewStore(" "); err == nil {
		t.Fatalf("expected missing path error")
	}
	origOpen := openSQLite
	openSQLite = func(string, string) (*sql.DB, error) {
		return nil, errors.New("open fail")
	}
	if _, err := NewStore(filepath.Join(t.TempDir(), "store.db")); err == nil {
		t.Fatalf("expected open error")
	}
	openSQLite = origOpen

	origSchema := schemaInit
	schemaInit = func(*sql.DB) error { return errors.New("schema fail") }
	t.Cleanup(func() { schemaInit = origSchema })
	if _, err := NewStore(filepath.Join(t.TempDir(), "store.db")); err == nil {
		t.Fatalf("expected schema error")
	}
}

func TestInitSchemaLoopError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open error: %v", err)
	}
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		t.Fatalf("pragma error: %v", err)
	}
	if err := initSchema(db); err == nil {
		t.Fatalf("expected schema error")
	}
}

func TestStoreScanErrors(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO feeds (id, title, url, last_fetched, created_at, updated_at) VALUES (1, 't', 'u', 'bad', 0, 0)`); err != nil {
		t.Fatalf("insert feed error: %v", err)
	}
	if feeds := store.Feeds(); len(feeds) != 0 {
		t.Fatalf("expected feed scan error")
	}

	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g', 't', 'u', 'bad', 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if articles := store.Articles(); len(articles) != 0 {
		t.Fatalf("expected article scan error")
	}
	if sorted := store.SortedArticles(); len(sorted) != 0 {
		t.Fatalf("expected sorted scan error")
	}

	if _, err := store.db.Exec(`INSERT INTO summaries (id, article_id, content, model, generated_at) VALUES (1, 1, 'c', 'm', 'bad')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if summaries := store.Summaries(); len(summaries) != 0 {
		t.Fatalf("expected summary scan error")
	}
}

func TestStoreSavedAndDeletedErrors(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`DROP TABLE saved`); err != nil {
		t.Fatalf("drop saved error: %v", err)
	}
	if saved := store.Saved(); saved != nil {
		t.Fatalf("expected saved query error")
	}

	store, _ = newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (?, 1, 'not-json', 'bad')`, articles[0].ID); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if saved := store.Saved(); len(saved) != 0 {
		t.Fatalf("expected saved scan error")
	}

	store, _ = newWritableStore(t)
	if _, err := store.db.Exec(`DROP TABLE deleted`); err != nil {
		t.Fatalf("drop deleted error: %v", err)
	}
	if deleted := store.Deleted(); deleted != nil {
		t.Fatalf("expected deleted query error")
	}

	store, _ = newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (1, 'g', 't', 'u', '', '', '', 'bad', 0, 0, 0, 'f', 0)`); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	if deleted := store.Deleted(); len(deleted) != 0 {
		t.Fatalf("expected deleted scan error")
	}
}

func TestDeleteArticleCleanupErrors(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE summaries`); err != nil {
		t.Fatalf("drop summaries error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[0].ID); err == nil {
		t.Fatalf("expected summary delete error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.SaveToRaindrop(articles[0].ID, 1, []string{"tag"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE saved`); err != nil {
		t.Fatalf("drop saved error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[0].ID); err == nil {
		t.Fatalf("expected saved delete error")
	}
}

func TestStoreMergeDuplicateArticles(t *testing.T) {
	store, _ := newWritableStore(t)
	feedA, err := store.InsertFeed(Feed{Title: "Feed A", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	feedB, err := store.InsertFeed(Feed{Title: "Feed B", URL: "https://example.com/b"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	base := "https://example.com/post"
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, ?, 'g1', 'One', ?, ?, '', '', '', 100, 100, 0, 0, ?)`,
		feedA.ID, base+"?x=1", base, feedA.Title); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, ?, 'g2', 'Two', ?, ?, '', '', '', 200, 200, 0, 0, ?)`,
		feedB.ID, base+"?x=2", base, feedB.Title); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO summaries (article_id, content) VALUES (2, 'summary')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (2, 1, '[]', 0)`); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err != nil {
		t.Fatalf("MergeDuplicateArticles error: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&count); err != nil {
		t.Fatalf("count error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one article after merge, got %d", count)
	}
	articles := store.Articles()
	if len(articles) != 1 {
		t.Fatalf("expected one article after merge, got %d from store.Articles", len(articles))
	}
	var summaryArticleID int
	if err := store.db.QueryRow(`SELECT article_id FROM summaries LIMIT 1`).Scan(&summaryArticleID); err != nil {
		t.Fatalf("summary scan error: %v", err)
	}
	if summaryArticleID != 1 {
		t.Fatalf("expected summary moved to 1, got %d", summaryArticleID)
	}
	if store.SavedCount() != 1 {
		t.Fatalf("expected saved moved")
	}
	if sources := store.ArticleSources(1); len(sources) != 2 {
		t.Fatalf("expected two sources")
	}
}

func TestBaseURL(t *testing.T) {
	if got := baseURL("https://example.com/post?x=1#y"); got != "https://example.com/post" {
		t.Fatalf("expected base url")
	}
	if got := baseURL(" "); got != "" {
		t.Fatalf("expected empty base url")
	}
	if got := baseURL("http://[::1"); got != "http://[::1" {
		t.Fatalf("expected parse error fallback")
	}
}

func TestMergeDuplicateArticlesKeepsExistingSummary(t *testing.T) {
	store, _ := newWritableStore(t)
	feedA, err := store.InsertFeed(Feed{Title: "Feed A", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	feedB, err := store.InsertFeed(Feed{Title: "Feed B", URL: "https://example.com/b"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	base := "https://example.com/post"
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, ?, 'g1', 'One', ?, ?, '', '', '', 100, 100, 0, 0, ?)`,
		feedA.ID, base+"?x=1", base, feedA.Title); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, ?, 'g2', 'Two', ?, ?, '', '', '', 200, 200, 0, 0, ?)`,
		feedB.ID, base+"?x=2", base, feedB.Title); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO summaries (article_id, content) VALUES (1, 'keep')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO summaries (article_id, content) VALUES (2, 'drop')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (1, 1, '[]', 0)`); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (2, 2, '[]', 0)`); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err != nil {
		t.Fatalf("MergeDuplicateArticles error: %v", err)
	}
	if count := store.SavedCount(); count != 1 {
		t.Fatalf("expected saved merged")
	}
	var summaryCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM summaries`).Scan(&summaryCount); err != nil {
		t.Fatalf("summary count error: %v", err)
	}
	if summaryCount != 1 {
		t.Fatalf("expected summary merged")
	}
}

func TestMergeDuplicateArticlesMergesReadAndStarred(t *testing.T) {
	store, _ := newWritableStore(t)
	feedA, err := store.InsertFeed(Feed{Title: "Feed A", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	feedB, err := store.InsertFeed(Feed{Title: "Feed B", URL: "https://example.com/b"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	base := "https://example.com/post"
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, ?, 'g1', 'One', ?, ?, '', '', '', 100, 100, 1, 0, ?)`,
		feedA.ID, base+"?x=1", base, feedA.Title); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, ?, 'g2', 'Two', ?, ?, '', '', '', 200, 200, 0, 1, ?)`,
		feedB.ID, base+"?x=2", base, feedB.Title); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err != nil {
		t.Fatalf("MergeDuplicateArticles error: %v", err)
	}
	articles := store.SortedArticles()
	if len(articles) != 1 {
		t.Fatalf("expected one article after merge")
	}
	if articles[0].IsRead {
		t.Fatalf("expected merged article to be unread")
	}
	if !articles[0].IsStarred {
		t.Fatalf("expected merged article to be starred")
	}
}

func TestUndeleteByPublishedDaysInvalid(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.UndeleteByPublishedDays(0); err == nil {
		t.Fatalf("expected invalid days error")
	}
}

func TestUndeleteByPublishedDaysEmpty(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.UndeleteByPublishedDays(3); err == nil {
		t.Fatalf("expected empty undelete error")
	}
}

func TestUndeleteByPublishedDaysRestoresUnread(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	published := time.Now().Add(-12 * time.Hour).UTC()
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a", PublishedAt: published, IsRead: true, IsStarred: true}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	article := articles[0]
	if err := store.UpdateArticle(article); err != nil {
		t.Fatalf("UpdateArticle error: %v", err)
	}
	if _, err := store.DeleteArticle(article.ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE deleted SET base_url = ''`); err != nil {
		t.Fatalf("update deleted base_url error: %v", err)
	}
	restored, err := store.UndeleteByPublishedDays(2)
	if err != nil {
		t.Fatalf("UndeleteByPublishedDays error: %v", err)
	}
	if restored != 1 {
		t.Fatalf("expected one restored article")
	}
	restoredArticles := store.SortedArticles()
	if len(restoredArticles) != 1 {
		t.Fatalf("expected one article")
	}
	if restoredArticles[0].IsRead {
		t.Fatalf("expected restored article to be unread")
	}
	if !restoredArticles[0].IsStarred {
		t.Fatalf("expected restored article to be starred")
	}
}

func TestUndeleteByPublishedDaysExistingArticle(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	base := "https://example.com/a"
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: base + "?x=1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	article := articles[0]
	article.IsRead = true
	if err := store.UpdateArticle(article); err != nil {
		t.Fatalf("UpdateArticle error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		feed.ID, "g2", "Deleted", base+"?x=2", base, "", "", "", timeToUnix(time.Now().UTC()), timeToUnix(time.Now().UTC()), 1, 1, feed.Title, timeToUnix(time.Now().UTC())); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	restored, err := store.UndeleteByPublishedDays(3)
	if err != nil {
		t.Fatalf("UndeleteByPublishedDays error: %v", err)
	}
	if restored != 1 {
		t.Fatalf("expected one restored")
	}
	updated := store.SortedArticles()
	if len(updated) != 1 {
		t.Fatalf("expected single article")
	}
	if updated[0].IsRead {
		t.Fatalf("expected unread after restore")
	}
	if !updated[0].IsStarred {
		t.Fatalf("expected starred after restore")
	}
	var remaining int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM deleted`).Scan(&remaining); err != nil {
		t.Fatalf("deleted count error: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected deleted cleared")
	}
}

func TestArticleSources(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	sources := store.ArticleSources(articles[0].ID)
	if len(sources) != 1 {
		t.Fatalf("expected article source")
	}

	if _, err := store.db.Exec(`DROP TABLE article_sources`); err != nil {
		t.Fatalf("drop article_sources error: %v", err)
	}
	if got := store.ArticleSources(articles[0].ID); got != nil {
		t.Fatalf("expected nil sources on query error")
	}
}

func TestArticleSourcesScanError(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE article_sources SET published_at = 'bad' WHERE article_id = ? AND feed_id = ?`, articles[0].ID, feed.ID); err != nil {
		t.Fatalf("update source error: %v", err)
	}
	if sources := store.ArticleSources(articles[0].ID); len(sources) != 0 {
		t.Fatalf("expected scan error sources")
	}
}

func TestFindArticleIDByBaseURL(t *testing.T) {
	store, _ := newWritableStore(t)
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin error: %v", err)
	}
	defer tx.Rollback()
	if id, err := findArticleIDByBaseURL(tx, " "); err != nil || id != 0 {
		t.Fatalf("expected empty base url")
	}
	if _, err := store.db.Exec(`DROP TABLE articles`); err != nil {
		t.Fatalf("drop articles error: %v", err)
	}
	if _, err := findArticleIDByBaseURL(tx, "https://example.com"); err == nil {
		t.Fatalf("expected find base url error")
	}
}

func TestEnsureArticleSourceBranches(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin error: %v", err)
	}
	if err := ensureArticleSource(tx, articles[0].ID, feed.ID, time.Time{}); err != nil {
		t.Fatalf("ensureArticleSource insert error: %v", err)
	}
	if err := ensureArticleSource(tx, articles[0].ID, feed.ID, time.Time{}); err != nil {
		t.Fatalf("ensureArticleSource no-op error: %v", err)
	}
	if err := ensureArticleSource(tx, articles[0].ID, feed.ID, time.Unix(123, 0)); err != nil {
		t.Fatalf("ensureArticleSource update error: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE article_sources`); err != nil {
		t.Fatalf("drop article_sources error: %v", err)
	}
	tx, err = store.db.Begin()
	if err != nil {
		t.Fatalf("begin error: %v", err)
	}
	defer tx.Rollback()
	if err := ensureArticleSource(tx, articles[0].ID, feed.ID, time.Unix(123, 0)); err == nil {
		t.Fatalf("expected ensureArticleSource query error")
	}
}

func TestInsertArticlesDedupByBaseURL(t *testing.T) {
	store, _ := newWritableStore(t)
	feedA, err := store.InsertFeed(Feed{Title: "Feed A", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	feedB, err := store.InsertFeed(Feed{Title: "Feed B", URL: "https://example.com/b"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	added, err := store.InsertArticles(feedA, []Article{{Title: "A", URL: "https://example.com/post?x=1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if len(added) != 1 {
		t.Fatalf("expected added article")
	}
	added, err = store.InsertArticles(feedB, []Article{{Title: "A", URL: "https://example.com/post?x=2"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("expected no added duplicates")
	}
	if sources := store.ArticleSources(addedArticleID(t, store)); len(sources) != 2 {
		t.Fatalf("expected two sources")
	}
}

func addedArticleID(t *testing.T, store *Store) int {
	t.Helper()
	var id int
	if err := store.db.QueryRow(`SELECT id FROM articles LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("article id error: %v", err)
	}
	return id
}

func TestExistsByID(t *testing.T) {
	store, _ := newWritableStore(t)
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin error: %v", err)
	}
	defer tx.Rollback()
	ok, err := existsByID(tx, "summaries", 1)
	if err != nil {
		t.Fatalf("existsByID error: %v", err)
	}
	if ok {
		t.Fatalf("expected no summary")
	}
	if _, err := tx.Exec(`DROP TABLE summaries`); err != nil {
		t.Fatalf("drop summaries error: %v", err)
	}
	if _, err := existsByID(tx, "summaries", 1); err == nil {
		t.Fatalf("expected existsByID error")
	}
}

func TestDeleteFeedSuccess(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.DeleteFeed(feed.ID); err != nil {
		t.Fatalf("DeleteFeed error: %v", err)
	}
	if count := len(store.Feeds()); count != 0 {
		t.Fatalf("expected feeds deleted")
	}
}

func TestUpdateArticleBaseURL(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a?x=1"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	article := articles[0]
	article.BaseURL = ""
	if err := store.UpdateArticle(article); err != nil {
		t.Fatalf("UpdateArticle error: %v", err)
	}
	var base string
	if err := store.db.QueryRow(`SELECT base_url FROM articles WHERE id = ?`, article.ID).Scan(&base); err != nil {
		t.Fatalf("base url query error: %v", err)
	}
	if base == "" {
		t.Fatalf("expected base_url set")
	}
}

func TestInsertArticlesBaseURLErrorBranches(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	origFind := findArticleIDByBaseURLFn
	findArticleIDByBaseURLFn = func(*sql.Tx, string) (int, error) {
		return 0, errors.New("find error")
	}
	t.Cleanup(func() { findArticleIDByBaseURLFn = origFind })
	if _, err := store.InsertArticles(feed, []Article{{Title: "A", URL: "https://example.com/a"}}); err == nil {
		t.Fatalf("expected find base url error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	origEnsure := ensureArticleSourceFn
	ensureArticleSourceFn = func(*sql.Tx, int, int, time.Time) error {
		return errors.New("source error")
	}
	t.Cleanup(func() { ensureArticleSourceFn = origEnsure })
	if _, err := store.InsertArticles(feed, []Article{{Title: "A", URL: "https://example.com/a"}}); err == nil {
		t.Fatalf("expected ensure source error")
	}
}

func TestInsertArticlesExistingSourceError(t *testing.T) {
	store, _ := newWritableStore(t)
	feedA, err := store.InsertFeed(Feed{Title: "Feed A", URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feedA, []Article{{Title: "A", URL: "https://example.com/post?x=1"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	feedB, err := store.InsertFeed(Feed{Title: "Feed B", URL: "https://example.com/b"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	origEnsure := ensureArticleSourceFn
	ensureArticleSourceFn = func(*sql.Tx, int, int, time.Time) error { return errors.New("source") }
	t.Cleanup(func() { ensureArticleSourceFn = origEnsure })
	if _, err := store.InsertArticles(feedB, []Article{{Title: "A", URL: "https://example.com/post?x=2"}}); err == nil {
		t.Fatalf("expected existing source error")
	}
}

func TestInsertArticlesUpdateFeedError(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER feeds_update_block BEFORE UPDATE ON feeds BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{Title: "A", URL: "https://example.com/a"}}); err == nil {
		t.Fatalf("expected update feed error")
	}
}

func TestUndeleteLastNoDeleted(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.UndeleteLast(); err == nil {
		t.Fatalf("expected undelete error")
	}
}

func TestInsertArticlesEmptyURL(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: ""}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	var base string
	if err := store.db.QueryRow(`SELECT base_url FROM articles LIMIT 1`).Scan(&base); err != nil {
		t.Fatalf("base url scan error: %v", err)
	}
	if base != "" {
		t.Fatalf("expected empty base_url")
	}
}

func TestUndeleteLastBaseURLFallback(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (1, 'g1', 't', 'https://example.com/a?x=1', '', '', '', '', 0, 0, 0, 0, 'f', 0)`); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	article, err := store.UndeleteLast()
	if err != nil {
		t.Fatalf("UndeleteLast error: %v", err)
	}
	if article.BaseURL == "" {
		t.Fatalf("expected base_url set")
	}
}

func TestInsertFeedExecAndLastInsertErrors(t *testing.T) {
	store, path := newWritableStore(t)
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro := openReadOnlyStore(t, path)
	defer ro.db.Close()
	if _, err := ro.InsertFeed(Feed{Title: "T", URL: "https://example.com"}); err == nil {
		t.Fatalf("expected insert feed error")
	}

	store, _ = newWritableStore(t)
	orig := lastInsertID
	lastInsertID = func(sql.Result) (int64, error) { return 0, errors.New("last id") }
	t.Cleanup(func() { lastInsertID = orig })
	if _, err := store.InsertFeed(Feed{Title: "T", URL: "https://example.com"}); err == nil {
		t.Fatalf("expected last insert error")
	}
}

func TestInsertArticlesErrorBranches(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE articles`); err != nil {
		t.Fatalf("drop articles: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected insert articles error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (feed_id, guid, title, url, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (?, NULL, 't', 'u', 0, 0, 0, 0, 'f')`, feed.ID); err != nil {
		t.Fatalf("insert bad article: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "2", Title: "B", URL: "u2"}}); err == nil {
		t.Fatalf("expected article guid scan error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE deleted`); err != nil {
		t.Fatalf("drop deleted: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected deleted query error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid) VALUES (?, NULL)`, feed.ID); err != nil {
		t.Fatalf("insert bad deleted: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected deleted scan error")
	}

	store, path := newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro := openReadOnlyStore(t, path)
	defer ro.db.Close()
	if _, err := ro.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected insert exec error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	origLast := lastInsertID
	lastInsertID = func(sql.Result) (int64, error) { return 0, errors.New("last id") }
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected last insert error")
	}
	lastInsertID = origLast

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE feeds`); err != nil {
		t.Fatalf("drop feeds: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected update feed error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	origCommit := commitTx
	commitTx = func(*sql.Tx) error { return errors.New("commit fail") }
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected commit error")
	}
	commitTx = origCommit
}

func TestUpdateAndDeleteErrorBranches(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}

	origRows := rowsAffected
	rowsAffected = func(sql.Result) (int64, error) { return 0, errors.New("rows fail") }
	t.Cleanup(func() { rowsAffected = origRows })
	if err := store.UpdateFeed(feed); err == nil {
		t.Fatalf("expected rows affected error")
	}
	if err := store.UpdateArticle(articles[0]); err == nil {
		t.Fatalf("expected rows affected error")
	}

	origBegin := beginTx
	beginTx = func(*sql.DB) (*sql.Tx, error) { return nil, errors.New("begin fail") }
	t.Cleanup(func() { beginTx = origBegin })
	if _, err := store.DeleteArticle(articles[0].ID); err == nil {
		t.Fatalf("expected begin error")
	}
}

func TestDeleteFeedExecErrors(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`DROP TABLE feeds`); err != nil {
		t.Fatalf("drop feeds: %v", err)
	}
	if err := store.DeleteFeed(feed.ID); err == nil {
		t.Fatalf("expected delete feeds error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER articles_delete_block BEFORE DELETE ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.DeleteFeed(feed.ID); err == nil {
		t.Fatalf("expected delete articles error")
	}
}

func TestUpsertSummaryErrorBranches(t *testing.T) {
	store, path := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro := openReadOnlyStore(t, path)
	defer ro.db.Close()
	if _, err := ro.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "A"}); err == nil {
		t.Fatalf("expected insert error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "A"}); err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	ro = openReadOnlyStore(t, store.path)
	defer ro.db.Close()
	if _, err := ro.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "B"}); err == nil {
		t.Fatalf("expected update error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	orig := lastInsertID
	lastInsertID = func(sql.Result) (int64, error) { return 0, errors.New("last id") }
	t.Cleanup(func() { lastInsertID = orig })
	if _, err := store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "A"}); err == nil {
		t.Fatalf("expected last insert error")
	}
}

func TestDeleteAndUndeleteErrors(t *testing.T) {
	store, path := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro := openReadOnlyStore(t, path)
	defer ro.db.Close()
	if _, err := ro.DeleteArticle(articles[0].ID); err == nil {
		t.Fatalf("expected delete exec error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER deleted_block BEFORE INSERT ON deleted BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[0].ID); err == nil {
		t.Fatalf("expected insert deleted error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	origCommit := commitTx
	commitTx = func(*sql.Tx) error { return errors.New("commit fail") }
	if _, err := store.DeleteArticle(articles[0].ID); err == nil {
		t.Fatalf("expected commit error")
	}
	commitTx = origCommit

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[0].ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro = openReadOnlyStore(t, store.path)
	defer ro.db.Close()
	if _, err := ro.UndeleteLast(); err == nil {
		t.Fatalf("expected undelete insert error")
	}

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[0].ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	origLast := lastInsertID
	lastInsertID = func(sql.Result) (int64, error) { return 0, errors.New("last id") }
	if _, err := store.UndeleteLast(); err == nil {
		t.Fatalf("expected undelete last insert error")
	}
	lastInsertID = origLast

	store, _ = newWritableStore(t)
	feed, err = store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err = store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[0].ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER deleted_delete_block BEFORE DELETE ON deleted BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if _, err := store.UndeleteLast(); err == nil {
		t.Fatalf("expected delete error")
	}
}

func TestDeleteOldAndSaveErrors(t *testing.T) {
	store, path := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "1", Title: "A", URL: "u"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro := openReadOnlyStore(t, path)
	defer ro.db.Close()
	if removed := ro.DeleteOldArticles(7); removed != 0 {
		t.Fatalf("expected delete old error")
	}

	store, _ = newWritableStore(t)
	orig := tagsMarshal
	tagsMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal fail") }
	if err := store.SaveToRaindrop(1, 2, []string{"t"}); err == nil {
		t.Fatalf("expected marshal error")
	}
	tagsMarshal = orig

	store, _ = newWritableStore(t)
	origRows := rowsAffected
	rowsAffected = func(sql.Result) (int64, error) { return 0, errors.New("rows fail") }
	if err := store.SaveToRaindrop(1, 2, []string{"t"}); err == nil {
		t.Fatalf("expected rows affected error")
	}
	rowsAffected = origRows

	store, _ = newWritableStore(t)
	if _, err := store.db.Exec(`CREATE TRIGGER saved_block BEFORE INSERT ON saved BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.SaveToRaindrop(1, 2, []string{"t"}); err == nil {
		t.Fatalf("expected insert error")
	}
}
