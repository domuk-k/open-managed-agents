package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebFetch_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test")
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{}
	input, _ := json.Marshal(map[string]any{"url": srv.URL})
	result, err := tool.Execute(nil, input, nil)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	json.Unmarshal(result, &out)

	if out["status"].(float64) != 200 {
		t.Errorf("expected status 200, got %v", out["status"])
	}
	if out["body"].(string) != "hello world" {
		t.Errorf("unexpected body: %v", out["body"])
	}
	if out["truncated"].(bool) != false {
		t.Error("should not be truncated")
	}
}

func TestWebFetch_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{}
	input, _ := json.Marshal(map[string]any{"url": srv.URL})
	result, err := tool.Execute(nil, input, nil)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	json.Unmarshal(result, &out)
	if out["status"].(float64) != 404 {
		t.Errorf("expected 404, got %v", out["status"])
	}
}

func TestWebSearch_NotConfigured(t *testing.T) {
	t.Setenv("OMA_SEARCH_API_URL", "")

	tool := &WebSearchTool{}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := tool.Execute(nil, input, nil)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]string
	json.Unmarshal(result, &out)
	if out["error"] == "" {
		t.Error("expected error about not configured")
	}
}

func TestWebSearch_MockAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{"title": "Test Result", "url": "https://example.com", "snippet": "A test result"},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("OMA_SEARCH_API_URL", srv.URL)
	t.Setenv("OMA_SEARCH_API_KEY", "test-key")

	tool := &WebSearchTool{}
	input, _ := json.Marshal(map[string]any{"query": "test query", "num_results": 3})
	result, err := tool.Execute(nil, input, nil)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	json.Unmarshal(result, &out)
	results := out["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}
