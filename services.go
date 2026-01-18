package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type RaindropClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type RaindropItem struct {
	Link  string   `json:"link"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Note  string   `json:"note"`
}

type raindropResponse struct {
	Item struct {
		ID int `json:"_id"`
	} `json:"item"`
}

var servicesJSONMarshal = json.Marshal

func NewRaindropClient(token string) *RaindropClient {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	base := strings.TrimSpace(os.Getenv("RAINDROP_BASE_URL"))
	if base == "" {
		base = "https://api.raindrop.io"
	}
	return &RaindropClient{
		baseURL: strings.TrimRight(base, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *RaindropClient) Save(item RaindropItem) (int, error) {
	if r == nil {
		return 0, errors.New("raindrop not configured")
	}
	blob, err := servicesJSONMarshal(item)
	if err != nil {
		return 0, err
	}
	endpoint := r.baseURL + "/rest/v1/raindrop"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(blob))
	if err != nil {
		return 0, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+r.token)
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, errors.New("raindrop http error")
	}
	var parsed raindropResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, err
	}
	return parsed.Item.ID, nil
}

func defaultOpenURL(target string) error {
	return defaultOpenURLForOS(runtime.GOOS, target)
}

func defaultOpenURLForOS(goos string, target string) error {
	if target == "" {
		return errors.New("empty url")
	}
	cmdName, args := openCommandForOS(goos, target)
	if cmdName == "" {
		return errors.New("unsupported platform")
	}
	cmd := exec.Command(cmdName, args...)
	return cmd.Start()
}

func defaultSendEmail(mailto string) error {
	return defaultOpenURL(mailto)
}

func openCommand(target string) (string, []string) {
	return openCommandForOS(runtime.GOOS, target)
}

func openCommandForOS(goos string, target string) (string, []string) {
	if goos == "unsupported" {
		return "", nil
	}
	switch goos {
	case "darwin":
		return "open", []string{target}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return "xdg-open", []string{target}
	}
}
