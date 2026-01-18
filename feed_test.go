package main

import (
	"net/http"
	"net/http/httptest"
	"io"
	"strings"
	"testing"
	"time"
)

const rssSample = `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Sample RSS</title>
    <link>https://example.com</link>
    <description>Desc</description>
    <item>
      <guid>abc</guid>
      <title>Item One</title>
      <link>https://example.com/1</link>
      <author>Alice</author>
      <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
      <description><![CDATA[<p>Hello</p>]]></description>
    </item>
  </channel>
</rss>`

const atomSample = `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <subtitle>Atom Desc</subtitle>
  <link href="https://example.com" rel="alternate" />
  <entry>
    <id>id-1</id>
    <title>Atom Item</title>
    <link href="https://example.com/entry" />
    <updated>2024-01-02T15:04:05Z</updated>
    <summary>Summary text</summary>
    <author><name>Bob</name></author>
  </entry>
</feed>`

func TestParseRSS(t *testing.T) {
	feed, err := parseFeed("https://example.com/rss", []byte(rssSample))
	if err != nil {
		t.Fatalf("parseFeed RSS error: %v", err)
	}
	if feed.Title != "Sample RSS" || len(feed.Articles) != 1 {
		t.Fatalf("unexpected rss feed: %+v", feed)
	}
	if feed.Articles[0].ContentText != "Hello" {
		t.Fatalf("expected stripped content")
	}
}

func TestParseAtom(t *testing.T) {
	feed, err := parseFeed("https://example.com/atom", []byte(atomSample))
	if err != nil {
		t.Fatalf("parseFeed Atom error: %v", err)
	}
	if feed.Title != "Atom Feed" || len(feed.Articles) != 1 {
		t.Fatalf("unexpected atom feed: %+v", feed)
	}
	if feed.Articles[0].Author != "Bob" {
		t.Fatalf("expected author")
	}
}

func TestDiscoverFeed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rss", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/rss+xml")
		_, _ = w.Write([]byte(rssSample))
	})
	mux.HandleFunc("/site", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><link rel="alternate" type="application/rss+xml" href="/rss" /></head></html>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewFeedFetcher()
	found, err := fetcher.DiscoverFeed(server.URL + "/site")
	if err != nil {
		t.Fatalf("DiscoverFeed error: %v", err)
	}
	if found.Title != "Sample RSS" {
		t.Fatalf("unexpected discovered feed: %+v", found)
	}
}

func TestDiscoverFeedDirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/xml")
		_, _ = w.Write([]byte(rssSample))
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	found, err := fetcher.DiscoverFeed(server.URL)
	if err != nil {
		t.Fatalf("DiscoverFeed direct error: %v", err)
	}
	if found.Title != "Sample RSS" {
		t.Fatalf("unexpected direct feed: %+v", found)
	}
}

func TestDiscoverFeedNoLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><head></head><body>No feeds</body></html>"))
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	if _, err := fetcher.DiscoverFeed(server.URL); err == nil {
		t.Fatalf("expected no feed link error")
	}
}

func TestParseFeedErrors(t *testing.T) {
	if _, err := parseFeed("https://example.com", []byte("<nope></nope>")); err == nil {
		t.Fatalf("expected unsupported feed error")
	}
	if _, err := parseFeed("https://example.com", []byte{}); err == nil {
		t.Fatalf("expected parse error")
	}
	if _, err := parseFeed("https://example.com", []byte("<rss>")); err == nil {
		t.Fatalf("expected invalid xml error")
	}
	if _, err := parseFeed("https://example.com", []byte("<")); err == nil {
		t.Fatalf("expected token error")
	}
}

func TestHelpers(t *testing.T) {
	if got := findFeedLink("<link rel=\"alternate\" type=\"application/rss+xml\" href=\"/feed\" />"); got != "/feed" {
		t.Fatalf("unexpected feed link: %s", got)
	}
	if got := findFeedLink("<link type=\"application/rss+xml\" href=\"/alt\" rel=\"alternate\" />"); got != "/alt" {
		t.Fatalf("unexpected feed link alt: %s", got)
	}
	if got := resolveURL("https://example.com/base", "/feed"); !strings.HasPrefix(got, "https://example.com") {
		t.Fatalf("unexpected resolved url: %s", got)
	}
	if got := resolveURL("http://example.com", "https://other.com/rss"); got != "https://other.com/rss" {
		t.Fatalf("expected absolute url")
	}
	if got := resolveURL("::bad", "relative"); got != "relative" {
		t.Fatalf("expected fallback url")
	}
	if got := resolveURL("https://example.com", "http://[::1"); got != "http://[::1" {
		t.Fatalf("expected fallback for invalid href")
	}
	if got := resolveURL("https://example.com", "http://exa mple.com"); got != "http://exa mple.com" {
		t.Fatalf("expected fallback for join error")
	}
	if got := resolveURL("https://example.com", "%zz"); got != "%zz" {
		t.Fatalf("expected fallback for bad href")
	}
	if got := stripHTML("<p>Hello</p>"); got != "Hello" {
		t.Fatalf("unexpected stripHTML: %s", got)
	}
	if got := stripHTML(""); got != "" {
		t.Fatalf("expected empty stripHTML")
	}
	if t1 := parseTime("Mon, 02 Jan 2006 15:04:05 -0700"); t1.IsZero() {
		t.Fatalf("expected parsed time")
	}
	if t2 := parseTime(""); !t2.IsZero() {
		t.Fatalf("expected zero time")
	}
	if !isLikelyFeed("application/xml", []byte("<rss></rss>")) {
		t.Fatalf("expected likely feed")
	}
	if isLikelyFeed("text/html", []byte("<html></html>")) {
		t.Fatalf("expected not feed")
	}
	if parseTime("not a date") != (time.Time{}) {
		t.Fatalf("expected zero on invalid time")
	}
	if got := firstNonEmpty("", " ", "\n"); got != "" {
		t.Fatalf("expected empty firstNonEmpty")
	}
	if link := findAtomLink([]atomLink{{Rel: "self", Href: "self"}, {Rel: "", Href: "alt"}}); link != "alt" {
		t.Fatalf("expected atom link alt")
	}
	if link := findAtomLink([]atomLink{{Rel: "self", Href: "self"}}); link != "self" {
		t.Fatalf("expected atom link fallback")
	}
	if link := findAtomLink([]atomLink{}); link != "" {
		t.Fatalf("expected empty atom link")
	}
}

func TestFetchFeedErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	if _, err := fetcher.FetchFeed(server.URL); err == nil {
		t.Fatalf("expected fetch error")
	}
}

func TestFetchFeedBadURL(t *testing.T) {
	fetcher := NewFeedFetcher()
	if _, err := fetcher.FetchFeed("http://[::1"); err == nil {
		t.Fatalf("expected bad url error")
	}
}

type errorBody struct{}

func (e *errorBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (e *errorBody) Close() error              { return nil }

type errorBodyRoundTripper struct{}

func (e *errorBodyRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       &errorBody{},
		Header:     http.Header{"content-type": []string{"application/rss+xml"}},
		Request:    &http.Request{Method: http.MethodGet},
	}, nil
}

func TestFetchFeedReadError(t *testing.T) {
	fetcher := &FeedFetcher{client: &http.Client{Transport: &errorBodyRoundTripper{}}}
	if _, err := fetcher.FetchFeed("https://example.com/rss"); err == nil {
		t.Fatalf("expected read error")
	}
}

func TestDiscoverFeedStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	if _, err := fetcher.DiscoverFeed(server.URL); err == nil {
		t.Fatalf("expected discover status error")
	}
}

func TestDiscoverFeedLinkFetchError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/site", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><link rel="alternate" type="application/rss+xml" href="/rss" /></head></html>`))
	})
	mux.HandleFunc("/rss", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewFeedFetcher()
	if _, err := fetcher.DiscoverFeed(server.URL + "/site"); err == nil {
		t.Fatalf("expected discover fetch error")
	}
}

func TestDiscoverFeedPlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain")
		_, _ = w.Write([]byte("no feed here"))
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	if _, err := fetcher.DiscoverFeed(server.URL); err == nil {
		t.Fatalf("expected plain text error")
	}
}

type feedErrorRoundTripper struct{}

func (e *feedErrorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestDiscoverFeedRequestError(t *testing.T) {
	fetcher := &FeedFetcher{client: &http.Client{Transport: &feedErrorRoundTripper{}}}
	if _, err := fetcher.DiscoverFeed("https://example.com"); err == nil {
		t.Fatalf("expected discover request error")
	}
}

func TestDiscoverFeedReadError(t *testing.T) {
	fetcher := &FeedFetcher{client: &http.Client{Transport: &errorBodyRoundTripper{}}}
	if _, err := fetcher.DiscoverFeed("https://example.com/rss"); err == nil {
		t.Fatalf("expected discover read error")
	}
}

func TestDiscoverFeedInvalidLinkedFeed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/site", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><link rel="alternate" type="application/rss+xml" href="/rss" /></head></html>`))
	})
	mux.HandleFunc("/rss", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/rss+xml")
		_, _ = w.Write([]byte("<rss>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewFeedFetcher()
	if _, err := fetcher.DiscoverFeed(server.URL + "/site"); err == nil {
		t.Fatalf("expected invalid linked feed error")
	}
}

func TestParseRSSMissingFields(t *testing.T) {
	content := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title></title>
    <item>
      <link>https://example.com/1</link>
    </item>
  </channel>
</rss>`
	feed, err := parseFeed("https://example.com/rss", []byte(content))
	if err != nil || feed.Articles[0].Title != "Untitled" {
		t.Fatalf("expected default title")
	}
}

func TestParseAtomNoAuthor(t *testing.T) {
	content := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry>
    <id>id-1</id>
    <title>Atom Item</title>
  </entry>
</feed>`
	feed, err := parseFeed("https://example.com/atom", []byte(content))
	if err != nil {
		t.Fatalf("parseFeed error: %v", err)
	}
	if feed.Articles[0].Author != "" {
		t.Fatalf("expected empty author")
	}
}

func TestParseFeedRDF(t *testing.T) {
	content := `<?xml version="1.0"?>
<RDF>
  <channel>
    <title>RDF Feed</title>
    <link>https://example.com</link>
    <description>Desc</description>
    <item>
      <link>https://example.com/1</link>
      <title>Item</title>
    </item>
  </channel>
</RDF>`
	feed, err := parseFeed("https://example.com/rdf", []byte(content))
	if err != nil || feed.Title != "RDF Feed" {
		t.Fatalf("expected rdf feed")
	}
}

func TestParseAtomError(t *testing.T) {
	if _, err := parseAtom([]byte("<feed>"), "https://example.com"); err == nil {
		t.Fatalf("expected atom parse error")
	}
}
