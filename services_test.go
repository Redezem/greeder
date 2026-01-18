package main

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRaindropClient(t *testing.T) {
	os.Setenv("RAINDROP_BASE_URL", "http://example.test")
	defer os.Unsetenv("RAINDROP_BASE_URL")
	client := NewRaindropClient("token")
	client.client = clientForResponse(http.StatusOK, `{"item":{"_id":42}}`, map[string]string{"content-type": "application/json"})
	id, err := client.Save(RaindropItem{Link: "https://example.com", Title: "Test"})
	if err != nil || id != 42 {
		t.Fatalf("raindrop save error: %v", err)
	}

	os.Setenv("RAINDROP_BASE_URL", "http://example.test")
	client = NewRaindropClient("token")
	client.client = clientForResponse(http.StatusBadRequest, "", nil)
	if _, err := client.Save(RaindropItem{Link: "https://example.com"}); err == nil {
		t.Fatalf("expected raindrop error")
	}
}

func TestOpenURL(t *testing.T) {
	if err := defaultOpenURL(""); err == nil {
		t.Fatalf("expected empty url error")
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, "xdg-open")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake xdg-open: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	if err := defaultOpenURL("https://example.com"); err != nil {
		t.Fatalf("defaultOpenURL error: %v", err)
	}
	if err := defaultSendEmail("mailto:test"); err != nil {
		t.Fatalf("defaultSendEmail error: %v", err)
	}

	if err := defaultOpenURLForOS("unsupported", "https://example.com"); err == nil {
		t.Fatalf("expected unsupported platform error")
	}
}

func TestOpenCommand(t *testing.T) {
	if cmd, args := openCommandForOS("darwin", "https://example.com"); cmd != "open" || len(args) == 0 {
		t.Fatalf("expected darwin open command")
	}
	if cmd, args := openCommandForOS("windows", "https://example.com"); cmd != "rundll32" || len(args) == 0 {
		t.Fatalf("expected windows open command")
	}
	if cmd, args := openCommandForOS("linux", "https://example.com"); cmd != "xdg-open" || len(args) == 0 {
		t.Fatalf("expected linux open command")
	}
	if cmd, _ := openCommand("https://example.com"); cmd == "" {
		t.Fatalf("expected open command")
	}
	if client := NewRaindropClient(" "); client != nil {
		t.Fatalf("expected nil raindrop client")
	}
}

func TestRaindropMarshalError(t *testing.T) {
	orig := servicesJSONMarshal
	servicesJSONMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("marshal fail")
	}
	t.Cleanup(func() { servicesJSONMarshal = orig })

	client := &RaindropClient{baseURL: "http://example.com", token: "token", client: http.DefaultClient}
	if _, err := client.Save(RaindropItem{Link: "https://example.com"}); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestRaindropRequestError(t *testing.T) {
	client := &RaindropClient{baseURL: "http://[::1", token: "token", client: http.DefaultClient}
	if _, err := client.Save(RaindropItem{Link: "https://example.com"}); err == nil {
		t.Fatalf("expected request error")
	}
}

type raindropErrorRoundTripper struct{}

func (e *raindropErrorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport fail")
}

func TestRaindropDoError(t *testing.T) {
	client := &RaindropClient{baseURL: "http://example.com", token: "token", client: &http.Client{Transport: &raindropErrorRoundTripper{}}}
	if _, err := client.Save(RaindropItem{Link: "https://example.com"}); err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestRaindropDecodeError(t *testing.T) {
	client := &RaindropClient{baseURL: "http://example.test", token: "token", client: clientForResponse(http.StatusOK, "not-json", map[string]string{"content-type": "application/json"})}
	if _, err := client.Save(RaindropItem{Link: "https://example.com"}); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestRaindropNilClient(t *testing.T) {
	var client *RaindropClient
	if _, err := client.Save(RaindropItem{Link: "https://example.com"}); err == nil {
		t.Fatalf("expected nil client error")
	}
}

func TestClipboardCommands(t *testing.T) {
	if cmds := clipboardCommandsForOS("darwin"); len(cmds) == 0 || cmds[0].name != "pbcopy" {
		t.Fatalf("expected darwin clipboard")
	}
	if cmds := clipboardCommandsForOS("windows"); len(cmds) == 0 || cmds[0].name == "" {
		t.Fatalf("expected windows clipboard")
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")
	if cmds := clipboardCommandsForOS("linux"); len(cmds) != 1 || cmds[0].name != "wl-copy" {
		t.Fatalf("expected wayland clipboard")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	if cmds := clipboardCommandsForOS("linux"); len(cmds) == 0 || cmds[0].name != "xclip" {
		t.Fatalf("expected x11 clipboard")
	}
	if cmds := clipboardCommandsForOS("plan9"); cmds != nil {
		t.Fatalf("expected no clipboard commands")
	}
}

func TestCopyToClipboardSuccess(t *testing.T) {
	orig := clipboardRun
	clipboardRun = func(cmd string, args []string, input string) error { return nil }
	t.Cleanup(func() { clipboardRun = orig })
	if err := copyToClipboard("hello"); err != nil {
		t.Fatalf("expected clipboard success: %v", err)
	}
}

func TestCopyToClipboardEmpty(t *testing.T) {
	if err := copyToClipboard(" "); err == nil {
		t.Fatalf("expected empty clipboard error")
	}
}

func TestCopyToClipboardUnsupported(t *testing.T) {
	orig := clipboardCommands
	clipboardCommands = func(goos string) []clipboardCommand { return nil }
	t.Cleanup(func() { clipboardCommands = orig })
	if err := copyToClipboard("hello"); err == nil {
		t.Fatalf("expected unsupported clipboard error")
	}
}

func TestCopyToClipboardFailure(t *testing.T) {
	origRun := clipboardRun
	clipboardRun = func(cmd string, args []string, input string) error { return errors.New("fail") }
	t.Cleanup(func() { clipboardRun = origRun })
	origCmds := clipboardCommands
	clipboardCommands = func(goos string) []clipboardCommand {
		return []clipboardCommand{{name: "one"}, {name: "two"}}
	}
	t.Cleanup(func() { clipboardCommands = origCmds })
	if err := copyToClipboard("hello"); err == nil {
		t.Fatalf("expected clipboard failure")
	}
}

func TestDefaultClipboardRun(t *testing.T) {
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestClipboardHelperProcess", "--", name}, args...)...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
	t.Cleanup(func() { execCommand = orig })

	if err := defaultClipboardRun("ignored", []string{"arg"}, "input"); err != nil {
		t.Fatalf("defaultClipboardRun error: %v", err)
	}
}

func TestClipboardHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}
