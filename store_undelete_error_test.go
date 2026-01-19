package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func seedDeletedArticle(t *testing.T, store *Store) Article {
	t.Helper()
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "https://example.com/a", PublishedAt: time.Now().UTC()}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	article := articles[0]
	if _, err := store.DeleteArticle(article.ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}
	return article
}

func TestUndeleteByPublishedDaysCountError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`DROP TABLE deleted`); err != nil {
		t.Fatalf("drop deleted error: %v", err)
	}
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected count error")
	}
}

func TestUndeleteByPublishedDaysMaxPublishedError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite error: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE deleted (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create deleted error: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO deleted (id) VALUES (1)`); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	store := &Store{path: path, db: db}
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected max published error")
	}
}

func TestUndeleteByPublishedDaysBeginError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := beginTx
	beginTx = func(*sql.DB) (*sql.Tx, error) { return nil, errors.New("begin") }
	t.Cleanup(func() { beginTx = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected begin error")
	}
}

func TestUndeleteByPublishedDaysQueryError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := beginTx
	beginTx = func(db *sql.DB) (*sql.Tx, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DROP TABLE deleted`); err != nil {
			return nil, err
		}
		return tx, nil
	}
	t.Cleanup(func() { beginTx = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected query error")
	}
}

func TestUndeleteByPublishedDaysScanError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (1, 'g', 't', 'u', '', '', '', '', 1, 0, 'bad', 0, 'f', 0)`); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected scan error")
	}
}

func TestUndeleteByPublishedDaysFindError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := findArticleIDByBaseURLFn
	findArticleIDByBaseURLFn = func(*sql.Tx, string) (int, error) { return 0, errors.New("find") }
	t.Cleanup(func() { findArticleIDByBaseURLFn = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected find error")
	}
}

func TestUndeleteByPublishedDaysEnsureSourceExistingError(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	base := "https://example.com/a"
	articles, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: base}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (?, 'g2', 't', ?, ?, '', '', '', 1, 0, 0, 0, ?, 0)`,
		feed.ID, base+"?x=1", base, feed.Title); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	orig := ensureArticleSourceFn
	ensureArticleSourceFn = func(*sql.Tx, int, int, time.Time) error { return errors.New("source") }
	t.Cleanup(func() { ensureArticleSourceFn = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected ensure source error")
	}
	if len(articles) == 0 {
		t.Fatalf("expected article for coverage")
	}
}

func TestUndeleteByPublishedDaysUpdateErrorExisting(t *testing.T) {
	store, _ := newWritableStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	base := "https://example.com/a"
	if _, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: base}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (?, 'g2', 't', ?, ?, '', '', '', 1, 0, 0, 0, ?, 0)`,
		feed.ID, base+"?x=1", base, feed.Title); err != nil {
		t.Fatalf("insert deleted error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER articles_update_block BEFORE UPDATE OF is_read ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected update error")
	}
}

func TestUndeleteByPublishedDaysInsertError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	if _, err := store.db.Exec(`CREATE TRIGGER articles_insert_block BEFORE INSERT ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected insert error")
	}
}

func TestUndeleteByPublishedDaysLastInsertIDError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := lastInsertID
	lastInsertID = func(sql.Result) (int64, error) { return 0, errors.New("last") }
	t.Cleanup(func() { lastInsertID = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected last insert error")
	}
}

func TestUndeleteByPublishedDaysEnsureSourceNewError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := ensureArticleSourceFn
	ensureArticleSourceFn = func(*sql.Tx, int, int, time.Time) error { return errors.New("source") }
	t.Cleanup(func() { ensureArticleSourceFn = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected ensure source error")
	}
}

func TestUndeleteByPublishedDaysDeleteError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	if _, err := store.db.Exec(`CREATE TRIGGER deleted_delete_block BEFORE DELETE ON deleted BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected delete error")
	}
}

func TestUndeleteByPublishedDaysRowsError(t *testing.T) {
	registerErrRowsDriver()
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := beginTx
	beginTx = func(*sql.DB) (*sql.Tx, error) {
		db, err := sql.Open("errrows", "")
		if err != nil {
			return nil, err
		}
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		return tx, nil
	}
	t.Cleanup(func() { beginTx = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected rows error")
	}
}

func TestUndeleteByPublishedDaysCommitError(t *testing.T) {
	store, _ := newWritableStore(t)
	seedDeletedArticle(t, store)
	orig := commitTx
	commitTx = func(*sql.Tx) error { return errors.New("commit") }
	t.Cleanup(func() { commitTx = orig })
	if _, err := store.UndeleteByPublishedDays(1); err == nil {
		t.Fatalf("expected commit error")
	}
}

var errRowsRegisterOnce sync.Once

func registerErrRowsDriver() {
	errRowsRegisterOnce.Do(func() {
		sql.Register("errrows", errRowsDriver{})
	})
}

type errRowsDriver struct{}

func (errRowsDriver) Open(name string) (driver.Conn, error) {
	return &errRowsConn{}, nil
}

type errRowsConn struct{}

func (c *errRowsConn) Prepare(query string) (driver.Stmt, error) {
	return &errRowsStmt{}, nil
}

func (c *errRowsConn) Close() error { return nil }

func (c *errRowsConn) Begin() (driver.Tx, error) { return &errRowsTx{}, nil }

func (c *errRowsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return &errRowsRows{}, nil
}

type errRowsStmt struct{}

func (s *errRowsStmt) Close() error                                    { return nil }
func (s *errRowsStmt) NumInput() int                                   { return 0 }
func (s *errRowsStmt) Exec(args []driver.Value) (driver.Result, error) { return errRowsResult(0), nil }
func (s *errRowsStmt) Query(args []driver.Value) (driver.Rows, error)  { return &errRowsRows{}, nil }

type errRowsTx struct{}

func (t *errRowsTx) Commit() error   { return nil }
func (t *errRowsTx) Rollback() error { return nil }

type errRowsRows struct{}

func (r *errRowsRows) Columns() []string { return []string{"id"} }
func (r *errRowsRows) Close() error      { return nil }
func (r *errRowsRows) Next(dest []driver.Value) error {
	return errors.New("rows error")
}

type errRowsResult int64

func (r errRowsResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r errRowsResult) RowsAffected() (int64, error) { return int64(r), nil }
