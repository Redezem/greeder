package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func newTUIApp(t *testing.T) *App {
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "store.json")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp error: %v", err)
	}
	return app
}

func TestRunTUI(t *testing.T) {
	app := newTUIApp(t)
	origNew := teaNewProgram
	origRun := runTeaProgram
	t.Cleanup(func() {
		teaNewProgram = origNew
		runTeaProgram = origRun
	})

	teaNewProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m)
	}
	runTeaProgram = func(program *tea.Program) (tea.Model, error) {
		return nil, nil
	}

	if err := RunTUI(app); err != nil {
		t.Fatalf("RunTUI error: %v", err)
	}
}

type quitModel struct{}

func (quitModel) Init() tea.Cmd {
	return tea.Quit
}

func (quitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return quitModel{}, tea.Quit
}

func (quitModel) View() string {
	return ""
}

func TestDefaultRunTeaProgram(t *testing.T) {
	program := tea.NewProgram(quitModel{}, tea.WithoutRenderer())
	if _, err := defaultRunTeaProgram(program); err != nil {
		t.Fatalf("defaultRunTeaProgram error: %v", err)
	}
}

func TestTUIModelInitView(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	if model.Init() != nil {
		t.Fatalf("expected nil init")
	}
	if view := model.View(); view != "Loading..." {
		t.Fatalf("expected loading view")
	}
}

func TestTUIWindowHelpAndInput(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(tuiModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = updated.(tuiModel)
	if !model.showHelp {
		t.Fatalf("expected help mode")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(tuiModel)
	if model.showHelp {
		t.Fatalf("expected help dismissed")
	}
	model.showHelp = true
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updated.(tuiModel)
	if model.showHelp {
		t.Fatalf("expected help dismissed with q")
	}

	model = model.startInput(inputAddFeed, "Add")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(tuiModel)
	if model.inputMode != inputNone || model.input.Value() != "" {
		t.Fatalf("expected input cancelled")
	}
}

func TestTUIInputCharUpdate(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model = model.startInput(inputAddFeed, "Add")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(tuiModel)
	if model.input.Value() == "" {
		t.Fatalf("expected input char")
	}
}

func TestTUIInputCommitFlows(t *testing.T) {
	app := newTUIApp(t)
	app.fetcher = &FeedFetcher{client: clientForResponse(http.StatusOK, rssSample, map[string]string{"content-type": "application/rss+xml"})}
	model := newTUIModel(app)

	model = model.startInput(inputAddFeed, "Add")
	model.input.SetValue("http://example.test/rss")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if len(model.app.feeds) == 0 {
		t.Fatalf("expected feed added")
	}

	opmlPath := filepath.Join(t.TempDir(), "feeds.opml")
	if err := ExportOPML(opmlPath, []Feed{{Title: "Feed", URL: "http://example.test/rss"}}); err != nil {
		t.Fatalf("ExportOPML error: %v", err)
	}
	model = model.startInput(inputImportOPML, "Import")
	model.input.SetValue(opmlPath)
	model = model.commitInput()
	if len(model.app.feeds) == 0 {
		t.Fatalf("expected import feeds")
	}

	model = model.startInput(inputImportOPML, "Import")
	model.input.SetValue(filepath.Join(t.TempDir(), "missing.opml"))
	model = model.commitInput()
	if !strings.Contains(model.app.status, "Import failed") {
		t.Fatalf("expected import failure")
	}

	exportPath := filepath.Join(t.TempDir(), "out.opml")
	model = model.startInput(inputExportOPML, "Export")
	model.input.SetValue(exportPath)
	model = model.commitInput()
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("expected export file")
	}

	model.app.articles = []Article{{ID: 1, Title: "A", URL: "https://example.com"}}
	model.app.selectedIndex = 0
	model = model.startInput(inputBookmarkTags, "Tags")
	model.input.SetValue("tag1, tag2")
	model = model.commitInput()
	if !strings.Contains(model.app.status, "Bookmark failed") {
		t.Fatalf("expected bookmark failure")
	}

	model = model.startInput(inputAddFeed, "Add")
	model.input.SetValue("http://[::1")
	model = model.commitInput()
	if !strings.Contains(model.app.status, "Add feed failed") {
		t.Fatalf("expected add feed failure")
	}

	exportDir := t.TempDir()
	model = model.startInput(inputExportOPML, "Export")
	model.input.SetValue(exportDir)
	model = model.commitInput()
	if !strings.Contains(model.app.status, "Export failed") {
		t.Fatalf("expected export failure")
	}

	model = model.startInput(inputImportOPML, "Import")
	model.input.SetValue(" ")
	model = model.commitInput()
	if !strings.Contains(model.app.status, "Input cancelled") {
		t.Fatalf("expected input cancelled")
	}
}

func TestTUIUpdateKeys(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}
	app.openURL = func(string) error { return nil }
	app.emailSender = func(string) error { return nil }
	model := newTUIModel(app)

	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("j")},
		{Type: tea.KeyRunes, Runes: []rune("k")},
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune("f")},
		{Type: tea.KeyRunes, Runes: []rune("r")},
		{Type: tea.KeyRunes, Runes: []rune("a")},
		{Type: tea.KeyRunes, Runes: []rune("i")},
		{Type: tea.KeyRunes, Runes: []rune("w")},
		{Type: tea.KeyRunes, Runes: []rune("b")},
		{Type: tea.KeyRunes, Runes: []rune("s")},
		{Type: tea.KeyRunes, Runes: []rune("m")},
		{Type: tea.KeyRunes, Runes: []rune("o")},
		{Type: tea.KeyRunes, Runes: []rune("e")},
		{Type: tea.KeyRunes, Runes: []rune("d")},
		{Type: tea.KeyRunes, Runes: []rune("u")},
	}
	for _, key := range keys {
		updated, _ := model.Update(key)
		model = updated.(tuiModel)
	}
	if model.app.summaryStatus != SummaryNotGenerated {
		t.Fatalf("expected not generated summary")
	}
}

func TestTUIUpdateActionKeys(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	keys := []string{"a", "i", "w", "b", "s", "m", "o", "e", "d", "u", "y", "G"}
	for _, key := range keys {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		model = updated.(tuiModel)
		model.inputMode = inputNone
		model.input.SetValue("")
	}
}

func TestTUIUpdateQuitAndArrows(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}
	model := newTUIModel(app)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updated.(tuiModel)
	if cmd == nil || model.app != app {
		return
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	_ = updated.(tuiModel)
}

func TestTUIUpdateUnknownMsg(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	type dummyMsg struct{}
	_, _ = model.Update(dummyMsg{})
}

func TestTUIRenderFunctions(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "Title", ContentText: "Body", IsStarred: true}, {ID: 2, Title: "Read", IsRead: true}}
	app.filter = FilterAll
	app.selectedIndex = 0
	app.summaryStatus = SummaryGenerated
	app.current = Summary{Content: "Summary"}
	model := newTUIModel(app)
	model.width = 80
	model.height = 24

	if out := model.renderLayout(); !strings.Contains(out, "Greeder") {
		t.Fatalf("expected layout")
	}
	if out := model.renderList(30); !strings.Contains(out, "★") {
		t.Fatalf("expected list flags")
	}
	if out := model.renderDetails(50); !strings.Contains(out, "Summary") {
		t.Fatalf("expected details")
	}
	if out := model.renderStatusBar(80); !strings.Contains(out, "Press / for help") {
		t.Fatalf("expected tooltip")
	}
	if out := model.renderHelpOverlay(); !strings.Contains(out, "Quick Commands") {
		t.Fatalf("expected help overlay")
	}
	model = model.startInput(inputAddFeed, "Add")
	if out := model.renderInputOverlay(""); !strings.Contains(out, "Add Feed") {
		t.Fatalf("expected input overlay")
	}
	if out := model.renderList(30); out == "" {
		t.Fatalf("expected list output")
	}
}

func TestTUIRenderLayoutSmallWidth(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.width = 40
	model.height = 10
	if out := model.renderLayout(); out == "" {
		t.Fatalf("expected layout")
	}
}

func TestTUIRenderListMinHeight(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "A"}}
	model := newTUIModel(app)
	model.height = 8
	if out := model.renderList(30); out == "" {
		t.Fatalf("expected list")
	}
}

func TestTUIRenderDetailsStatuses(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "Title", ContentText: "Body"}}
	app.selectedIndex = 0
	model := newTUIModel(app)

	app.summaryStatus = SummaryGenerating
	if out := model.renderDetails(40); !strings.Contains(out, "Generating") {
		t.Fatalf("expected generating")
	}
	app.summaryStatus = SummaryNoConfig
	if out := model.renderDetails(40); !strings.Contains(out, "LM_BASE_URL") {
		t.Fatalf("expected no config")
	}
	app.summaryStatus = SummaryFailed
	if out := model.renderDetails(40); !strings.Contains(out, "failed") {
		t.Fatalf("expected failed")
	}
	app.summaryStatus = SummaryGenerated
	app.current = Summary{}
	if out := model.renderDetails(40); !strings.Contains(out, "No summary") {
		t.Fatalf("expected no summary")
	}
	app.summaryStatus = SummaryNotGenerated
	if out := model.renderDetails(40); !strings.Contains(out, "Press Enter") {
		t.Fatalf("expected prompt")
	}
}

func TestTUIViewStates(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.width = 80
	model.height = 24
	model.showHelp = true
	if out := model.View(); !strings.Contains(out, "Quick Commands") {
		t.Fatalf("expected help view")
	}
	model.showHelp = false
	model.inputMode = inputImportOPML
	model.input.Focus()
	if out := model.View(); !strings.Contains(out, "Import OPML") {
		t.Fatalf("expected input view")
	}
	model.inputMode = inputNone
	model.app.articles = []Article{{ID: 1, Title: "A"}}
	model.app.selectedIndex = 0
	if out := model.View(); !strings.Contains(out, "Greeder") {
		t.Fatalf("expected base view")
	}
}

func TestTUIHelpers(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)

	model.inputMode = inputAddFeed
	if tip := model.tooltipText(); !strings.Contains(tip, "Enter") {
		t.Fatalf("expected input tooltip")
	}
	model.inputMode = inputNone
	if tip := model.tooltipText(); !strings.Contains(tip, "/") {
		t.Fatalf("expected help tooltip")
	}

	app.summaryStatus = SummaryGenerated
	app.current = Summary{Content: "Summary"}
	if model.summaryText() != "Summary" {
		t.Fatalf("expected summary text")
	}
	app.current = Summary{}
	if !strings.Contains(model.summaryText(), "No summary") {
		t.Fatalf("expected no summary text")
	}

	if clamp(1, 2, 3) != 2 {
		t.Fatalf("expected clamp min")
	}
	if clamp(5, 2, 3) != 3 {
		t.Fatalf("expected clamp max")
	}
}

func TestTUIRenderListEmpty(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.height = 10
	if out := model.renderList(30); !strings.Contains(out, "No articles") {
		t.Fatalf("expected empty list")
	}
}

func TestTUIRenderDetailsNoArticle(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	if out := model.renderDetails(40); !strings.Contains(out, "Select an article") {
		t.Fatalf("expected no article")
	}
}

func TestTUIRenderStatusBarStates(t *testing.T) {
	app := newTUIApp(t)
	app.status = ""
	model := newTUIModel(app)
	out := ansi.Strip(model.renderStatusBar(80))
	if !strings.Contains(out, "Ready") {
		t.Fatalf("expected ready status, got %q", out)
	}
	app.status = "Status"
	model.inputMode = inputExportOPML
	out = ansi.Strip(model.renderStatusBar(80))
	if !strings.Contains(out, "Enter to confirm") {
		t.Fatalf("expected input hint, got %q", out)
	}
}

func TestTUIInputPromptValues(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.inputMode = inputAddFeed
	if model.inputPrompt() != "Add Feed" {
		t.Fatalf("expected add feed prompt")
	}
	model.inputMode = inputImportOPML
	if model.inputPrompt() != "Import OPML" {
		t.Fatalf("expected import prompt")
	}
	model.inputMode = inputExportOPML
	if model.inputPrompt() != "Export OPML" {
		t.Fatalf("expected export prompt")
	}
	model.inputMode = inputBookmarkTags
	if model.inputPrompt() != "Bookmark Tags" {
		t.Fatalf("expected bookmark prompt")
	}
	model.inputMode = inputNone
	if model.inputPrompt() != "Input" {
		t.Fatalf("expected default prompt")
	}
}

func TestTUIRenderStatusBarPadding(t *testing.T) {
	app := newTUIApp(t)
	app.status = strings.Repeat("x", 200)
	model := newTUIModel(app)
	if out := model.renderStatusBar(10); out == "" {
		t.Fatalf("expected status output")
	}
}

func TestTUIViewWithStatus(t *testing.T) {
	app := newTUIApp(t)
	app.status = "Ready"
	app.articles = []Article{{ID: 1, Title: "A", ContentText: "Body"}}
	app.selectedIndex = 0
	model := newTUIModel(app)
	model.width = 80
	model.height = 24
	if out := model.View(); !strings.Contains(out, "Ready") {
		t.Fatalf("expected status")
	}
}
