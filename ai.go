package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type Summarizer struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

var aiJSONMarshal = json.Marshal

func NewSummarizerFromEnv() *Summarizer {
	base := strings.TrimSpace(os.Getenv("LM_BASE_URL"))
	if base == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv("LM_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Summarizer{
		baseURL: strings.TrimRight(base, "/"),
		apiKey:  strings.TrimSpace(os.Getenv("LM_API_KEY")),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *Summarizer) GenerateSummary(title, content string) (string, string, error) {
	if s == nil {
		return "", "", errors.New("summarizer not configured")
	}
	content = truncateText(content, 10000)
	prompt := "Please summarize the following article:\n\nTitle: " + title + "\n\nContent:\n" + content
	payload := chatRequest{
		Model: s.model,
		Messages: []chatMessage{
			{Role: "system", Content: summarySystemPrompt()},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
	}
	blob, err := aiJSONMarshal(payload)
	if err != nil {
		return "", "", err
	}
	endpoint := s.baseURL + "/v1/chat/completions"
	if strings.Contains(s.baseURL, "/v1") {
		endpoint = s.baseURL + "/chat/completions"
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(blob))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("content-type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("authorization", "Bearer "+s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", errors.New("summarizer http error")
	}
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", "", err
	}
	if len(parsed.Choices) == 0 {
		return "", "", errors.New("empty summary response")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), s.model, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func summarySystemPrompt() string {
	return "Summarize this article as 3-5 bullet points.\n" +
		"Output ONLY the bullet points - no introductions, conclusions, or commentary.\n" +
		"Start each line with \"- \" and state one key fact or finding.\n" +
		"Never write phrases like \"Here are the key points\" or \"In summary\" - just the bullets."
}

func truncateText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	truncated := value[:max]
	for !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}
