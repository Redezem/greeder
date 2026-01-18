package main

import (
	"errors"
	"net/http"
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
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/chat/completions") {
			return newResponse(http.StatusNotFound, "", nil, r), nil
		}
		return newResponse(http.StatusOK, `{"choices":[{"message":{"content":"- one\n- two"}}]}`, map[string]string{"content-type": "application/json"}, r), nil
	})}

	os.Setenv("LM_BASE_URL", "http://example.test")
	os.Setenv("LM_MODEL", "test-model")
	defer os.Unsetenv("LM_BASE_URL")
	defer os.Unsetenv("LM_MODEL")

	summarizer := NewSummarizerFromEnv()
	if summarizer == nil {
		t.Fatalf("expected summarizer")
	}
	summarizer.client = client
	content, model, err := summarizer.GenerateSummary("Title", strings.Repeat("a", 20001))
	if err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
	if model != "test-model" || !strings.Contains(content, "one") {
		t.Fatalf("unexpected summary: %s %s", model, content)
	}
}

func TestSummarizerErrors(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return newResponse(http.StatusBadRequest, "", nil, r), nil
	})}
	os.Setenv("LM_BASE_URL", "http://example.test")
	defer os.Unsetenv("LM_BASE_URL")
	summarizer := NewSummarizerFromEnv()
	if summarizer == nil {
		t.Fatalf("expected summarizer")
	}
	summarizer.client = client
	if _, _, err := summarizer.GenerateSummary("Title", "Body"); err == nil {
		t.Fatalf("expected http error")
	}

	clientEmpty := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return newResponse(http.StatusOK, `{"choices":[]}`, map[string]string{"content-type": "application/json"}, r), nil
	})}
	os.Setenv("LM_BASE_URL", "http://example.test")
	summarizer = NewSummarizerFromEnv()
	summarizer.client = clientEmpty
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
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			return newResponse(http.StatusNotFound, "", nil, r), nil
		}
		return newResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}, r), nil
	})}
	s := &Summarizer{baseURL: "http://example.test/v1", model: "m", client: client}
	if _, _, err := s.GenerateSummary("Title", "Body"); err != nil {
		t.Fatalf("GenerateSummary error: %v", err)
	}
}

func TestSummarizerDecodeError(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return newResponse(http.StatusOK, "not-json", map[string]string{"content-type": "application/json"}, r), nil
	})}
	s := &Summarizer{baseURL: "http://example.test", model: "m", client: client}
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
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("authorization"); got != "Bearer key" {
			return newResponse(http.StatusUnauthorized, "", nil, r), nil
		}
		return newResponse(http.StatusOK, `{"choices":[{"message":{"content":"- ok"}}]}`, map[string]string{"content-type": "application/json"}, r), nil
	})}
	s := &Summarizer{baseURL: "http://example.test", model: "m", apiKey: "key", client: client}
	if _, _, err := s.GenerateSummary("Title", "Body"); err != nil {
		t.Fatalf("expected summary success: %v", err)
	}
}
