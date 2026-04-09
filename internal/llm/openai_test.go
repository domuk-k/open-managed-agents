package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChat_SimpleResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request shape
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", auth)
		}

		body, _ := io.ReadAll(r.Body)
		var req openaiRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Stream {
			t.Error("expected stream=false for Chat")
		}
		// Should have system message + user message
		if len(req.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message role=system, got %s", req.Messages[0].Role)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Hello! How can I help you?"
				}
			}]
		}`)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key")
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model:  "gpt-4",
		System: "You are a helpful assistant.",
		Messages: []Message{
			{Role: "user", Content: json.RawMessage(`"Hi there"`)},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
}

func TestChat_NoAuthWhenKeyEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "")
	_, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "local-model",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"test"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChat_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req openaiRequest
		json.Unmarshal(body, &req)

		// Verify tools were sent
		if len(req.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "get_weather" {
			t.Errorf("expected tool name get_weather, got %s", req.Tools[0].Function.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call_123",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"location\":\"Seoul\"}"
						}
					}]
				}
			}]
		}`)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key")
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather in Seoul?"`)},
		},
		Tools: []ToolDef{
			{
				Type: "function",
				Function: FunctionDef{
					Name:        "get_weather",
					Description: "Get weather for a location",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
				},
			},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got %s", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("expected tool call ID call_123, got %s", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather, got %s", tc.Function.Name)
	}

	// OpenAI returns arguments as a JSON string, so RawMessage contains a quoted string.
	// First unmarshal the string, then parse the inner JSON.
	var argsStr string
	if err := json.Unmarshal(tc.Function.Arguments, &argsStr); err != nil {
		t.Fatalf("failed to unmarshal arguments string: %v", err)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		t.Fatalf("failed to unmarshal inner arguments: %v", err)
	}
	if args["location"] != "Seoul" {
		t.Errorf("expected location Seoul, got %s", args["location"])
	}
}

func TestChat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limit exceeded"}`)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key")
	_, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if err == nil {
		t.Fatal("expected error for 429 status")
	}
	if got := err.Error(); !contains(got, "429") {
		t.Errorf("expected error to contain status code 429, got: %s", got)
	}
}

func TestStream_TextChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req openaiRequest
		json.Unmarshal(body, &req)

		if !req.Stream {
			t.Error("expected stream=true for Stream")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		chunks := []string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"}}]}`,
			`data: {"choices":[{"delta":{"content":"!"}}]}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			fmt.Fprintln(w) // blank line between events
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key")
	ch, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	var gotDone bool
	for chunk := range ch {
		text += chunk.Text
		if chunk.Done {
			gotDone = true
		}
	}

	if text != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", text)
	}
	if !gotDone {
		t.Error("expected Done chunk")
	}
}

func TestStream_ToolCallAccumulation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		chunks := []string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"bash","arguments":""}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\""}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"ls\"}"}}]}}]}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key")
	ch, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"list files"`)}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var lastChunk StreamChunk
	for chunk := range ch {
		if chunk.Done {
			lastChunk = chunk
		}
	}

	if len(lastChunk.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(lastChunk.ToolCalls))
	}
	tc := lastChunk.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("expected ID call_abc, got %s", tc.ID)
	}
	if tc.Function.Name != "bash" {
		t.Errorf("expected function name bash, got %s", tc.Function.Name)
	}
	if string(tc.Function.Arguments) != `{"cmd":"ls"}` {
		t.Errorf("expected arguments {\"cmd\":\"ls\"}, got %s", string(tc.Function.Arguments))
	}
}

func TestStream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal"}`)
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key")
	_, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if got := err.Error(); !contains(got, "500") {
		t.Errorf("expected error to contain 500, got: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
