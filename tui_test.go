package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAndRender(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "Title", URL: "https://example.com", ContentText: "Body"}}
	app.store.data.Articles = app.articles
	app.store.data.NextArticleID = 2
	app.selectedIndex = 0

	input := "\n?\nq\n"
	var out bytes.Buffer
	if err := Run(app, strings.NewReader(input), &out); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Commands") {
		t.Fatalf("expected help output")
	}
	if !strings.Contains(output, "Title") {
		t.Fatalf("expected render output")
	}
}

func TestHandleCommandErrors(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}

	cases := []string{"a", "i", "w"}
	for _, cmd := range cases {
		if err := handleCommand(app, cmd, io.Discard); err == nil {
			t.Fatalf("expected error for %s", cmd)
		}
	}

	app.store.data.Articles = []Article{{ID: 1, Title: "A", URL: "u"}}
	app.articles = app.store.data.Articles
	app.selectedIndex = 0
	app.openURL = func(string) error { return nil }
	app.emailSender = func(string) error { return nil }
	app.summarizer = nil

	if err := handleCommand(app, "j", io.Discard); err != nil {
		t.Fatalf("command j error: %v", err)
	}
	if err := handleCommand(app, "k", io.Discard); err != nil {
		t.Fatalf("command k error: %v", err)
	}
	if err := handleCommand(app, "enter", io.Discard); err != nil {
		t.Fatalf("command enter error: %v", err)
	}
	if err := handleCommand(app, "s", io.Discard); err != nil {
		t.Fatalf("command s error: %v", err)
	}
	if err := handleCommand(app, "m", io.Discard); err != nil {
		t.Fatalf("command m error: %v", err)
	}
	if err := handleCommand(app, "o", io.Discard); err != nil {
		t.Fatalf("command o error: %v", err)
	}
	if err := handleCommand(app, "e", io.Discard); err != nil {
		t.Fatalf("command e error: %v", err)
	}
	if err := handleCommand(app, "f", io.Discard); err != nil {
		t.Fatalf("command f error: %v", err)
	}
	if err := handleCommand(app, "d", io.Discard); err != nil {
		t.Fatalf("command d error: %v", err)
	}
	if err := handleCommand(app, "u", io.Discard); err != nil {
		t.Fatalf("command u error: %v", err)
	}
	if err := handleCommand(app, "q", io.Discard); err != nil {
		t.Fatalf("command q error: %v", err)
	}
	if err := handleCommand(app, "b tag", io.Discard); err == nil {
		t.Fatalf("expected raindrop error")
	}
	if err := handleCommand(app, "unknown", io.Discard); err != nil {
		t.Fatalf("expected no error for unknown command")
	}
	if err := handleCommand(app, "   ", io.Discard); err != nil {
		t.Fatalf("expected no error for empty command")
	}
	if err := handleCommand(app, "b", io.Discard); err == nil {
		t.Fatalf("expected raindrop error for empty tags")
	}
}

func TestRenderEdgeCases(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.summaryStatus = SummaryFailed
	app.status = ""
	if output := render(app); !strings.Contains(output, "No article") {
		t.Fatalf("expected no article output")
	}
	app.summaryStatus = SummaryGenerating
	app.articles = []Article{{ID: 1, Title: "T", URL: "u", Content: "c"}}
	app.store.data.Articles = app.articles
	app.selectedIndex = 0
	if output := render(app); !strings.Contains(output, "Generating") {
		t.Fatalf("expected generating output")
	}
	app.summaryStatus = SummaryNoConfig
	if output := render(app); !strings.Contains(output, "not configured") {
		t.Fatalf("expected no config output")
	}
	app.summaryStatus = SummaryGenerated
	app.current = Summary{ArticleID: 1, Content: strings.Repeat("a", 100)}
	if output := render(app); !strings.Contains(output, "...") {
		t.Fatalf("expected truncated output")
	}
	app.current = Summary{ArticleID: 2, Content: "Other"}
	if output := render(app); !strings.Contains(output, "Press Enter") && !strings.Contains(output, "Other") {
		t.Fatalf("expected summary output")
	}
	app.summaryStatus = SummaryNotGenerated
	if output := render(app); !strings.Contains(output, "Press Enter") {
		t.Fatalf("expected prompt output")
	}
}

func TestRunScannerError(t *testing.T) {
	app := &App{store: &Store{}}
	errReader := &failingReader{}
	if err := Run(app, errReader, io.Discard); err == nil {
		t.Fatalf("expected scanner error")
	}
}

func TestRunEmptyInput(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	var out bytes.Buffer
	if err := Run(app, strings.NewReader(""), &out); err != nil {
		t.Fatalf("expected no error for empty input")
	}
}

func TestRunHandleCommandError(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	var out bytes.Buffer
	if err := Run(app, strings.NewReader("a\n"), &out); err == nil {
		t.Fatalf("expected command error")
	}
}

type failingReader struct{}

func (f *failingReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestHeaderLine(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "A"}}
	if line := headerLine(app, 10); line == "" {
		t.Fatalf("expected header line")
	}
}

func TestPadLines(t *testing.T) {
	lines := padLines([]string{"a"}, 3)
	if len(lines) != 3 {
		t.Fatalf("expected padded lines")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("unexpected truncate: %s", got)
	}
	if got := truncate("long long text", 5); got != "lo..." {
		t.Fatalf("unexpected truncate: %s", got)
	}
}

func TestRenderEmptyContentStatus(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.articles = []Article{{ID: 1, Title: "Title", URL: "u"}}
	app.selectedIndex = 0
	app.status = "ready"
	app.summaryStatus = SummaryGenerating
	if output := render(app); !strings.Contains(output, "No content available") || !strings.Contains(output, "ready") {
		t.Fatalf("expected content/status output")
	}
}

func TestRenderSummaryFailed(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	app.summaryStatus = SummaryFailed
	if output := renderRightPane(nil, app); !strings.Contains(strings.Join(output, "\n"), "No article") {
		t.Fatalf("expected no article output")
	}
	app.articles = []Article{{ID: 1, Title: "T"}}
	app.selectedIndex = 0
	output := render(app)
	if !strings.Contains(output, "Failed to generate summary") {
		t.Fatalf("expected failed summary output")
	}
}

func TestRenderMaxList(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(root, "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	for i := 0; i < 10; i++ {
		app.articles = append(app.articles, Article{ID: i + 1, Title: strings.Repeat("T", i+1)})
	}
	app.selectedIndex = 5
	output := render(app)
	if strings.Count(output, "|") < 8 {
		t.Fatalf("expected capped list rendering")
	}
}

func TestHandleCommandRefresh(t *testing.T) {
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
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: feedServer.URL})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	app.feeds = []Feed{feed}
	if err := handleCommand(app, "r", io.Discard); err != nil {
		t.Fatalf("refresh command error: %v", err)
	}
}

func TestHandleCommandSuccesses(t *testing.T) {
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

	raindropServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"item":{"_id":1}}`))
	}))
	defer raindropServer.Close()
	app.raindrop = &RaindropClient{baseURL: raindropServer.URL, token: "token", client: raindropServer.Client()}
	app.openURL = func(string) error { return nil }
	app.emailSender = func(string) error { return nil }

	if err := handleCommand(app, "a "+feedServer.URL, io.Discard); err != nil {
		t.Fatalf("add command error: %v", err)
	}
	if err := handleCommand(app, "down", io.Discard); err != nil {
		t.Fatalf("down command error: %v", err)
	}
	if err := handleCommand(app, "up", io.Discard); err != nil {
		t.Fatalf("up command error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	if err := handleCommand(app, "b tag1,tag2", io.Discard); err != nil {
		t.Fatalf("bookmark command error: %v", err)
	}
	if err := handleCommand(app, "bookmark tag1,tag2", io.Discard); err != nil {
		t.Fatalf("bookmark command error: %v", err)
	}

	opmlPath := filepath.Join(root, "feeds.opml")
	if err := ExportOPML(opmlPath, app.feeds); err != nil {
		t.Fatalf("ExportOPML error: %v", err)
	}
	if err := handleCommand(app, "i "+opmlPath, io.Discard); err != nil {
		t.Fatalf("import command error: %v", err)
	}
	if err := handleCommand(app, "import "+opmlPath, io.Discard); err != nil {
		t.Fatalf("import command error: %v", err)
	}
	outPath := filepath.Join(root, "out.opml")
	if err := handleCommand(app, "w "+outPath, io.Discard); err != nil {
		t.Fatalf("export command error: %v", err)
	}
	if err := handleCommand(app, "export "+outPath, io.Discard); err != nil {
		t.Fatalf("export command error: %v", err)
	}
	if err := handleCommand(app, "refresh", io.Discard); err != nil {
		t.Fatalf("refresh command error: %v", err)
	}
	if err := handleCommand(app, "open", io.Discard); err != nil {
		t.Fatalf("open command error: %v", err)
	}
	if err := handleCommand(app, "email", io.Discard); err != nil {
		t.Fatalf("email command error: %v", err)
	}
	if err := handleCommand(app, "filter", io.Discard); err != nil {
		t.Fatalf("filter command error: %v", err)
	}
	if err := handleCommand(app, "delete", io.Discard); err != nil {
		t.Fatalf("delete command error: %v", err)
	}
	if err := handleCommand(app, "undelete", io.Discard); err != nil {
		t.Fatalf("undelete command error: %v", err)
	}
	if err := handleCommand(app, "quit", io.Discard); err != nil {
		t.Fatalf("quit command error: %v", err)
	}
	if err := handleCommand(app, "help", io.Discard); err != nil {
		t.Fatalf("help command error: %v", err)
	}
}
