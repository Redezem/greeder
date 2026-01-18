package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRaindropClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"item":{"_id":42}}`))
	}))
	defer server.Close()
	os.Setenv("RAINDROP_BASE_URL", server.URL)
	defer os.Unsetenv("RAINDROP_BASE_URL")

	client := NewRaindropClient("token")
	id, err := client.Save(RaindropItem{Link: "https://example.com", Title: "Test"})
	if err != nil || id != 42 {
		t.Fatalf("raindrop save error: %v", err)
	}

	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer errServer.Close()
	os.Setenv("RAINDROP_BASE_URL", errServer.URL)
	client = NewRaindropClient("token")
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client := &RaindropClient{baseURL: server.URL, token: "token", client: server.Client()}
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
