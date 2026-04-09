package llm

import (
	"context"
	"encoding/json"
)

type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*Response, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
}

type ChatRequest struct {
	Model    string    `json:"model"`
	System   string    `json:"system,omitempty"`
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools,omitempty"`
}

type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function FunctionCall    `json:"function"`
}

type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolResult struct {
	ID      string          `json:"tool_call_id"`
	Content json.RawMessage `json:"content"`
}

type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Response struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type StreamChunk struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Done      bool       `json:"done"`
}

func (r *Response) Accumulate(chunk StreamChunk) {
	r.Content += chunk.Text
	// Tool call accumulation handled by caller for streaming deltas
}

func (r *Response) ToAssistantMessage() Message {
	msg := Message{
		Role:      "assistant",
		ToolCalls: r.ToolCalls,
	}
	if r.Content != "" {
		content, _ := json.Marshal(r.Content)
		msg.Content = content
	}
	return msg
}
