package main

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyPathsHelpers(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})
	if got := legacyConfigPath(); !strings.Contains(got, root) {
		t.Fatalf("expected legacy config path under root")
	}
	if got := legacyDefaultDBPath(); !strings.Contains(got, root) {
		t.Fatalf("expected legacy db path under root")
	}

	os.Unsetenv("XDG_DATA_HOME")
	os.Setenv("HOME", root)
	if got := legacyDefaultDBPath(); !strings.Contains(got, root) {
		t.Fatalf("expected legacy db path under home")
	}
}

func TestLegacyPathFallbacks(t *testing.T) {
	origConfig := userConfigDir
	userConfigDir = func() (string, error) { return "", errors.New("fail") }
	t.Cleanup(func() { userConfigDir = origConfig })
	if got := legacyConfigPath(); got != "config.toml" {
		t.Fatalf("expected config fallback")
	}

	origHome := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("fail") }
	t.Cleanup(func() { userHomeDir = origHome })
	os.Unsetenv("XDG_DATA_HOME")
	if got := legacyDefaultDBPath(); got != "feeds.db" {
		t.Fatalf("expected db fallback")
	}
}

func TestMaybeOfferMigrationYes(t *testing.T) {
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
	if err := os.WriteFile(legacyConfig, []byte("db_path = \""+legacyDB+"\"\n"), 0o600); err != nil {
		t.Fatalf("write config error: %v", err)
	}
	if err := os.WriteFile(legacyDB, []byte(`{"feeds":[],"articles":[],"summaries":[],"saved":[],"deleted":[]}`), 0o600); err != nil {
		t.Fatalf("write db error: %v", err)
	}

	orig := terminalCheck
	terminalCheck = func(io.Reader, io.Writer) bool { return true }
	t.Cleanup(func() { terminalCheck = orig })

	stdin := bytes.NewBufferString("y\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if err := maybeOfferMigration(stdin, stdout, stderr); err != nil {
		t.Fatalf("maybeOfferMigration error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Migration complete") {
		t.Fatalf("expected migration complete")
	}
}

func TestMigrateLegacyConfigBadParse(t *testing.T) {
	root := t.TempDir()
	legacyConfig := filepath.Join(root, "bad.toml")
	newConfig := filepath.Join(root, "new.toml")
	if err := os.WriteFile(legacyConfig, []byte("badline"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyConfigAndDB(legacyConfig, newConfig); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestMigrateLegacyDBEmpty(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "legacy.json")
	newPath := filepath.Join(root, "store.db")
	if err := os.WriteFile(oldPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(oldPath, newPath); err != nil {
		t.Fatalf("migrate error: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected new db")
	}
}

func TestMaybeOfferMigrationNewConfigExists(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })
	newConfig := configPath()
	if err := os.MkdirAll(filepath.Dir(newConfig), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(newConfig, []byte("db_path = \"x\"\n"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := maybeOfferMigration(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("maybeOfferMigration error: %v", err)
	}
}

func TestMigrateLegacyConfigErrorBranches(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := migrateLegacyConfigAndDB(legacyDir, filepath.Join(root, "new.toml")); err == nil {
		t.Fatalf("expected read error")
	}

	legacyConfig := filepath.Join(root, "legacy.toml")
	legacyDB := filepath.Join(root, "legacy.db")
	if err := os.WriteFile(legacyConfig, []byte("db_path = \""+legacyDB+"\"\n"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := os.WriteFile(legacyDB, []byte(`{"feeds":[],"articles":[],"summaries":[],"saved":[],"deleted":[]}`), 0o600); err != nil {
		t.Fatalf("write db error: %v", err)
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := migrateLegacyConfigAndDB(legacyConfig, filepath.Join(blocker, "config.toml")); err == nil {
		t.Fatalf("expected mkdir error")
	}

	readonlyDir := filepath.Join(root, "readonly")
	if err := os.MkdirAll(readonlyDir, 0o500); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := migrateLegacyConfigAndDB(legacyConfig, readonlyDir); err == nil {
		t.Fatalf("expected write error")
	}

	if err := os.Chmod(readonlyDir, 0o700); err != nil {
		t.Fatalf("chmod error: %v", err)
	}
	if err := os.WriteFile(legacyConfig, []byte("db_path = \"/nope\"\n"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyConfigAndDB(legacyConfig, filepath.Join(root, "new.toml")); err == nil {
		t.Fatalf("expected migrate db error")
	}
}

func TestMigrateLegacyDBErrorBranches(t *testing.T) {
	root := t.TempDir()
	badDir := filepath.Join(root, "dir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := migrateLegacyDB(badDir, filepath.Join(root, "new.db")); err == nil {
		t.Fatalf("expected read error")
	}

	unreadable := filepath.Join(root, "unreadable.json")
	if err := os.WriteFile(unreadable, []byte("[]"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod error: %v", err)
	}
	if err := migrateLegacyDB(unreadable, filepath.Join(root, "new.db")); err == nil {
		t.Fatalf("expected read error")
	}
	if err := os.Chmod(unreadable, 0o600); err != nil {
		t.Fatalf("chmod error: %v", err)
	}

	badJSON := filepath.Join(root, "bad.json")
	if err := os.WriteFile(badJSON, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(badJSON, filepath.Join(root, "new.db")); err == nil {
		t.Fatalf("expected json error")
	}

	validJSON := filepath.Join(root, "valid.json")
	if err := os.WriteFile(validJSON, []byte(`{"feeds":[],"articles":[],"summaries":[],"saved":[],"deleted":[]}`), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dbdir"), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := migrateLegacyDB(validJSON, filepath.Join(root, "dbdir")); err == nil {
		t.Fatalf("expected new store error")
	}

	origBegin := beginTx
	beginTx = func(*sql.DB) (*sql.Tx, error) { return nil, errors.New("begin fail") }
	t.Cleanup(func() { beginTx = origBegin })
	if err := migrateLegacyDB(validJSON, filepath.Join(root, "new.db")); err == nil {
		t.Fatalf("expected begin error")
	}

	origRead := legacyReadFile
	legacyReadFile = func(string) ([]byte, error) { return nil, errors.New("read fail") }
	t.Cleanup(func() { legacyReadFile = origRead })
	if err := migrateLegacyDB(validJSON, filepath.Join(root, "new.db")); err == nil {
		t.Fatalf("expected read error")
	}
}

func TestMigrateLegacyDBLoopErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.json")

	data := `{"feeds":[{"id":1,"title":"A","url":"u"},{"id":2,"title":"B","url":"u"}],"articles":[],"summaries":[],"saved":[],"deleted":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(path, filepath.Join(root, "feed.db")); err == nil {
		t.Fatalf("expected feed insert error")
	}

	data = `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[{"id":1,"feed_id":1,"guid":"g"},{"id":1,"feed_id":1,"guid":"g2"}],"summaries":[],"saved":[],"deleted":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(path, filepath.Join(root, "article.db")); err == nil {
		t.Fatalf("expected article insert error")
	}

	data = `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[{"id":1,"feed_id":1,"guid":"g"}],"summaries":[{"id":1,"article_id":1},{"id":2,"article_id":1}],"saved":[],"deleted":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(path, filepath.Join(root, "summary.db")); err == nil {
		t.Fatalf("expected summary insert error")
	}

	data = `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[{"id":1,"feed_id":1,"guid":"g"}],"summaries":[],"saved":[{"article_id":1,"raindrop_id":1,"tags":["a"]},{"article_id":1,"raindrop_id":2,"tags":["b"]}],"deleted":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(path, filepath.Join(root, "saved.db")); err == nil {
		t.Fatalf("expected saved insert error")
	}

	origMarshal := legacyJSONMarshal
	legacyJSONMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal fail") }
	t.Cleanup(func() { legacyJSONMarshal = origMarshal })
	data = `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[],"summaries":[],"saved":[{"article_id":1,"raindrop_id":1,"tags":["a"]}],"deleted":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(path, filepath.Join(root, "marshal.db")); err == nil {
		t.Fatalf("expected marshal error")
	}

	origSchema := schemaInit
	schemaInit = func(db *sql.DB) error {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS feeds (
			id INTEGER PRIMARY KEY,
			title TEXT,
			url TEXT UNIQUE,
			site_url TEXT,
			description TEXT,
			last_fetched INTEGER,
			created_at INTEGER,
			updated_at INTEGER
		);`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS articles (
			id INTEGER PRIMARY KEY,
			feed_id INTEGER,
			guid TEXT,
			title TEXT,
			url TEXT,
			base_url TEXT,
			author TEXT,
			content TEXT,
			content_text TEXT,
			published_at INTEGER,
			fetched_at INTEGER,
			is_read INTEGER,
			is_starred INTEGER,
			feed_title TEXT,
			UNIQUE(feed_id, guid)
		);`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS article_sources (
			article_id INTEGER,
			feed_id INTEGER,
			published_at INTEGER
		);`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS summaries (
			id INTEGER PRIMARY KEY,
			article_id INTEGER UNIQUE,
			content TEXT,
			model TEXT,
			generated_at INTEGER
		);`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS saved (
			article_id INTEGER PRIMARY KEY,
			raindrop_id INTEGER,
			tags TEXT,
			saved_at INTEGER
		);`); err != nil {
			return err
		}
		return nil
	}
	t.Cleanup(func() { schemaInit = origSchema })
	data = `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[],"summaries":[],"saved":[],"deleted":[{"feed_id":1,"guid":"g","article":{"title":"t","url":"u"}}]}`
	data = `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[],"summaries":[],"saved":[],"deleted":[{"feed_id":1,"guid":"g","deleted_at":"2024-01-01T00:00:00Z","article":{"id":1,"feed_id":1,"guid":"g","title":"t","url":"u"}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := migrateLegacyDB(path, filepath.Join(root, "deleted.db")); err == nil {
		t.Fatalf("expected deleted insert error")
	}
}

func TestMigrateLegacyDBArticleSourcesError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.json")
	data := `{"feeds":[{"id":1,"title":"A","url":"u"}],"articles":[{"id":1,"feed_id":1,"guid":"g","title":"t","url":"https://example.com/a"}],"summaries":[],"saved":[],"deleted":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	origSchema := schemaInit
	schemaInit = func(db *sql.DB) error {
		if err := origSchema(db); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE TRIGGER article_sources_insert_block BEFORE INSERT ON article_sources BEGIN SELECT RAISE(FAIL, 'no'); END;`); err != nil {
			return err
		}
		return nil
	}
	t.Cleanup(func() { schemaInit = origSchema })
	if err := migrateLegacyDB(path, filepath.Join(root, "sources.db")); err == nil {
		t.Fatalf("expected article_sources insert error")
	}
}
