package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

const maxBodySize = 100 * 1024 // 100KB

// WebFetchTool fetches the content of a URL.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string { return "Fetch the content of a URL" }
func (t *WebFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "URL to fetch"},
			"timeout": {"type": "integer", "description": "Timeout in seconds", "default": 30}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(_ context.Context, input json.RawMessage, _ sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		URL     string `json:"url"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	timeout := time.Duration(params.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", params.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "OMA/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}

	truncated := len(body) > maxBodySize
	if truncated {
		body = body[:maxBodySize]
	}

	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return json.Marshal(map[string]any{
		"status":    resp.StatusCode,
		"headers":   headers,
		"body":      string(body),
		"truncated": truncated,
	})
}

// WebSearchTool searches the web using a configurable search API.
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return "Search the web using a search engine" }
func (t *WebSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query"},
			"num_results": {"type": "integer", "description": "Number of results", "default": 5}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) Execute(_ context.Context, input json.RawMessage, _ sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Query      string `json:"query"`
		NumResults int    `json:"num_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.NumResults == 0 {
		params.NumResults = 5
	}

	apiURL := os.Getenv("OMA_SEARCH_API_URL")
	apiKey := os.Getenv("OMA_SEARCH_API_KEY")
	if apiURL == "" {
		return json.Marshal(map[string]string{
			"error": "web_search not configured. Set OMA_SEARCH_API_URL and OMA_SEARCH_API_KEY environment variables.",
		})
	}

	reqBody, _ := json.Marshal(map[string]any{
		"query":       params.Query,
		"num_results": params.NumResults,
	})

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return json.Marshal(map[string]any{"error": err.Error()})
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return json.Marshal(map[string]any{"error": fmt.Sprintf("search API error (status %d): %s", resp.StatusCode, string(body))})
	}

	return body, nil
}
