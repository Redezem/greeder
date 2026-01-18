package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
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
	if err := store.SaveToRaindrop(7, 8, []string{"a"}); err != nil {
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
	if _, err := store.db.Exec(`DROP TABLE articles`); err != nil {
		t.Fatalf("drop articles: %v", err)
	}
	if err := store.DeleteFeed(feed.ID); err == nil {
		t.Fatalf("expected delete articles error")
	}
}

func TestUpsertSummaryErrorBranches(t *testing.T) {
	store, path := newWritableStore(t)
	if err := store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	ro := openReadOnlyStore(t, path)
	defer ro.db.Close()
	if _, err := ro.UpsertSummary(Summary{ArticleID: 1, Content: "A"}); err == nil {
		t.Fatalf("expected insert error")
	}

	store, _ = newWritableStore(t)
	if _, err := store.UpsertSummary(Summary{ArticleID: 1, Content: "A"}); err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	ro = openReadOnlyStore(t, store.path)
	defer ro.db.Close()
	if _, err := ro.UpsertSummary(Summary{ArticleID: 1, Content: "B"}); err == nil {
		t.Fatalf("expected update error")
	}

	store, _ = newWritableStore(t)
	orig := lastInsertID
	lastInsertID = func(sql.Result) (int64, error) { return 0, errors.New("last id") }
	t.Cleanup(func() { lastInsertID = orig })
	if _, err := store.UpsertSummary(Summary{ArticleID: 2, Content: "A"}); err == nil {
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
