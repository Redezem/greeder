package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type FeedFetcher struct {
	client *http.Client
}

type DiscoveredFeed struct {
	Title       string
	URL         string
	SiteURL     string
	Description string
	Articles    []Article
}

func NewFeedFetcher() *FeedFetcher {
	return &FeedFetcher{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *FeedFetcher) FetchFeed(feedURL string) (DiscoveredFeed, error) {
	resp, err := f.client.Get(feedURL)
	if err != nil {
		return DiscoveredFeed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DiscoveredFeed{}, fmt.Errorf("fetch feed: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DiscoveredFeed{}, err
	}
	return parseFeed(feedURL, body)
}

func (f *FeedFetcher) DiscoverFeed(startURL string) (DiscoveredFeed, error) {
	resp, err := f.client.Get(startURL)
	if err != nil {
		return DiscoveredFeed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DiscoveredFeed{}, fmt.Errorf("discover feed: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DiscoveredFeed{}, err
	}
	contentType := resp.Header.Get("content-type")
	if isLikelyFeed(contentType, body) {
		return parseFeed(resp.Request.URL.String(), body)
	}

	feedURL := findFeedLink(string(body))
	if feedURL == "" {
		return DiscoveredFeed{}, errors.New("no feed link found")
	}
	resolved := resolveURL(resp.Request.URL.String(), feedURL)
	return f.FetchFeed(resolved)
}

func isLikelyFeed(contentType string, body []byte) bool {
	if strings.Contains(contentType, "xml") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<rss")) || bytes.HasPrefix(trimmed, []byte("<feed"))
}

func findFeedLink(html string) string {
	linkRe := regexp.MustCompile(`(?i)<link[^>]+rel=["']alternate["'][^>]+type=["']application/(rss|atom)\+xml["'][^>]+href=["']([^"']+)["']`)
	match := linkRe.FindStringSubmatch(html)
	if len(match) >= 3 {
		return match[2]
	}
	altRe := regexp.MustCompile(`(?i)<link[^>]+type=["']application/(rss|atom)\+xml["'][^>]+href=["']([^"']+)["']`)
	match = altRe.FindStringSubmatch(html)
	if len(match) >= 3 {
		return match[2]
	}
	return ""
}

func resolveURL(baseURL string, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	resolved, err := parsed.Parse(href)
	if err != nil {
		return href
	}
	return resolved.String()
}

func parseFeed(feedURL string, body []byte) (DiscoveredFeed, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return DiscoveredFeed{}, err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "rss", "RDF":
				return parseRSS(body, feedURL)
			case "feed":
				return parseAtom(body, feedURL)
			}
		}
	}
	return DiscoveredFeed{}, errors.New("unsupported feed format")
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Author      string `xml:"author"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Content     string `xml:"encoded"`
}

func parseRSS(body []byte, feedURL string) (DiscoveredFeed, error) {
	var doc rssDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return DiscoveredFeed{}, err
	}
	feed := DiscoveredFeed{
		Title:       strings.TrimSpace(doc.Channel.Title),
		URL:         feedURL,
		SiteURL:     strings.TrimSpace(doc.Channel.Link),
		Description: strings.TrimSpace(doc.Channel.Description),
	}
	for _, item := range doc.Channel.Items {
		content := firstNonEmpty(item.Content, item.Description)
		article := Article{
			GUID:        strings.TrimSpace(firstNonEmpty(item.GUID, item.Link, item.Title)),
			Title:       strings.TrimSpace(firstNonEmpty(item.Title, "Untitled")),
			URL:         strings.TrimSpace(item.Link),
			Author:      strings.TrimSpace(item.Author),
			Content:     strings.TrimSpace(content),
			ContentText: stripHTML(content),
			PublishedAt: parseTime(item.PubDate),
		}
		feed.Articles = append(feed.Articles, article)
	}
	return feed, nil
}

type atomFeed struct {
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	Links    []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomEntry struct {
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Links     []atomLink  `xml:"link"`
	Updated   string      `xml:"updated"`
	Published string      `xml:"published"`
	Summary   string      `xml:"summary"`
	Content   string      `xml:"content"`
	Authors   []atomAuthor `xml:"author"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func parseAtom(body []byte, feedURL string) (DiscoveredFeed, error) {
	var doc atomFeed
	if err := xml.Unmarshal(body, &doc); err != nil {
		return DiscoveredFeed{}, err
	}
	feed := DiscoveredFeed{
		Title:       strings.TrimSpace(doc.Title),
		URL:         feedURL,
		SiteURL:     strings.TrimSpace(findAtomLink(doc.Links)),
		Description: strings.TrimSpace(doc.Subtitle),
	}
	for _, entry := range doc.Entries {
		content := firstNonEmpty(entry.Content, entry.Summary)
		author := ""
		if len(entry.Authors) > 0 {
			author = strings.TrimSpace(entry.Authors[0].Name)
		}
		article := Article{
			GUID:        strings.TrimSpace(firstNonEmpty(entry.ID, entry.Title)),
			Title:       strings.TrimSpace(firstNonEmpty(entry.Title, "Untitled")),
			URL:         strings.TrimSpace(findAtomLink(entry.Links)),
			Author:      author,
			Content:     strings.TrimSpace(content),
			ContentText: stripHTML(content),
			PublishedAt: parseTime(firstNonEmpty(entry.Published, entry.Updated)),
		}
		feed.Articles = append(feed.Articles, article)
	}
	return feed, nil
}

func findAtomLink(links []atomLink) string {
	for _, link := range links {
		if link.Rel == "alternate" || link.Rel == "" {
			return link.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 02 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

func stripHTML(value string) string {
	if value == "" {
		return ""
	}
	text := tagRe.ReplaceAllString(value, " ")
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	return text
}
