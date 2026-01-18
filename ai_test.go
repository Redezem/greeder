package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSummarizerFromEnv(t *testing.T) {
	os.Unsetenv("LM_BASE_URL")
	if got := NewSummarizerFromEnv(); got != nil {
		t.Fatalf("expected nil summarizer")
	}
}

func TestSummarizerGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/chat/completions") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"- one\n- two"}}]}`))
	}))
	defer server.Close()

	os.Setenv("LM_BASE_URL", server.URL)
	os.Setenv("LM_MODEL", "test-model")
	defer os.Unsetenv("LM_BASE_URL")
	defer os.Unsetenv("LM_MODEL")

	summarizer := NewSummarizerFromEnv()
	if summarizer == nil {
		t.Fatalf("expected summarizer")
	}
	content, model, err := summarizer.GenerateSummary("Title", strings.Repeat("a", 20001))
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
	if model != "test-model" || !strings.Contains(content, "one") {
		t.Fatalf("unexpected summary: %s %s", model, content)
	}
}

func TestSummarizerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	os.Setenv("LM_BASE_URL", server.URL)
	defer os.Unsetenv("LM_BASE_URL")
	summarizer := NewSummarizerFromEnv()
	if summarizer == nil {
		t.Fatalf("expected summarizer")
	}
	if _, _, err := summarizer.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected http error")
	}

	serverEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer serverEmpty.Close()

	os.Setenv("LM_BASE_URL", serverEmpty.URL)
	summarizer = NewSummarizerFromEnv()
	if _, _, err := summarizer.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected empty choices error")
	}
}

func TestTruncateTextInvalidUTF8(t *testing.T) {
	input := string([]byte{0xff, 0xfe, 0xfd})
	if got := truncateText(input, 2); got == input {
		t.Fatalf("expected truncated utf8 cleanup")
	}
}

func TestSummarizerBaseURLWithV1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"- ok"}}]}`))
	}))
	defer server.Close()

	s := &Summarizer{baseURL: server.URL + "/v1", model: "m", client: server.Client()}
	if _, _, err := s.GenerateSummary("Title", "Body"); err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
}

func TestSummarizerDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	s := &Summarizer{baseURL: server.URL, model: "m", client: server.Client()}
	if _, _, err := s.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestSummarizerNil(t *testing.T) {
	var s *Summarizer
	if _, _, err := s.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected nil summarizer error")
	}
}

func TestSummarizerMarshalError(t *testing.T) {
	orig := aiJSONMarshal
	aiJSONMarshal = func(v any) ([]byte, error) {
		return nil, errors.New("marshal fail")
	}
	t.Cleanup(func() { aiJSONMarshal = orig })

	s := &Summarizer{baseURL: "http://example.com", model: "m", client: http.DefaultClient}
	if _, _, err := s.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestSummarizerRequestError(t *testing.T) {
	s := &Summarizer{baseURL: "http://[::1", model: "m", client: http.DefaultClient}
	if _, _, err := s.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected request error")
	}
}

type errorRoundTripper struct{}

func (e *errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("transport fail")
}

func TestSummarizerDoError(t *testing.T) {
	client := &http.Client{Transport: &errorRoundTripper{}}
	s := &Summarizer{baseURL: "http://example.com", model: "m", client: client}
	if _, _, err := s.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestSummarizerAPIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("authorization"); got != "Bearer key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"- ok"}}]}`))
	}))
	defer server.Close()

	s := &Summarizer{baseURL: server.URL, model: "m", apiKey: "key", client: server.Client()}
	if _, _, err := s.GenerateSummary("Title", "Body"); err != nil {
		t.Fatalf("expected summary success: %v", err)
	}
}
