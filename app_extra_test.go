package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppFiltersAndRefreshErrors(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	feed, err := app.store.InsertFeed(Feed{Title: "Bad", URL: server.URL})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	app.feeds = []Feed{feed}
	if err := app.RefreshFeeds(); err != nil {
		t.Fatalf("RefreshFeeds error: %v", err)
	}
	if !strings.Contains(app.status, "refreshed") {
		t.Fatalf("expected refreshed status")
	}
}

func TestAppGenerateSummaryExistingAndError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
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

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer errorServer.Close()
	app.summarizer = &Summarizer{baseURL: errorServer.URL + "/v1", model: "m", client: errorServer.Client()}
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
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/rss+xml")
		_, _ = w.Write([]byte(rssSample))
	}))
	defer feedServer.Close()

	if err := app.AddFeed(feedServer.URL); err != nil {
		t.Fatalf("AddFeed error: %v", err)
	}
	if err := app.AddFeed(feedServer.URL); err == nil {
		t.Fatalf("expected duplicate add error")
	}
	if err := app.AddFeed("example.com"); err == nil {
		t.Fatalf("expected scheme error")
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
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "T", URL: "https://example.com"}}
	app.selectedIndex = 0

	raindropServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"item":{"_id":5}}`))
	}))
	defer raindropServer.Close()
	app.raindrop = &RaindropClient{baseURL: raindropServer.URL, token: "token", client: raindropServer.Client()}

	if err := app.SaveToRaindrop([]string{"t"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
}

func TestAppToggleReadStarNoArticle(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
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

func TestAppDeleteSelectionAdjust(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
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

func TestAppSaveToRaindropWithSummary(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "T", URL: "https://example.com"}}
	app.selectedIndex = 0
	app.current = Summary{ArticleID: 1, Content: "Summary"}

	raindropServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"item":{"_id":9}}`))
	}))
	defer raindropServer.Close()
	app.raindrop = &RaindropClient{baseURL: raindropServer.URL, token: "token", client: raindropServer.Client()}

	if err := app.SaveToRaindrop([]string{"t"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}
}

func TestNewAppWithServices(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
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
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if err := app.ImportOPML(filepath.Join(root, "missing.opml")); err == nil {
		t.Fatalf("expected import error")
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
	cfg.DBPath = filepath.Join(root, "store.json")
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

	summaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"- ok"}}]}`))
	}))
	defer summaryServer.Close()
	app.summarizer = &Summarizer{baseURL: summaryServer.URL, model: "m", client: summaryServer.Client()}

	orig := storeJSONMarshal
	storeJSONMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("save fail")
	}
	t.Cleanup(func() { storeJSONMarshal = orig })

	if err := app.GenerateSummary(); err == nil {
		t.Fatalf("expected save error")
	}
}

func TestAppDeleteSelectedStoreError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
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
	cfg.DBPath = filepath.Join(root, "store.json")
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
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "T", URL: "https://example.com"}}
	app.selectedIndex = 0

	raindropServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer raindropServer.Close()
	app.raindrop = &RaindropClient{baseURL: raindropServer.URL, token: "token", client: raindropServer.Client()}

	if err := app.SaveToRaindrop(nil); err == nil {
		t.Fatalf("expected save error")
	}
}
