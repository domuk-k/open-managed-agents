package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type omaClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newClient() *omaClient {
	baseURL := os.Getenv("OMA_SERVER_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	apiKey := os.Getenv("OMA_API_KEY")

	return &omaClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *omaClient) do(req *http.Request) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *omaClient) get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *omaClient) post(path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

// getStream returns the http.Response directly for SSE streaming.
// The caller is responsible for closing the response body.
func (c *omaClient) getStream(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Use a client without timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
