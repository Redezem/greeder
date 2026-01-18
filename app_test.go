package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppBasics(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")

	summaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"- ok"}}]}`))
	}))
	defer summaryServer.Close()

	raindropServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"item":{"_id":7}}`))
	}))
	defer raindropServer.Close()

	os.Setenv("RAINDROP_BASE_URL", raindropServer.URL)
	defer os.Unsetenv("RAINDROP_BASE_URL")

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.summarizer = &Summarizer{baseURL: summaryServer.URL + "/v1", model: "test", client: summaryServer.Client()}
	app.raindrop = NewRaindropClient("token")

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/rss+xml")
		_, _ = w.Write([]byte(rssSample))
	}))
	defer feedServer.Close()

	if err := app.AddFeed(feedServer.URL); err != nil {
		t.Fatalf("AddFeed error: %v", err)
	}
	if err := app.RefreshFeeds(); err != nil {
		t.Fatalf("RefreshFeeds error: %v", err)
	}
	if len(app.articles) == 0 {
		t.Fatalf("expected articles")
	}

	app.openURL = func(string) error { return nil }
	app.emailSender = func(string) error { return nil }

	if err := app.GenerateSummary(); err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
	if err := app.ToggleRead(); err != nil {
		t.Fatalf("ToggleRead error: %v", err)
	}
	if err := app.ToggleStar(); err != nil {
		t.Fatalf("ToggleStar error: %v", err)
	}
	if err := app.OpenSelected(); err != nil {
		t.Fatalf("OpenSelected error: %v", err)
	}
	if err := app.EmailSelected(); err != nil {
		t.Fatalf("EmailSelected error: %v", err)
	}
	if err := app.SaveToRaindrop([]string{"tag"}); err != nil {
		t.Fatalf("SaveToRaindrop error: %v", err)
	}

	app.ToggleFilter()
	app.MoveSelection(1)
	app.MoveSelection(-1)

	if err := app.DeleteSelected(); err != nil {
		t.Fatalf("DeleteSelected error: %v", err)
	}
	if err := app.Undelete(); err != nil {
		t.Fatalf("Undelete error: %v", err)
	}
}

func TestAppErrors(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	if err := app.AddFeed(""); err == nil {
		t.Fatalf("expected add feed error")
	}
	if err := app.OpenSelected(); err != nil {
		t.Fatalf("expected no selected error: %v", err)
	}
	if err := app.GenerateSummary(); err != nil {
		t.Fatalf("expected summary nil error: %v", err)
	}
	app.summarizer = nil
	app.summaryStatus = SummaryNotGenerated
	app.articles = []Article{{ID: 1, Title: "T", URL: "u"}}
	if err := app.GenerateSummary(); err != nil {
		t.Fatalf("expected no config error: %v", err)
	}
	if app.summaryStatus != SummaryNoConfig {
		t.Fatalf("expected no config status")
	}
	app.articles = nil
	if err := app.DeleteSelected(); err != nil {
		t.Fatalf("expected delete no article: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "T", URL: "u"}}
	app.selectedIndex = 0
	if err := app.SaveToRaindrop(nil); err == nil {
		t.Fatalf("expected raindrop not configured")
	}

	app.status = ""
	if err := app.RefreshFeeds(); err != nil {
		t.Fatalf("refresh error: %v", err)
	}
	if !strings.Contains(app.status, "no feeds") {
		t.Fatalf("expected status set")
	}
}

func TestBuildMailto(t *testing.T) {
	article := &Article{Title: "Title", URL: "https://example.com", ContentText: "Body"}
	summary := Summary{ArticleID: 1, Content: "Summary"}
	article.ID = 1
	mailto := buildMailto(article, summary)
	if !strings.Contains(mailto, "mailto:") {
		t.Fatalf("expected mailto")
	}
}
