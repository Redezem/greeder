package main

import (
	"os"
	"path/filepath"
	"testing"
)

const opmlSample = `<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline text="Feed" title="Feed" type="rss" xmlUrl="https://example.com/rss" htmlUrl="https://example.com" />
  </body>
</opml>`

func TestOPMLImportExport(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "feeds.opml")
	if err := os.WriteFile(input, []byte(opmlSample), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	feeds, err := ParseOPML(input)
	if err != nil {
		t.Fatalf("ParseOPML error: %v", err)
	}
	if len(feeds) != 1 || feeds[0].URL == "" {
		t.Fatalf("unexpected feeds: %+v", feeds)
	}

	output := filepath.Join(root, "out.opml")
	if err := ExportOPML(output, feeds); err != nil {
		t.Fatalf("ExportOPML error: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("expected output file")
	}
}

func TestOPMLError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.opml")
	if err := os.WriteFile(path, []byte("<opml></opml>"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := ParseOPML(path); err == nil {
		t.Fatalf("expected error for empty opml")
	}
}

func TestOPMLMissingFile(t *testing.T) {
	if _, err := ParseOPML(filepath.Join(t.TempDir(), "missing.opml")); err == nil {
		t.Fatalf("expected missing file error")
	}
}

func TestOPMLInvalidXML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.xml")
	if err := os.WriteFile(path, []byte("<opml>"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := ParseOPML(path); err == nil {
		t.Fatalf("expected xml error")
	}
}

func TestOPMLNestedOutlines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested.opml")
	content := `<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline text="Group">
      <outline text="Feed" title="Feed" type="rss" xmlUrl="https://example.com/rss" htmlUrl="https://example.com" />
    </outline>
  </body>
</opml>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	feeds, err := ParseOPML(path)
	if err != nil || len(feeds) != 1 {
		t.Fatalf("expected nested feed")
	}
}

func TestOPMLMarshalError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "out.opml")
	orig := opmlMarshal
	opmlMarshal = func(v any) ([]byte, error) {
		return nil, os.ErrInvalid
	}
	t.Cleanup(func() { opmlMarshal = orig })
	if err := ExportOPML(path, []Feed{{Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestOPMLWriteError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	path := filepath.Join(blocker, "out.opml")
	if err := ExportOPML(path, []Feed{{Title: "A", URL: "u"}}); err == nil {
		t.Fatalf("expected write error")
	}
}
