package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMainImportRefreshAndRun(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	defer os.Unsetenv("XDG_CONFIG_HOME")
	defer os.Unsetenv("XDG_DATA_HOME")

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/rss+xml")
		_, _ = w.Write([]byte(rssSample))
	}))
	defer feedServer.Close()

	opmlPath := filepath.Join(root, "feeds.opml")
	if err := ExportOPML(opmlPath, []Feed{{Title: "Feed", URL: feedServer.URL}}); err != nil {
		t.Fatalf("ExportOPML error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain([]string{"--import", opmlPath}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("runMain import error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Imported feeds") {
		t.Fatalf("expected import output")
	}

	stdout.Reset()
	if err := runMain([]string{"--refresh"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("runMain refresh error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Refreshed") {
		t.Fatalf("expected refresh output")
	}

	stdout.Reset()
	if err := runMain(nil, strings.NewReader("q\n"), &stdout, &stderr); err != nil {
		t.Fatalf("runMain run error: %v", err)
	}
}

func TestRunMainConfigError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	defer os.Unsetenv("XDG_CONFIG_HOME")

	path := filepath.Join(root, "speedy-reader", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(path, []byte("badline"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain(nil, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("expected config error")
	}
	if !strings.Contains(stderr.String(), "config error") {
		t.Fatalf("expected config error output")
	}
}

func TestMainExit(t *testing.T) {
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
	called := 0
	orig := exitFunc
	exitFunc = func(code int) { called = code }
	t.Cleanup(func() { exitFunc = orig })

	origArgs := os.Args
	os.Args = []string{"greeder"}
	t.Cleanup(func() { os.Args = origArgs })

	main()
	if called != 1 {
		t.Fatalf("expected exit code 1")
	}
}

func TestRunMainInitError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })
	path := filepath.Join(root, "speedy-reader", "config.toml")
	dbDir := filepath.Join(root, "dbdir")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(path, []byte("db_path = \""+dbDir+"\"\n"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain(nil, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("expected init error")
	}
	if !strings.Contains(stderr.String(), "init error") {
		t.Fatalf("expected init error output")
	}
}

func TestRunMainRunError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain(nil, &failingReader{}, &stdout, &stderr); err == nil {
		t.Fatalf("expected run error")
	}
	if !strings.Contains(stderr.String(), "run error") {
		t.Fatalf("expected run error output")
	}
}

func TestRunMainImportError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain([]string{"--import", filepath.Join(root, "missing.opml")}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("expected import error")
	}
	if !strings.Contains(stderr.String(), "import error") {
		t.Fatalf("expected import error output")
	}
}

func TestRunMainRefreshError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})
	orig := refreshFeeds
	refreshFeeds = func(*App) error { return errors.New("refresh fail") }
	t.Cleanup(func() { refreshFeeds = orig })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain([]string{"--refresh"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatalf("expected refresh error")
	}
	if !strings.Contains(stderr.String(), "refresh error") {
		t.Fatalf("expected refresh error output")
	}
}

func TestRunMainUsesTUI(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	orig := runTUI
	called := false
	runTUI = func(*App) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runTUI = orig })

	tty, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer tty.Close()

	if err := runMain(nil, tty, tty, &bytes.Buffer{}); err != nil {
		t.Fatalf("runMain error: %v", err)
	}
	if !called {
		t.Fatalf("expected runTUI call")
	}
}

func TestIsTerminalHelpers(t *testing.T) {
	if isTerminalReader(strings.NewReader("x")) {
		t.Fatalf("expected non-terminal reader")
	}
	if isTerminalWriter(&bytes.Buffer{}) {
		t.Fatalf("expected non-terminal writer")
	}

	tty, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer tty.Close()
	if !isTerminalReader(tty) {
		t.Fatalf("expected terminal reader")
	}
	if !isTerminalWriter(tty) {
		t.Fatalf("expected terminal writer")
	}

	bad := os.NewFile(^uintptr(0), "bad")
	if isTerminalReader(bad) {
		t.Fatalf("expected bad reader false")
	}
	if isTerminalWriter(bad) {
		t.Fatalf("expected bad writer false")
	}
}

func TestRunMainNonTTYFallback(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runMain(nil, strings.NewReader("q\n"), &stdout, &stderr); err != nil {
		t.Fatalf("expected fallback run success: %v", err)
	}
}

func TestRunMainTUIError(t *testing.T) {
	root := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", root)
	os.Setenv("XDG_DATA_HOME", root)
	t.Cleanup(func() {
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_DATA_HOME")
	})

	orig := runTUI
	runTUI = func(*App) error { return errors.New("tui fail") }
	t.Cleanup(func() { runTUI = orig })

	tty, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer tty.Close()

	if err := runMain(nil, tty, tty, &bytes.Buffer{}); err == nil {
		t.Fatalf("expected tui error")
	}
}
