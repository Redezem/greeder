package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaybeOfferMigrationNonInteractive(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	legacyPath := legacyConfigPath()
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("db_path = \"legacy.json\"\n"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := maybeOfferMigration(strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("maybeOfferMigration error: %v", err)
	}
	if !strings.Contains(stderr.String(), "Legacy config found") {
		t.Fatalf("expected warning")
	}
}

func TestMaybeOfferMigrationPromptNo(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	legacyPath := legacyConfigPath()
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("db_path = \"legacy.json\"\n"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	stdin, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer stdin.Close()
	stdout, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer stdout.Close()

	if err := maybeOfferMigration(stdin, stdout, os.Stdout); err != nil {
		t.Fatalf("maybeOfferMigration error: %v", err)
	}
}

func TestMigrateLegacyConfigAndDB(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	legacyConfig := legacyConfigPath()
	legacyDB := legacyDefaultDBPath()
	if err := os.MkdirAll(filepath.Dir(legacyConfig), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyDB), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}

	legacyData := legacyStoreData{
		Feeds:     []Feed{{ID: 1, Title: "Feed", URL: "https://example.com/rss", CreatedAt: time.Now().UTC()}},
		Articles:  []Article{{ID: 1, FeedID: 1, GUID: "1", Title: "Title", URL: "https://example.com", FeedTitle: "Feed"}},
		Summaries: []Summary{{ID: 1, ArticleID: 1, Content: "Summary"}},
		Saved:     []Saved{{ArticleID: 1, RaindropID: 10, Tags: []string{"t"}, SavedAt: time.Now().UTC()}},
		Deleted:   []Deleted{{FeedID: 1, GUID: "d1", DeletedAt: time.Now().UTC(), Article: Article{Title: "Old", URL: "u"}}},
	}
	blob, err := json.Marshal(legacyData)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(legacyDB, blob, 0o600); err != nil {
		t.Fatalf("write db error: %v", err)
	}
	if err := os.WriteFile(legacyConfig, []byte("db_path = \""+legacyDB+"\"\n"), 0o600); err != nil {
		t.Fatalf("write config error: %v", err)
	}

	newConfig := configPath()
	if err := migrateLegacyConfigAndDB(legacyConfig, newConfig); err != nil {
		t.Fatalf("migrate error: %v", err)
	}
	if !fileExists(newConfig) {
		t.Fatalf("expected new config")
	}

	newCfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	store, err := NewStore(newCfg.DBPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	if len(store.Feeds()) != 1 || len(store.Articles()) != 1 {
		t.Fatalf("expected migrated data")
	}
	if _, ok := store.FindSummary(1); !ok {
		t.Fatalf("expected migrated summary")
	}
	if store.SavedCount() != 1 {
		t.Fatalf("expected migrated saved")
	}
}

func TestMigrateLegacyDBMissing(t *testing.T) {
	if err := migrateLegacyDB("/nope", "/tmp/new.db"); err == nil {
		t.Fatalf("expected migrate error")
	}
}

func TestFileExists(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if !fileExists(path) {
		t.Fatalf("expected file exists")
	}
	if fileExists(root) {
		t.Fatalf("expected dir not a file")
	}
}
