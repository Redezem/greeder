package main

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

func insertMergeFeed(t *testing.T, store *Store, id int) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO feeds (id, title, url) VALUES (?, ?, ?)`,
		id, fmt.Sprintf("Feed %d", id), fmt.Sprintf("https://example.com/%d", id)); err != nil {
		t.Fatalf("insert feed error: %v", err)
	}
}

func TestMergeDuplicateArticlesBeginError(t *testing.T) {
	store, _ := newWritableStore(t)
	orig := beginTx
	beginTx = func(*sql.DB) (*sql.Tx, error) { return nil, errors.New("begin") }
	t.Cleanup(func() { beginTx = orig })
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected begin error")
	}
}

func TestMergeDuplicateArticlesQueryError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`DROP TABLE articles`); err != nil {
		t.Fatalf("drop articles error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected query error")
	}
}

func TestMergeDuplicateArticlesScanError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 'bad', 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected scan error")
	}
}

func TestMergeDuplicateArticlesUpdateBaseURLError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'https://example.com/post?x=1', '', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER base_block BEFORE UPDATE OF base_url ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected update base_url error")
	}
}

func TestMergeDuplicateArticlesEnsureSourceError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	orig := ensureArticleSourceFn
	ensureArticleSourceFn = func(*sql.Tx, int, int, time.Time) error { return errors.New("source") }
	t.Cleanup(func() { ensureArticleSourceFn = orig })
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected ensure source error")
	}
}

func TestMergeDuplicateArticlesEnsureSourceDuplicateError(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	orig := ensureArticleSourceFn
	calls := 0
	ensureArticleSourceFn = func(*sql.Tx, int, int, time.Time) error {
		calls++
		if calls == 2 {
			return errors.New("source")
		}
		return nil
	}
	t.Cleanup(func() { ensureArticleSourceFn = orig })
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected ensure source duplicate error")
	}
}

func TestMergeDuplicateArticlesExistsByIDError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	orig := existsByIDFn
	existsByIDFn = func(*sql.Tx, string, int) (bool, error) { return false, errors.New("exists") }
	t.Cleanup(func() { existsByIDFn = orig })
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected existsByID error")
	}
}

func TestMergeDuplicateArticlesExistsByIDSavedError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	orig := existsByIDFn
	existsByIDFn = func(tx *sql.Tx, table string, articleID int) (bool, error) {
		if table == "summaries" {
			return false, nil
		}
		return false, errors.New("exists")
	}
	t.Cleanup(func() { existsByIDFn = orig })
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected existsByID saved error")
	}
}

func TestMergeDuplicateArticlesDeleteSummaryError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO summaries (article_id, content) VALUES (1, 'keep')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO summaries (article_id, content) VALUES (2, 'drop')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER summaries_delete_block BEFORE DELETE ON summaries BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected delete summary error")
	}
}

func TestMergeDuplicateArticlesUpdateSummaryError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO summaries (article_id, content) VALUES (2, 'move')`); err != nil {
		t.Fatalf("insert summary error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER summaries_update_block BEFORE UPDATE ON summaries BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected update summary error")
	}
}

func TestMergeDuplicateArticlesDeleteSavedError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (1, 1, '[]', 0)`); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (2, 2, '[]', 0)`); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER saved_delete_block BEFORE DELETE ON saved BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected delete saved error")
	}
}

func TestMergeDuplicateArticlesUpdateSavedError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (2, 2, '[]', 0)`); err != nil {
		t.Fatalf("insert saved error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER saved_update_block BEFORE UPDATE ON saved BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected update saved error")
	}
}

func TestMergeDuplicateArticlesDeleteArticleError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER articles_delete_block BEFORE DELETE ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected delete article error")
	}
}

func TestMergeDuplicateArticlesUpdateFlagsError(t *testing.T) {
	store, _ := newWritableStore(t)
	insertMergeFeed(t, store, 1)
	insertMergeFeed(t, store, 2)
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', 'u', 'u', '', '', '', 0, 0, 1, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (2, 2, 'g2', 'Two', 'u', 'u', '', '', '', 0, 0, 0, 1, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER articles_update_flags_block BEFORE UPDATE OF is_read ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err == nil {
		t.Fatalf("expected update flags error")
	}
}

func TestMergeDuplicateArticlesNormalizedEmpty(t *testing.T) {
	store, _ := newWritableStore(t)
	if _, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"}); err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (1, 1, 'g1', 'One', '', '', '', '', '', 0, 0, 0, 0, 'f')`); err != nil {
		t.Fatalf("insert article error: %v", err)
	}
	if err := store.MergeDuplicateArticles(); err != nil {
		t.Fatalf("MergeDuplicateArticles error: %v", err)
	}
}
