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

func TestAnthropic_Chat_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected path /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key test-key, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %s", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Stream {
			t.Error("expected stream=false for Chat")
		}
		if req.System != "You are helpful." {
			t.Errorf("expected system prompt, got %q", req.System)
		}
		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("expected model claude-sonnet-4-20250514, got %s", req.Model)
		}
		if req.MaxTokens != 8192 {
			t.Errorf("expected max_tokens 8192, got %d", req.MaxTokens)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(req.Messages))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"content": [
				{"type": "text", "text": "Hello! How can I help?"}
			],
			"stop_reason": "end_turn"
		}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model:  "claude-sonnet-4-20250514",
		System: "You are helpful.",
		Messages: []Message{
			{Role: "user", Content: json.RawMessage(`"Hi there"`)},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello! How can I help?" {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
}

func TestAnthropic_Chat_ToolUseResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		json.Unmarshal(body, &req)

		if len(req.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Name != "get_weather" {
			t.Errorf("expected tool name get_weather, got %s", req.Tools[0].Name)
		}
		// Verify input_schema is used (not parameters)
		var schema map[string]interface{}
		json.Unmarshal(req.Tools[0].InputSchema, &schema)
		if schema["type"] != "object" {
			t.Errorf("expected input_schema type object, got %v", schema["type"])
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"content": [
				{"type": "tool_use", "id": "toolu_123", "name": "get_weather", "input": {"location": "Seoul"}}
			],
			"stop_reason": "tool_use"
		}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model: "claude-sonnet-4-20250514",
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
	if tc.ID != "toolu_123" {
		t.Errorf("expected ID toolu_123, got %s", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("expected type function, got %s", tc.Type)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather, got %s", tc.Function.Name)
	}

	var args map[string]string
	if err := json.Unmarshal(tc.Function.Arguments, &args); err != nil {
		t.Fatalf("failed to unmarshal arguments: %v", err)
	}
	if args["location"] != "Seoul" {
		t.Errorf("expected location Seoul, got %s", args["location"])
	}
}

func TestAnthropic_Chat_ToolResultConversion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		json.Unmarshal(body, &req)

		// Should have 3 messages: user, assistant (with tool_use), user (with tool_result)
		if len(req.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(req.Messages))
		}

		// Verify tool result message
		var blocks []contentBlock
		json.Unmarshal(req.Messages[2].Content, &blocks)
		if len(blocks) != 1 {
			t.Fatalf("expected 1 content block in tool result, got %d", len(blocks))
		}
		if blocks[0].Type != "tool_result" {
			t.Errorf("expected type tool_result, got %s", blocks[0].Type)
		}
		if blocks[0].ToolUseID != "toolu_123" {
			t.Errorf("expected tool_use_id toolu_123, got %s", blocks[0].ToolUseID)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"content": [{"type": "text", "text": "The weather in Seoul is sunny."}],
			"stop_reason": "end_turn"
		}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")

	toolArgs, _ := json.Marshal(map[string]string{"location": "Seoul"})
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather in Seoul?"`)},
			{
				Role:    "assistant",
				Content: json.RawMessage(`""`),
				ToolCalls: []ToolCall{
					{
						ID:   "toolu_123",
						Type: "function",
						Function: FunctionCall{
							Name:      "get_weather",
							Arguments: toolArgs,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "toolu_123",
				Content:    json.RawMessage(`"Sunny, 25°C"`),
			},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "The weather in Seoul is sunny." {
		t.Errorf("unexpected content: %s", resp.Content)
	}
}

func TestAnthropic_Chat_ModelPrefixStrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		json.Unmarshal(body, &req)

		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("expected model without prefix, got %s", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content": [{"type": "text", "text": "ok"}], "stop_reason": "end_turn"}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	_, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "anthropic/claude-sonnet-4-20250514",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"test"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnthropic_Chat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"rate limit exceeded"}}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	_, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if err == nil {
		t.Fatal("expected error for 429 status")
	}
	if got := err.Error(); !contains(got, "429") {
		t.Errorf("expected error to contain 429, got: %s", got)
	}
}

func TestAnthropic_Stream_Text(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		json.Unmarshal(body, &req)

		if !req.Stream {
			t.Error("expected stream=true for Stream")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"!\"}}\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}

		for _, ev := range events {
			fmt.Fprint(w, ev)
			fmt.Fprintln(w) // blank line separator
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	ch, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-20250514",
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

func TestAnthropic_Stream_ToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_abc\",\"name\":\"bash\",\"input\":{}}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"cmd\\\"\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\": \\\"ls\\\"}\"}}\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}

		for _, ev := range events {
			fmt.Fprint(w, ev)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	ch, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-20250514",
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
	if tc.ID != "toolu_abc" {
		t.Errorf("expected ID toolu_abc, got %s", tc.ID)
	}
	if tc.Function.Name != "bash" {
		t.Errorf("expected function name bash, got %s", tc.Function.Name)
	}
	if string(tc.Function.Arguments) != `{"cmd": "ls"}` {
		t.Errorf("expected arguments {\"cmd\": \"ls\"}, got %s", string(tc.Function.Arguments))
	}
}

func TestAnthropic_Stream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"type":"server_error","message":"internal"}}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key")
	_, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if got := err.Error(); !contains(got, "500") {
		t.Errorf("expected error to contain 500, got: %s", got)
	}
}

func TestAnthropic_DefaultBaseURL(t *testing.T) {
	provider := NewAnthropicProvider("", "test-key")
	if provider.baseURL != "https://api.anthropic.com" {
		t.Errorf("expected default base URL, got %s", provider.baseURL)
	}
}
