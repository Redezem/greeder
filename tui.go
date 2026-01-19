package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func Run(app *App, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	fmt.Fprintln(out, render(app))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			line = "enter"
		}
		if err := handleCommand(app, line, out); err != nil {
			return err
		}
		if line == "q" || line == "quit" {
			break
		}
		fmt.Fprintln(out, render(app))
	}
	return scanner.Err()
}

func handleCommand(app *App, line string, out io.Writer) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	switch parts[0] {
	case "q", "quit":
		app.store.Compact(7)
		return nil
	case "j", "down":
		app.MoveSelection(1)
	case "k", "up":
		app.MoveSelection(-1)
	case "enter":
		return app.GenerateSummary()
	case "r", "refresh":
		return app.RefreshFeeds()
	case "a", "add":
		if len(parts) < 2 {
			return fmt.Errorf("missing feed url")
		}
		return app.AddFeed(parts[1])
	case "i", "import":
		if len(parts) < 2 {
			return fmt.Errorf("missing opml path")
		}
		return app.ImportOPML(parts[1])
	case "w", "export":
		if len(parts) < 2 {
			return fmt.Errorf("missing opml path")
		}
		return app.ExportOPML(parts[1])
	case "I", "import-state":
		if len(parts) < 2 {
			return fmt.Errorf("missing state path")
		}
		return app.ImportState(parts[1])
	case "E", "export-state":
		if len(parts) < 2 {
			return fmt.Errorf("missing state path")
		}
		return app.ExportState(parts[1])
	case "s", "star":
		return app.ToggleStar()
	case "m", "mark":
		return app.ToggleRead()
	case "o", "open":
		return app.OpenSelected()
	case "e", "email":
		return app.EmailSelected()
	case "y", "copy":
		return app.CopySelectedURL()
	case "b", "bookmark":
		tags := []string{}
		if len(parts) > 1 {
			tags = strings.Split(parts[1], ",")
		}
		return app.SaveToRaindrop(tags)
	case "f", "filter":
		app.ToggleFilter()
	case "d", "delete":
		return app.DeleteSelected()
	case "u", "undelete":
		return app.Undelete()
	case "G", "bulk":
		return app.GenerateMissingSummaries()
	case "?", "help":
		fmt.Fprintln(out, helpText())
	}
	return nil
}

func render(app *App) string {
	articles := app.FilteredArticles()
	leftWidth := 32
	lines := []string{}
	lines = append(lines, headerLine(app, leftWidth))
	max := len(articles)
	if max > 8 {
		max = 8
	}
	for i := 0; i < max; i++ {
		article := articles[i]
		prefix := " "
		if i == app.selectedIndex {
			prefix = ">"
		}
		title := truncate(article.Title, leftWidth-4)
		line := fmt.Sprintf("%s %-*s |", prefix, leftWidth-2, title)
		lines = append(lines, line)
	}
	for len(lines) < 10 {
		lines = append(lines, fmt.Sprintf("  %-*s |", leftWidth-2, ""))
	}
	article := app.SelectedArticle()
	right := renderRightPane(article, app)
	right = padLines(right, len(lines))
	combined := make([]string, len(lines))
	for i := range lines {
		combined[i] = lines[i] + " " + right[i]
	}
	return strings.Join(combined, "\n")
}

func headerLine(app *App, width int) string {
	label := fmt.Sprintf(" %d Articles", len(app.articles))
	saved := fmt.Sprintf("%d Saved ", app.store.SavedCount())
	padding := width - len(label) - len(saved)
	if padding < 1 {
		padding = 1
	}
	return label + strings.Repeat(" ", padding) + saved + "|"
}

func renderRightPane(article *Article, app *App) []string {
	lines := []string{}
	if article == nil {
		lines = append(lines, "No article selected")
		return padLines(lines, 10)
	}
	lines = append(lines, "Title: "+article.Title)
	content := firstNonEmpty(article.ContentText, article.Content)
	if content == "" {
		content = "No content available"
	}
	lines = append(lines, "Content: "+truncate(content, 60))
	lines = append(lines, "Summary:")
	switch app.summaryStatus {
	case SummaryGenerating:
		lines = append(lines, "  Generating...")
	case SummaryNoConfig:
		lines = append(lines, "  Summarizer not configured")
	case SummaryFailed:
		lines = append(lines, "  Failed to generate summary")
	case SummaryGenerated:
		lines = append(lines, "  "+truncate(app.current.Content, 60))
	default:
		lines = append(lines, "  Press Enter to summarize")
	}
	lines = append(lines, "Metadata:")
	lines = append(lines, "  Published: "+formatLocalTime(article.PublishedAt))
	lines = append(lines, "  Feed: "+valueOrFallback(article.FeedTitle, "Unknown"))
	lines = append(lines, "  Author: "+valueOrFallback(article.Author, "Unknown"))
	lines = append(lines, "  URL: "+valueOrFallback(article.URL, "Unknown"))
	if app.status != "" {
		lines = append(lines, "Status: "+app.status)
	}
	return padLines(lines, 10)
}

func padLines(lines []string, total int) []string {
	for len(lines) < total {
		lines = append(lines, "")
	}
	return lines
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func helpText() string {
	return strings.Join([]string{
		"Commands:",
		"  j/k: move",
		"  enter: summarize",
		"  G: summarize all missing",
		"  r: refresh",
		"  a <url>: add feed",
		"  i <path>: import opml",
		"  w <path>: export opml",
		"  I <path>: import state",
		"  E <path>: export state",
		"  s: star",
		"  m: mark read",
		"  o: open",
		"  e: email",
		"  y: copy url",
		"  b <tag,tag>: bookmark",
		"  f: filter",
		"  d: delete",
		"  u: undelete",
		"  q: quit",
	}, "\n")
}
