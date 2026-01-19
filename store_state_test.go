package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreExportImportState(t *testing.T) {
	store := newTestStore(t)
	feed := Feed{Title: "Feed", URL: "https://example.com/rss"}
	savedFeed, err := store.InsertFeed(feed)
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := store.InsertArticles(savedFeed, []Article{
		{GUID: "one", Title: "One", URL: "https://example.com/one"},
		{GUID: "two", Title: "Two", URL: "https://example.com/two"},
	})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "Summary", Model: "m"}); err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	if err := store.SaveToRaindrop(articles[0].ID, 42, []string{"tag"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
	if _, err := store.DeleteArticle(articles[1].ID); err != nil {
		t.Fatalf("DeleteArticle error: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "state.json")
	if err := store.ExportState(exportPath); err != nil {
		t.Fatalf("ExportState error: %v", err)
	}

	other := newTestStore(t)
	if err := other.ImportState(exportPath); err != nil {
		t.Fatalf("ImportState error: %v", err)
	}
	if len(other.Feeds()) != 1 {
		t.Fatalf("expected feeds imported")
	}
	if len(other.Articles()) != 1 {
		t.Fatalf("expected articles imported")
	}
	if len(other.Summaries()) != 1 {
		t.Fatalf("expected summaries imported")
	}
	if len(other.Saved()) != 1 {
		t.Fatalf("expected saved imported")
	}
	if len(other.Deleted()) != 1 {
		t.Fatalf("expected deleted imported")
	}
}

func TestStoreImportStateErrors(t *testing.T) {
	store := newTestStore(t)
	if err := store.ExportState(""); err == nil {
		t.Fatalf("expected export path error")
	}
	if err := store.ImportState(""); err == nil {
		t.Fatalf("expected import path error")
	}

	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected import parse error")
	}

	payload, err := json.Marshal(ExportState{Version: 99, ExportedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected unsupported version error")
	}

	orig := tagsMarshal
	tagsMarshal = func(any) ([]byte, error) { return nil, errors.New("tags fail") }
	t.Cleanup(func() { tagsMarshal = orig })
	state := ExportState{
		Version:    exportStateVersion,
		ExportedAt: time.Now().UTC(),
		Saved: []Saved{
			{ArticleID: 1, RaindropID: 2, Tags: []string{"a"}, SavedAt: time.Now().UTC()},
		},
	}
	payload, err = json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected tags marshal error")
	}
}

func TestStoreExportStateMarshalError(t *testing.T) {
	store := newTestStore(t)
	orig := stateMarshalIndent
	stateMarshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal") }
	t.Cleanup(func() { stateMarshalIndent = orig })
	if err := store.ExportState(filepath.Join(t.TempDir(), "state.json")); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestStoreImportStateReadError(t *testing.T) {
	store := newTestStore(t)
	if err := store.ImportState(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatalf("expected read error")
	}
}

func TestStoreImportStateBeginTxError(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "state.json")
	writeStateFile(t, path, ExportState{Version: exportStateVersion})
	orig := beginTx
	beginTx = func(*sql.DB) (*sql.Tx, error) { return nil, errors.New("begin") }
	t.Cleanup(func() { beginTx = orig })
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected begin error")
	}
}

func TestStoreImportStateDeleteErrors(t *testing.T) {
	cases := []struct {
		name  string
		table string
	}{
		{"summaries", "summaries"},
		{"saved", "saved"},
		{"deleted", "deleted"},
		{"feeds", "feeds"},
	}
	for _, testCase := range cases {
		store := newTestStore(t)
		path := filepath.Join(t.TempDir(), "state.json")
		writeStateFile(t, path, ExportState{Version: exportStateVersion})
		if _, err := store.db.Exec("DROP TABLE " + testCase.table); err != nil {
			t.Fatalf("drop %s error: %v", testCase.table, err)
		}
		if err := store.ImportState(path); err == nil {
			t.Fatalf("expected delete error for %s", testCase.name)
		}
	}
}

func TestStoreImportStateDeleteArticlesError(t *testing.T) {
	store := newTestStore(t)
	feed, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.InsertArticles(feed, []Article{{GUID: "g1", Title: "A", URL: "u"}}); err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	writeStateFile(t, path, ExportState{Version: exportStateVersion})
	if _, err := store.db.Exec(`CREATE TRIGGER articles_delete_block BEFORE DELETE ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected delete articles error")
	}
}

func TestStoreImportStateInsertErrors(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "state.json")
	state := ExportState{
		Version: exportStateVersion,
		Feeds:   []Feed{{ID: 1, Title: "Feed", URL: "https://example.com/rss"}},
		Articles: []Article{
			{ID: 1, FeedID: 1, GUID: "g1", Title: "A", URL: "u"},
		},
		Summaries: []Summary{
			{ID: 1, ArticleID: 1, Content: "S"},
		},
		Saved: []Saved{
			{ArticleID: 1, RaindropID: 2, Tags: []string{"a"}},
		},
		Deleted: []Deleted{
			{
				FeedID:    1,
				GUID:      "g2",
				DeletedAt: time.Now().UTC(),
				Article:   Article{Title: "T", URL: "u"},
			},
		},
	}

	writeStateFile(t, path, state)
	if _, err := store.db.Exec(`CREATE TRIGGER feeds_block BEFORE INSERT ON feeds BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected feed insert error")
	}

	store = newTestStore(t)
	writeStateFile(t, path, state)
	if _, err := store.db.Exec(`CREATE TRIGGER articles_block BEFORE INSERT ON articles BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected article insert error")
	}

	store = newTestStore(t)
	writeStateFile(t, path, state)
	if _, err := store.db.Exec(`CREATE TRIGGER summaries_block BEFORE INSERT ON summaries BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected summary insert error")
	}

	store = newTestStore(t)
	writeStateFile(t, path, state)
	if _, err := store.db.Exec(`CREATE TRIGGER saved_block BEFORE INSERT ON saved BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected saved insert error")
	}

	store = newTestStore(t)
	writeStateFile(t, path, state)
	if _, err := store.db.Exec(`CREATE TRIGGER deleted_block BEFORE INSERT ON deleted BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected deleted insert error")
	}
}

func TestStoreImportStateCommitError(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "state.json")
	writeStateFile(t, path, ExportState{Version: exportStateVersion})
	orig := commitTx
	commitTx = func(*sql.Tx) error { return errors.New("commit") }
	t.Cleanup(func() { commitTx = orig })
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected commit error")
	}
}

func TestStoreImportStateBaseURLFallback(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "state.json")
	state := ExportState{
		Version: exportStateVersion,
		Feeds:   []Feed{{ID: 1, Title: "Feed", URL: "https://example.com/rss"}},
		Articles: []Article{
			{ID: 1, FeedID: 1, GUID: "g1", Title: "A", URL: "", BaseURL: ""},
		},
		Deleted: []Deleted{
			{
				FeedID:    1,
				GUID:      "g2",
				DeletedAt: time.Now().UTC(),
				Article:   Article{Title: "T", URL: "", BaseURL: ""},
			},
		},
	}
	writeStateFile(t, path, state)
	if err := store.ImportState(path); err != nil {
		t.Fatalf("ImportState error: %v", err)
	}
	var sources int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM article_sources`).Scan(&sources); err != nil {
		t.Fatalf("sources count error: %v", err)
	}
	if sources != 1 {
		t.Fatalf("expected article source inserted")
	}
}

func TestStoreImportStateDeleteFeedsError(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "state.json")
	writeStateFile(t, path, ExportState{Version: exportStateVersion})
	if _, err := store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"}); err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER feeds_delete_block BEFORE DELETE ON feeds BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
		t.Fatalf("trigger error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected delete feeds error")
	}
}

func TestStoreImportStateArticleSourcesError(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(t.TempDir(), "state.json")
	state := ExportState{
		Version: exportStateVersion,
		Feeds:   []Feed{{ID: 1, Title: "Feed", URL: "https://example.com/rss"}},
		Articles: []Article{
			{ID: 1, FeedID: 1, GUID: "g1", Title: "A", URL: "https://example.com/a"},
		},
	}
	writeStateFile(t, path, state)
	if _, err := store.db.Exec(`DROP TABLE article_sources`); err != nil {
		t.Fatalf("drop article_sources error: %v", err)
	}
	if err := store.ImportState(path); err == nil {
		t.Fatalf("expected article_sources insert error")
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	return store
}

func writeStateFile(t *testing.T, path string, state ExportState) {
	t.Helper()
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
}
