package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigParseRender(t *testing.T) {
	input := strings.Join([]string{
		"db_path = \"/tmp/test.db\"",
		"refresh_interval_minutes = 15",
		"default_tags = [\"rss\", \"news\"]",
		"raindrop_token = \"token\"",
	}, "\n")
	cfg := DefaultConfig()
	if err := parseConfig(input, &cfg); err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}
	if cfg.DBPath != "/tmp/test.db" || cfg.RefreshIntervalMinutes != 15 {
		t.Fatalf("unexpected config values: %+v", cfg)
	}
	if got := renderConfig(cfg); !strings.Contains(got, "db_path") {
		t.Fatalf("renderConfig missing db_path: %s", got)
	}
}

func TestConfigLoadSave(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	cfg.DBPath = filepath.Join(root, "feeds.db")
	cfg.DefaultTags = []string{}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}
	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig second error: %v", err)
	}
	if cfg2.DBPath != cfg.DBPath {
		t.Fatalf("expected db path %s got %s", cfg.DBPath, cfg2.DBPath)
	}
	if len(cfg2.DefaultTags) != 0 {
		t.Fatalf("expected empty default tags")
	}
}

func TestParseConfigErrors(t *testing.T) {
	cfg := DefaultConfig()
	if err := parseConfig("badline", &cfg); err == nil {
		t.Fatalf("expected error")
	}
	if err := parseConfig("refresh_interval_minutes = nope", &cfg); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseStringArray("nope"); err == nil {
		t.Fatalf("expected array error")
	}
}

func TestTrimQuotes(t *testing.T) {
	if got := trimQuotes("\"hello\""); got != "hello" {
		t.Fatalf("unexpected trimQuotes: %s", got)
	}
	if got := trimQuotes("hello"); got != "hello" {
		t.Fatalf("unexpected trimQuotes no quotes: %s", got)
	}
	if got := trimQuotes(""); got != "" {
		t.Fatalf("expected empty trimQuotes")
	}
	if got := trimQuotes("\"bad"); got != "bad" {
		t.Fatalf("expected fallback trimQuotes")
	}
}

func TestConfigPathFallback(t *testing.T) {
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	})

	if got := configPath(); got != "config.toml" {
		t.Fatalf("expected fallback config path, got %s", got)
	}
}

func TestDefaultDBPathXDG(t *testing.T) {
	root := t.TempDir()
	old := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() { _ = os.Setenv("XDG_DATA_HOME", old) })

	if got := defaultDBPath(); !strings.Contains(got, root) {
		t.Fatalf("expected db path under xdg data")
	}
}

func TestSaveConfigError(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "config-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	old := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filePath)
	t.Cleanup(func() { _ = os.Setenv("XDG_CONFIG_HOME", old) })

	if err := SaveConfig(DefaultConfig()); err == nil {
		t.Fatalf("expected SaveConfig error")
	}
}

func TestParseConfigScannerError(t *testing.T) {
	cfg := DefaultConfig()
	longLine := strings.Repeat("a", 70000)
	if err := parseConfig(longLine, &cfg); err == nil {
		t.Fatalf("expected scanner error")
	}
}

func TestParseConfigUnknownKey(t *testing.T) {
	cfg := DefaultConfig()
	if err := parseConfig("unknown_key = 1", &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigComments(t *testing.T) {
	cfg := DefaultConfig()
	if err := parseConfig("# comment\n\n", &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigDefaultTagsError(t *testing.T) {
	cfg := DefaultConfig()
	if err := parseConfig("default_tags = nope", &cfg); err == nil {
		t.Fatalf("expected default_tags error")
	}
}

func TestDefaultDBPathNoHome(t *testing.T) {
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_DATA_HOME")
	os.Unsetenv("HOME")
	os.Unsetenv("XDG_DATA_HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_DATA_HOME", oldXDG)
	})

	if got := defaultDBPath(); got != "feeds.db" {
		t.Fatalf("expected fallback db path, got %s", got)
	}
}

func TestLoadConfigParseError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })
	path := filepath.Join(root, "speedy-reader", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(path, []byte("badline"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected load error")
	}
}

func TestLoadConfigReadError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })
	path := filepath.Join(root, "speedy-reader", "config.toml")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected read error")
	}
}

func TestLoadConfigSaveError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })
	orig := saveConfig
	saveConfig = func(Config) error { return errors.New("save fail") }
	t.Cleanup(func() { saveConfig = orig })

	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected save error")
	}
}
