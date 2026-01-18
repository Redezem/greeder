package main

import (
	"encoding/xml"
	"errors"
	"os"
)

type opmlDocument struct {
	Body opmlBody `xml:"body"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text        string        `xml:"text,attr"`
	Title       string        `xml:"title,attr"`
	Type        string        `xml:"type,attr"`
	XMLURL      string        `xml:"xmlUrl,attr"`
	HTMLURL     string        `xml:"htmlUrl,attr"`
	Children    []opmlOutline `xml:"outline"`
}

var opmlMarshal = func(v any) ([]byte, error) {
	return xml.MarshalIndent(v, "", "  ")
}

func ParseOPML(path string) ([]Feed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc opmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	feeds := []Feed{}
	collectOpml(&feeds, doc.Body.Outlines)
	if len(feeds) == 0 {
		return nil, errors.New("no feeds found in OPML")
	}
	return feeds, nil
}

func collectOpml(feeds *[]Feed, outlines []opmlOutline) {
	for _, outline := range outlines {
		if outline.XMLURL != "" {
			feed := Feed{
				Title:       firstNonEmpty(outline.Title, outline.Text, "Untitled"),
				URL:         outline.XMLURL,
				SiteURL:     outline.HTMLURL,
				Description: "",
			}
			*feeds = append(*feeds, feed)
		}
		if len(outline.Children) > 0 {
			collectOpml(feeds, outline.Children)
		}
	}
}

func ExportOPML(path string, feeds []Feed) error {
	outlines := make([]opmlOutline, 0, len(feeds))
	for _, feed := range feeds {
		outlines = append(outlines, opmlOutline{
			Title:   feed.Title,
			Text:    feed.Title,
			Type:    "rss",
			XMLURL:  feed.URL,
			HTMLURL: feed.SiteURL,
		})
	}
	doc := opmlDocument{Body: opmlBody{Outlines: outlines}}
	data, err := opmlMarshal(doc)
	if err != nil {
		return err
	}
	data = append([]byte(xml.Header), data...)
	return os.WriteFile(path, data, 0o644)
}
