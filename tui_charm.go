package main

import (
	"fmt"
	"strings"

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

type tuiModel struct {
	app        *App
	width      int
	height     int
	input      textinput.Model
	inputMode  inputMode
	showHelp   bool
	statusHint string
}

var (
	teaNewProgram = tea.NewProgram
	runTeaProgram = defaultRunTeaProgram
)

func defaultRunTeaProgram(program *tea.Program) (tea.Model, error) {
	return program.Run()
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
	return tuiModel{app: app, input: input}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			_ = m.app.GenerateSummary()
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
			_ = m.app.GenerateMissingSummaries()
		}
	}
	return m, nil
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
	right := m.renderDetails(rightWidth)
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
		title := truncate(article.Title, width-6)
		line := fmt.Sprintf("%s %s %s", prefix, flag, title)
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

func (m tuiModel) renderDetails(width int) string {
	style := lipgloss.NewStyle().Width(width).Padding(1, 1, 0, 1)
	article := m.app.SelectedArticle()
	if article == nil {
		return style.Render("Select an article to view details.")
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	contentStyle := lipgloss.NewStyle().Width(width - 2)
	summaryStyle := lipgloss.NewStyle().Width(width - 2).Foreground(lipgloss.Color("214"))

	content := firstNonEmpty(article.ContentText, article.Content)
	if content == "" {
		content = "No content available."
	}

	summary := m.summaryText()
	sections := []string{
		titleStyle.Render(article.Title),
		"",
		lipgloss.NewStyle().Bold(true).Render("Content"),
		contentStyle.Render(content),
		"",
		lipgloss.NewStyle().Bold(true).Render("Summary"),
		summaryStyle.Render(summary),
	}
	return style.Render(strings.Join(sections, "\n"))
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
