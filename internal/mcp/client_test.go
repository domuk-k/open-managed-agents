package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
)

func TestListTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Method != "tools/list" {
			t.Fatalf("expected method tools/list, got %s", req.Method)
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		result, _ := json.Marshal(toolsListResult{
			Tools: []mcpTool{
				{
					Name:        "search",
					Description: "Search the web",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
				},
				{
					Name:        "fetch",
					Description: "Fetch a URL",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
				},
			},
		})
		resp.Result = result

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(agent.McpServerConfig{
		Name: "test",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	if tools[0].Function.Name != "search" {
		t.Errorf("expected tool name 'search', got %q", tools[0].Function.Name)
	}
	if tools[0].Function.Description != "Search the web" {
		t.Errorf("expected description 'Search the web', got %q", tools[0].Function.Description)
	}
	if tools[1].Function.Name != "fetch" {
		t.Errorf("expected tool name 'fetch', got %q", tools[1].Function.Name)
	}
}

func TestCallTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if req.Method != "tools/call" {
			t.Fatalf("expected method tools/call, got %s", req.Method)
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		result, _ := json.Marshal(callToolResult{
			Content: []mcpContent{
				{Type: "text", Text: "Hello from MCP tool!"},
			},
		})
		resp.Result = result

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(agent.McpServerConfig{
		Name: "test",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	args, _ := json.Marshal(map[string]string{"query": "hello"})
	result, err := client.CallTool(context.Background(), "search", args)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var output struct {
		Content []string `json:"content"`
	}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(output.Content) != 1 || output.Content[0] != "Hello from MCP tool!" {
		t.Errorf("unexpected content: %v", output.Content)
	}
}

func TestCallToolJSONRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32601,
				Message: "Method not found",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(agent.McpServerConfig{
		Name: "test",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.CallTool(context.Background(), "nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedMsg := "JSON-RPC error -32601: Method not found"
	if !containsString(err.Error(), expectedMsg) {
		t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
	}
}

func TestNewClientMissingURL(t *testing.T) {
	_, err := NewClient(agent.McpServerConfig{
		Name: "test",
	})
	if err == nil {
		t.Fatal("expected error for missing URL, got nil")
	}
}

func TestCallToolHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client, err := NewClient(agent.McpServerConfig{
		Name: "test",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.CallTool(context.Background(), "search", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestCallToolInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client, err := NewClient(agent.McpServerConfig{
		Name: "test",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.CallTool(context.Background(), "search", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
