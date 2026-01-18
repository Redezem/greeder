package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type inputMode int

const (
	inputNone inputMode = iota
	inputAddFeed
	inputImportOPML
	inputExportOPML
	inputBookmarkTags
)

type spinnerTickMsg struct{}

type summaryResultMsg struct {
	articleID   int
	summaryText string
	model       string
	err         error
}

type tuiModel struct {
	app           *App
	width         int
	height        int
	input         textinput.Model
	inputMode     inputMode
	showHelp      bool
	statusHint    string
	summaryQueue  []Article
	batchActive   bool
	spinnerIndex  int
	spinnerFrames []string
}

var (
	teaNewProgram  = tea.NewProgram
	runTeaProgram  = defaultRunTeaProgram
	programExecute = func(program *tea.Program) (tea.Model, error) { return program.Run() }
	runProgram     = func(program *tea.Program) (tea.Model, error) { return programExecute(program) }
	programRun     = func(program *tea.Program) (tea.Model, error) { return runProgram(program) }
)

func defaultRunTeaProgram(program *tea.Program) (tea.Model, error) {
	return programRun(program)
}

func RunTUI(app *App) error {
	model := newTUIModel(app)
	program := teaNewProgram(model, tea.WithAltScreen())
	_, err := runTeaProgram(program)
	return err
}

func newTUIModel(app *App) tuiModel {
	input := textinput.New()
	input.Placeholder = ""
	input.CharLimit = 256
	input.Width = 50
	input.Prompt = "> "
	return tuiModel{
		app:           app,
		input:         input,
		spinnerFrames: []string{"|", "/", "-", "\\"},
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case spinnerTickMsg:
		if len(m.spinnerFrames) > 0 {
			m.spinnerIndex = (m.spinnerIndex + 1) % len(m.spinnerFrames)
		}
		return m, tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
			return spinnerTickMsg{}
		})
	case summaryResultMsg:
		delete(m.app.summaryPending, msg.articleID)
		if msg.err != nil {
			if selected := m.app.SelectedArticle(); selected != nil && selected.ID == msg.articleID {
				m.app.summaryStatus = SummaryFailed
			}
			m.app.status = "Summary failed: " + msg.err.Error()
		} else {
			summary := Summary{
				ArticleID:   msg.articleID,
				Content:     msg.summaryText,
				Model:       msg.model,
				GeneratedAt: time.Now().UTC(),
			}
			stored, err := m.app.store.UpsertSummary(summary)
			if err != nil {
				m.app.status = "Summary save failed: " + err.Error()
			} else if selected := m.app.SelectedArticle(); selected != nil && selected.ID == msg.articleID {
				m.app.current = stored
				m.app.summaryStatus = SummaryGenerated
			}
		}
		return m, m.startNextBatchSummary()
	case tea.KeyMsg:
		key := msg.String()
		if m.showHelp {
			if key == "/" || key == "esc" || key == "q" {
				m.showHelp = false
			}
			return m, nil
		}
		if m.inputMode != inputNone {
			var cmd tea.Cmd
			switch key {
			case "esc":
				m.inputMode = inputNone
				m.input.Blur()
				m.input.SetValue("")
				return m, nil
			case "enter":
				m = m.commitInput()
				return m, nil
			}
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		switch key {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "/":
			m.showHelp = true
		case "j", "down":
			m.app.MoveSelection(1)
		case "k", "up":
			m.app.MoveSelection(-1)
		case "enter":
			if article := m.app.SelectedArticle(); article != nil {
				return m, m.startSummary(*article)
			}
		case "r":
			_ = m.app.RefreshFeeds()
		case "a":
			m = m.startInput(inputAddFeed, "Add feed URL")
		case "i":
			m = m.startInput(inputImportOPML, "Import OPML path")
		case "w":
			m = m.startInput(inputExportOPML, "Export OPML path")
		case "b":
			m = m.startInput(inputBookmarkTags, "Raindrop tags (comma separated)")
		case "s":
			_ = m.app.ToggleStar()
		case "m":
			_ = m.app.ToggleRead()
		case "o":
			_ = m.app.OpenSelected()
		case "e":
			_ = m.app.EmailSelected()
		case "y":
			_ = m.app.CopySelectedURL()
		case "f":
			m.app.ToggleFilter()
		case "d":
			_ = m.app.DeleteSelected()
		case "u":
			_ = m.app.Undelete()
		case "G":
			m.queueMissingSummaries()
			return m, m.startNextBatchSummary()
		}
	}
	return m, nil
}

func (m *tuiModel) queueMissingSummaries() {
	if m.app.summarizer == nil {
		m.app.summaryStatus = SummaryNoConfig
		m.app.status = "Summarizer not configured"
		return
	}
	existing := map[int]bool{}
	for _, summary := range m.app.store.Summaries() {
		existing[summary.ArticleID] = true
	}
	m.summaryQueue = m.summaryQueue[:0]
	for _, article := range m.app.articles {
		if existing[article.ID] || m.app.summaryPending[article.ID] {
			continue
		}
		m.summaryQueue = append(m.summaryQueue, article)
	}
	if len(m.summaryQueue) == 0 {
		m.app.status = "No missing summaries"
		m.batchActive = false
		return
	}
	m.batchActive = true
	m.app.status = fmt.Sprintf("Generating %d summaries...", len(m.summaryQueue))
}

func (m *tuiModel) startNextBatchSummary() tea.Cmd {
	if !m.batchActive || len(m.summaryQueue) == 0 {
		m.batchActive = false
		return nil
	}
	article := m.summaryQueue[0]
	m.summaryQueue = m.summaryQueue[1:]
	return m.startSummary(article)
}

func (m *tuiModel) startSummary(article Article) tea.Cmd {
	if m.app.summarizer == nil {
		m.app.summaryStatus = SummaryNoConfig
		m.app.status = "Summarizer not configured"
		return nil
	}
	if summary, ok := m.app.store.FindSummary(article.ID); ok {
		m.app.current = summary
		m.app.summaryStatus = SummaryGenerated
		return nil
	}
	if m.app.summaryPending[article.ID] {
		return nil
	}
	m.app.summaryPending[article.ID] = true
	if selected := m.app.SelectedArticle(); selected != nil && selected.ID == article.ID {
		m.app.summaryStatus = SummaryGenerating
	}
	title := article.Title
	content := firstNonEmpty(article.ContentText, article.Content)
	return summaryCmd(article.ID, title, content, m.app.summarizer)
}

func summaryCmd(articleID int, title string, content string, summarizer *Summarizer) tea.Cmd {
	return func() tea.Msg {
		summaryText, model, err := summarizer.GenerateSummary(title, content)
		return summaryResultMsg{articleID: articleID, summaryText: summaryText, model: model, err: err}
	}
}

func (m tuiModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	base := m.renderLayout()
	if m.showHelp {
		return m.renderHelpOverlay()
	}
	if m.inputMode != inputNone {
		return m.renderInputOverlay(base)
	}
	return base
}

func (m tuiModel) renderLayout() string {
	leftWidth := clamp(int(float64(m.width)*0.32), 24, 40)
	rightWidth := m.width - leftWidth - 2
	if rightWidth < 30 {
		rightWidth = 30
	}

	left := m.renderList(leftWidth)
	paneHeight := m.height - 1
	if paneHeight < 10 {
		paneHeight = 10
	}
	right := m.renderDetails(rightWidth, paneHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	status := m.renderStatusBar(m.width)
	return lipgloss.JoinVertical(lipgloss.Top, body, status)
}

func (m tuiModel) renderList(width int) string {
	style := lipgloss.NewStyle().Width(width).Padding(1, 1, 0, 1)
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("Greeder")
	articles := m.app.FilteredArticles()
	lines := []string{header}
	max := m.height - 6
	if max < 5 {
		max = 5
	}
	if len(articles) < max {
		max = len(articles)
	}
	for i := 0; i < max; i++ {
		article := articles[i]
		prefix := " "
		if i == m.app.selectedIndex {
			prefix = "▸"
		}
		flag := ""
		if article.IsStarred {
			flag = "★"
		} else if article.IsRead {
			flag = "·"
		}
		spinner := ""
		if m.app.summaryPending[article.ID] && len(m.spinnerFrames) > 0 {
			spinner = m.spinnerFrames[m.spinnerIndex]
		}
		titleWidth := width - 8
		if titleWidth < 10 {
			titleWidth = 10
		}
		title := truncate(article.Title, titleWidth)
		line := fmt.Sprintf("%s %s%s %s", prefix, spinner, flag, title)
		if i == m.app.selectedIndex {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(line)
		}
		lines = append(lines, line)
	}
	if len(articles) == 0 {
		lines = append(lines, "No articles. Press 'a' to add a feed.")
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m tuiModel) renderDetails(width int, height int) string {
	style := lipgloss.NewStyle().Width(width).Height(height).Padding(1, 1, 0, 1)
	article := m.app.SelectedArticle()
	if article == nil {
		return style.Render("Select an article to view details.")
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	contentStyle := lipgloss.NewStyle().Width(width - 2)
	summaryStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("214"))
	metaStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("245"))

	content := firstNonEmpty(article.ContentText, article.Content)
	if content == "" {
		content = "No content available."
	}

	summary := m.summaryText()
	topSections := []string{
		titleStyle.Render(article.Title),
		"",
		lipgloss.NewStyle().Bold(true).Render("Content"),
		contentStyle.Render(content),
		"",
		lipgloss.NewStyle().Bold(true).Render("Summary"),
		summaryStyle.Render(summary),
	}

	metaSections := []string{
		lipgloss.NewStyle().Bold(true).Render("Metadata"),
		metaStyle.Render("Published: " + formatLocalTime(article.PublishedAt)),
		metaStyle.Render("Feed: " + valueOrFallback(article.FeedTitle, "Unknown")),
		metaStyle.Render("Author: " + valueOrFallback(article.Author, "Unknown")),
		metaStyle.Render("URL: " + valueOrFallback(article.URL, "Unknown")),
	}

	topHeight := (height - 2) / 2
	if topHeight < 6 {
		topHeight = 6
	}
	bottomHeight := height - topHeight - 2
	if bottomHeight < 4 {
		bottomHeight = 4
	}
	top := lipgloss.NewStyle().Height(topHeight).Render(strings.Join(topSections, "\n"))
	bottom := lipgloss.NewStyle().Height(bottomHeight).Render(strings.Join(metaSections, "\n"))
	return style.Render(lipgloss.JoinVertical(lipgloss.Top, top, bottom))
}

func (m tuiModel) renderStatusBar(width int) string {
	style := lipgloss.NewStyle().Width(width).Padding(0, 1).Foreground(lipgloss.Color("241"))
	status := m.app.status
	if status == "" {
		status = "Ready"
	}
	tip := m.tooltipText()
	left := status
	right := tip
	padding := width - len(left) - len(right) - 2
	if padding < 1 {
		padding = 1
	}
	line := left + strings.Repeat(" ", padding) + right
	return style.Render(line)
}

func (m tuiModel) renderHelpOverlay() string {
	style := lipgloss.NewStyle().Width(m.width).Height(m.height)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).BorderForeground(lipgloss.Color("63"))
	content := []string{
		"Quick Commands",
		"",
		"j/k or arrows  - navigate",
		"enter          - summarize",
		"G              - summarize all",
		"r              - refresh",
		"a              - add feed",
		"i              - import OPML",
		"w              - export OPML",
		"b              - bookmark",
		"s              - star",
		"m              - mark read",
		"o              - open",
		"e              - email",
		"y              - copy url",
		"f              - filter",
		"d              - delete",
		"u              - undelete",
		"/ or esc        - close",
	}
	center := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box.Render(strings.Join(content, "\n")))
	return style.Render(center)
}

func (m tuiModel) renderInputOverlay(base string) string {
	label := m.inputPrompt()
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).BorderForeground(lipgloss.Color("62"))
	content := label + "\n\n" + m.input.View()
	overlay := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box.Render(content))
	return overlay
}

func (m tuiModel) inputPrompt() string {
	switch m.inputMode {
	case inputAddFeed:
		return "Add Feed"
	case inputImportOPML:
		return "Import OPML"
	case inputExportOPML:
		return "Export OPML"
	case inputBookmarkTags:
		return "Bookmark Tags"
	default:
		return "Input"
	}
}

func (m tuiModel) tooltipText() string {
	if m.inputMode != inputNone {
		return "Enter to confirm, Esc to cancel"
	}
	return "Press / for help"
}

func (m tuiModel) summaryText() string {
	switch m.app.summaryStatus {
	case SummaryGenerating:
		return "Generating summary..."
	case SummaryNoConfig:
		return "Configure LM_BASE_URL to enable summaries."
	case SummaryFailed:
		return "Summary failed. Press Enter to retry."
	case SummaryGenerated:
		if m.app.current.Content != "" {
			return m.app.current.Content
		}
		return "No summary available."
	default:
		return "Press Enter to generate a summary."
	}
}

func formatLocalTime(value time.Time) string {
	if value.IsZero() {
		return "Unknown"
	}
	return value.In(time.Local).Format("2006-01-02 15:04")
}

func valueOrFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (m tuiModel) startInput(mode inputMode, placeholder string) tuiModel {
	m.inputMode = mode
	m.input.Placeholder = placeholder
	m.input.SetValue("")
	m.input.Focus()
	return m
}

func (m tuiModel) commitInput() tuiModel {
	mode := m.inputMode
	value := strings.TrimSpace(m.input.Value())
	m.inputMode = inputNone
	m.input.Blur()
	m.input.SetValue("")

	if value == "" {
		m.app.status = "Input cancelled"
		return m
	}

	switch mode {
	case inputAddFeed:
		if err := m.app.AddFeed(value); err != nil {
			m.app.status = "Add feed failed: " + err.Error()
		}
	case inputImportOPML:
		if err := m.app.ImportOPML(value); err != nil {
			m.app.status = "Import failed: " + err.Error()
		}
	case inputExportOPML:
		if err := m.app.ExportOPML(value); err != nil {
			m.app.status = "Export failed: " + err.Error()
		}
	case inputBookmarkTags:
		tags := strings.Split(value, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		if err := m.app.SaveToRaindrop(tags); err != nil {
			m.app.status = "Bookmark failed: " + err.Error()
		}
	}
	return m
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
