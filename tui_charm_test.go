package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func newTUIApp(t *testing.T) *App {
	cfg := DefaultConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "store.db")
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
	program := tea.NewProgram(quitModel{}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	if _, err := defaultRunTeaProgram(program); err != nil {
		t.Fatalf("defaultRunTeaProgram error: %v", err)
	}
}

func TestRefreshCmdAndStatus(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.width = 40
	model.app.refreshPending = true
	model.app.refreshStatus = "Refreshing feeds..."
	model.spinnerIndex = 1
	if status := model.renderStatusBar(40); !strings.Contains(status, "Refreshing feeds") {
		t.Fatalf("expected refresh status")
	}

	msg := refreshCmd(app)()
	result, ok := msg.(refreshResultMsg)
	if !ok || result.err != nil {
		t.Fatalf("expected refresh result")
	}
	updated, _ := model.Update(refreshResultMsg{err: errors.New("fail")})
	model = updated.(tuiModel)
	if model.app.refreshPending {
		t.Fatalf("expected refresh cleared")
	}
	if !strings.Contains(model.app.status, "Refresh failed") {
		t.Fatalf("expected refresh failure status")
	}
}

func TestTUIModelInitView(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	cmd := model.Init()
	if cmd == nil {
		t.Fatalf("expected init command")
	}
	if msg := cmd(); msg == nil {
		t.Fatalf("expected tick message")
	}
	if view := model.View(); view != "Loading..." {
		t.Fatalf("expected loading view")
	}
}

func TestTUIInputPrompt(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.inputMode = inputImportState
	if model.inputPrompt() == "" {
		t.Fatalf("expected import state prompt")
	}
	model.inputMode = inputExportState
	if model.inputPrompt() == "" {
		t.Fatalf("expected export state prompt")
	}
}

func TestWrapTextAndVisibleLines(t *testing.T) {
	lines := wrapText("one two three four", 4)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines")
	}
	empty := wrapText("", 4)
	if len(empty) == 0 {
		t.Fatalf("expected empty wrapped lines")
	}
	if single := wrapText("toolongword", 2); len(single) == 0 {
		t.Fatalf("expected long word wrap")
	}
	if blank := wrapText(" \n\n", 4); len(blank) == 0 {
		t.Fatalf("expected blank wrap")
	}
	if small := wrapText("x", 0); len(small) == 0 {
		t.Fatalf("expected width zero wrap")
	}
	scroll := 100
	visible := visibleLines([]string{"a", "b", "c", "d"}, 2, &scroll)
	if len(visible) != 2 || visible[0] != "c" {
		t.Fatalf("expected clamped visible lines")
	}
	scroll = -2
	visible = visibleLines([]string{"a", "b", "c"}, 2, &scroll)
	if visible[0] != "a" {
		t.Fatalf("expected negative scroll clamp")
	}
	visible = visibleLines([]string{"a"}, 2, &scroll)
	if len(visible) != 2 {
		t.Fatalf("expected padded visible lines")
	}
	visible = visibleLines([]string{"a"}, 0, &scroll)
	if len(visible) != 0 {
		t.Fatalf("expected empty visible lines")
	}
}

func TestRenderDetailsScrollOrder(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "UniqueTitle", ContentText: strings.Repeat("word ", 40)}}
	app.selectedIndex = 0
	app.summaryStatus = SummaryGenerated
	app.current = Summary{ArticleID: 1, Content: strings.Repeat("sum ", 20)}
	model := newTUIModel(app)
	output := model.renderDetails(40, 20)
	summaryPos := strings.Index(output, "Summary")
	contentPos := strings.Index(output, "Content")
	if summaryPos == -1 || contentPos == -1 || summaryPos > contentPos {
		t.Fatalf("expected summary before content")
	}
	if !strings.Contains(output, "Scroll") {
		t.Fatalf("expected scroll indicator")
	}
	model.detailScroll = 100
	output = model.renderDetails(40, 10)
	if strings.Contains(output, "UniqueTitle") {
		t.Fatalf("expected title scrolled out")
	}
	_ = model.renderDetails(3, 10)
	_ = model.renderDetails(40, 1)
}

func TestDetailScrollKeys(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "A"}}
	app.selectedIndex = 0
	model := newTUIModel(app)
	model.detailScroll = 5
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = updated.(tuiModel)
	if model.detailScroll != 2 {
		t.Fatalf("expected scroll up")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(tuiModel)
	if model.detailScroll != 5 {
		t.Fatalf("expected scroll down")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = updated.(tuiModel)
	if model.detailScroll != 0 {
		t.Fatalf("expected scroll home")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = updated.(tuiModel)
	if model.detailScroll == 0 {
		t.Fatalf("expected scroll end")
	}

	model.adjustDetailScroll(0)
	model.detailScroll = 1
	model.adjustDetailScroll(-10)
	if model.detailScroll != 0 {
		t.Fatalf("expected clamped scroll")
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

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("I")})
	model = updated.(tuiModel)
	if model.inputMode != inputImportState {
		t.Fatalf("expected import state mode")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(tuiModel)
	if model.inputMode != inputNone {
		t.Fatalf("expected import state cancelled")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	model = updated.(tuiModel)
	if model.inputMode != inputExportState {
		t.Fatalf("expected export state mode")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(tuiModel)
	if model.inputMode != inputNone {
		t.Fatalf("expected export state cancelled")
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

	statePath := filepath.Join(t.TempDir(), "state.json")
	model = model.startInput(inputExportState, "Export state")
	model.input.SetValue(statePath)
	model = model.commitInput()
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state export file")
	}

	model = model.startInput(inputImportState, "Import state")
	model.input.SetValue(statePath)
	model = model.commitInput()
	if !strings.Contains(model.app.status, "State imported") {
		t.Fatalf("expected state import status")
	}

	model = model.startInput(inputImportState, "Import state")
	model.input.SetValue(filepath.Join(t.TempDir(), "missing.json"))
	model = model.commitInput()
	if !strings.Contains(model.app.status, "State import failed") {
		t.Fatalf("expected state import failure")
	}

	model = model.startInput(inputExportState, "Export state")
	model.input.SetValue(t.TempDir())
	model = model.commitInput()
	if !strings.Contains(model.app.status, "State export failed") {
		t.Fatalf("expected state export failure")
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
	app.summarizer = nil
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
		{Type: tea.KeyRunes, Runes: []rune("I")},
		{Type: tea.KeyRunes, Runes: []rune("E")},
		{Type: tea.KeyRunes, Runes: []rune("b")},
		{Type: tea.KeyRunes, Runes: []rune("s")},
		{Type: tea.KeyRunes, Runes: []rune("m")},
		{Type: tea.KeyRunes, Runes: []rune("o")},
		{Type: tea.KeyRunes, Runes: []rune("e")},
		{Type: tea.KeyRunes, Runes: []rune("d")},
		{Type: tea.KeyRunes, Runes: []rune("u")},
		{Type: tea.KeyCtrlU},
		{Type: tea.KeyCtrlD},
		{Type: tea.KeyPgUp},
		{Type: tea.KeyPgDown},
		{Type: tea.KeyHome},
		{Type: tea.KeyEnd},
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

func TestTUISpinnerTick(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.spinnerFrames = []string{"-", "+"}
	updated, cmd := model.Update(spinnerTickMsg{})
	next := updated.(tuiModel)
	if next.spinnerIndex != 1 {
		t.Fatalf("expected spinner index advance")
	}
	if cmd == nil {
		t.Fatalf("expected tick command")
	}
	if msg := cmd(); msg == nil {
		t.Fatalf("expected tick message")
	}
}

func TestSummaryCmdSuccess(t *testing.T) {
	summarizer := &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}),
	}
	cmd := summaryCmd(7, "Title", "Content", summarizer)
	msg := cmd()
	result := msg.(summaryResultMsg)
	if result.articleID != 7 || result.err != nil || result.summaryText == "" {
		t.Fatalf("expected summary result success")
	}
}

func TestTUISummaryResultHandling(t *testing.T) {
	app := newTUIApp(t)
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.summaryPending[articles[0].ID] = true
	model := newTUIModel(app)

	msg := summaryResultMsg{articleID: articles[0].ID, summaryText: "Summary", model: "m"}
	updated, _ := model.Update(msg)
	updatedModel := updated.(tuiModel)
	if updatedModel.app.summaryStatus != SummaryGenerated {
		t.Fatalf("expected summary generated")
	}
	if _, ok := updatedModel.app.store.FindSummary(articles[0].ID); !ok {
		t.Fatalf("expected summary stored")
	}
}

func TestTUIBatchQueue(t *testing.T) {
	app := newTUIApp(t)
	model := newTUIModel(app)
	model.queueMissingSummaries()
	if model.app.summaryStatus != SummaryNoConfig {
		t.Fatalf("expected no config summary")
	}

	app.summarizer = &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}),
	}
	model.queueMissingSummaries()
	if model.app.status != "No missing summaries" {
		t.Fatalf("expected no missing summaries")
	}

	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{
		{GUID: "1", Title: "One", URL: "u1"},
		{GUID: "2", Title: "Two", URL: "u2"},
	})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	if _, err := app.store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "Existing"}); err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	app.articles = app.store.SortedArticles()

	model.queueMissingSummaries()
	if len(model.summaryQueue) != 1 || !model.batchActive {
		t.Fatalf("expected batch queue")
	}
	if cmd := model.startNextBatchSummary(); cmd == nil {
		t.Fatalf("expected batch command")
	}
}

func TestTUISummaryResultErrorHandling(t *testing.T) {
	app := newTUIApp(t)
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.summaryPending[articles[0].ID] = true
	model := newTUIModel(app)

	msg := summaryResultMsg{articleID: articles[0].ID, err: errors.New("fail")}
	updated, _ := model.Update(msg)
	updatedModel := updated.(tuiModel)
	if updatedModel.app.summaryStatus != SummaryFailed {
		t.Fatalf("expected summary failed")
	}
	if !strings.Contains(updatedModel.app.status, "Summary failed") {
		t.Fatalf("expected failure status")
	}
}

func TestTUISummarySaveErrorHandling(t *testing.T) {
	app := newTUIApp(t)
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	app.summaryPending[articles[0].ID] = true
	if err := app.store.db.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	model := newTUIModel(app)
	msg := summaryResultMsg{articleID: articles[0].ID, summaryText: "Summary", model: "m"}
	updated, _ := model.Update(msg)
	updatedModel := updated.(tuiModel)
	if !strings.Contains(updatedModel.app.status, "Summary save failed") {
		t.Fatalf("expected save failure status")
	}
}

func TestTUIStartSummaryBranches(t *testing.T) {
	app := newTUIApp(t)
	app.summarizer = &Summarizer{
		baseURL: "http://example.test",
		model:   "m",
		client:  clientForResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}),
	}
	feed, err := app.store.InsertFeed(Feed{Title: "Feed", URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("InsertFeed error: %v", err)
	}
	articles, err := app.store.InsertArticles(feed, []Article{{GUID: "1", Title: "Title", URL: "u"}})
	if err != nil {
		t.Fatalf("InsertArticles error: %v", err)
	}
	app.articles = app.store.SortedArticles()
	app.selectedIndex = 0
	model := newTUIModel(app)

	if _, err := app.store.UpsertSummary(Summary{ArticleID: articles[0].ID, Content: "Existing"}); err != nil {
		t.Fatalf("UpsertSummary error: %v", err)
	}
	if cmd := model.startSummary(articles[0]); cmd != nil {
		t.Fatalf("expected no cmd for existing summary")
	}
	if model.app.summaryStatus != SummaryGenerated {
		t.Fatalf("expected generated summary status")
	}

	if _, err := app.store.db.Exec(`DELETE FROM summaries`); err != nil {
		t.Fatalf("delete summaries error: %v", err)
	}
	model.app.summaryPending = map[int]bool{}
	if cmd := model.startSummary(articles[0]); cmd == nil {
		t.Fatalf("expected summary cmd")
	}
	if model.app.summaryStatus != SummaryGenerating {
		t.Fatalf("expected generating status")
	}

	model.app.summaryPending[articles[0].ID] = true
	if cmd := model.startSummary(articles[0]); cmd != nil {
		t.Fatalf("expected no cmd for pending summary")
	}
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
	if out := model.renderDetails(50, 20); !strings.Contains(out, "Summary") {
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
	model.spinnerFrames = []string{"*"}
	model.app.summaryPending[1] = true
	if out := model.renderList(8); !strings.Contains(out, "*") {
		t.Fatalf("expected spinner")
	}
}

func TestTUIRenderDetailsStatuses(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "Title", ContentText: "Body"}}
	app.selectedIndex = 0
	model := newTUIModel(app)

	app.summaryStatus = SummaryGenerating
	if out := model.renderDetails(40, 20); !strings.Contains(out, "Generating") {
		t.Fatalf("expected generating")
	}
	app.summaryStatus = SummaryNoConfig
	if out := model.renderDetails(40, 20); !strings.Contains(out, "LM_BASE_URL") {
		t.Fatalf("expected no config")
	}
	app.summaryStatus = SummaryFailed
	if out := model.renderDetails(40, 20); !strings.Contains(out, "failed") {
		t.Fatalf("expected failed")
	}
	app.summaryStatus = SummaryGenerated
	app.current = Summary{}
	if out := model.renderDetails(40, 20); !strings.Contains(out, "No summary") {
		t.Fatalf("expected no summary")
	}
	app.summaryStatus = SummaryNotGenerated
	if out := model.renderDetails(40, 20); !strings.Contains(out, "Press Enter") {
		t.Fatalf("expected prompt")
	}
}

func TestTUIRenderDetailsSmallHeight(t *testing.T) {
	app := newTUIApp(t)
	app.articles = []Article{{ID: 1, Title: "Title", ContentText: "Body"}}
	app.selectedIndex = 0
	model := newTUIModel(app)
	if out := model.renderDetails(40, 8); out == "" {
		t.Fatalf("expected details output")
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

	if formatLocalTime(time.Time{}) != "Unknown" {
		t.Fatalf("expected unknown time")
	}
	if valueOrFallback("", "x") != "x" {
		t.Fatalf("expected fallback value")
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
	if out := model.renderDetails(40, 20); !strings.Contains(out, "Select an article") {
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
