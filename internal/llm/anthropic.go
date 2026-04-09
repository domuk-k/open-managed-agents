package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicProvider implements the Provider interface using the native Anthropic Messages API.
type AnthropicProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
	model   string
}

func NewAnthropicProvider(baseURL, apiKey string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{},
		model:   "claude-sonnet-4-20250514",
	}
}

// --- Anthropic API request/response types ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	Content []contentBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
}

// SSE event types

type anthropicSSEEvent struct {
	Type         string          `json:"type"`
	Index        int             `json:"index,omitempty"`
	ContentBlock *contentBlock   `json:"content_block,omitempty"`
	Delta        *anthropicDelta `json:"delta,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// --- Conversion helpers ---

func (p *AnthropicProvider) buildMessages(req ChatRequest) []anthropicMessage {
	var msgs []anthropicMessage

	for _, m := range req.Messages {
		if m.Role == "tool" {
			// Convert OMA tool result message to Anthropic format
			var blocks []contentBlock
			blocks = append(blocks, contentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   unquoteJSON(m.Content),
			})
			raw, _ := json.Marshal(blocks)
			msgs = append(msgs, anthropicMessage{
				Role:    "user",
				Content: raw,
			})
			continue
		}

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Convert assistant message with tool calls to Anthropic content blocks
			var blocks []contentBlock
			// Add text if present
			text := unquoteJSON(m.Content)
			if text != "" {
				blocks = append(blocks, contentBlock{
					Type: "text",
					Text: text,
				})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: tc.Function.Arguments,
				})
			}
			raw, _ := json.Marshal(blocks)
			msgs = append(msgs, anthropicMessage{
				Role:    "assistant",
				Content: raw,
			})
			continue
		}

		// Regular user/assistant message — pass content through
		msgs = append(msgs, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return msgs
}

func (p *AnthropicProvider) buildTools(tools []ToolDef) []anthropicTool {
	var result []anthropicTool
	for _, t := range tools {
		result = append(result, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return result
}

func (p *AnthropicProvider) buildAPIRequest(req ChatRequest, stream bool) anthropicRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}
	// Strip provider prefix if present
	model = strings.TrimPrefix(model, "anthropic/")

	ar := anthropicRequest{
		Model:     model,
		MaxTokens: 8192,
		System:    req.System,
		Messages:  p.buildMessages(req),
		Stream:    stream,
	}
	if len(req.Tools) > 0 {
		ar.Tools = p.buildTools(req.Tools)
	}
	return ar
}

func (p *AnthropicProvider) doHTTP(ctx context.Context, body []byte) (*http.Response, error) {
	url := p.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	return p.client.Do(httpReq)
}

// unquoteJSON tries to unmarshal a json.RawMessage as a Go string.
// If it fails (e.g. it's an array or null), returns empty string.
func unquoteJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}

// --- Chat (non-streaming) ---

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*Response, error) {
	apiReq := p.buildAPIRequest(req, false)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := p.doHTTP(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return p.convertResponse(apiResp), nil
}

func (p *AnthropicProvider) convertResponse(apiResp anthropicResponse) *Response {
	result := &Response{}
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		}
	}
	return result
}

// --- Stream (SSE) ---

func (p *AnthropicProvider) Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	apiReq := p.buildAPIRequest(req, true)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := p.doHTTP(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		// Track tool calls being accumulated by index
		type tcAccum struct {
			ID   string
			Name string
			Args strings.Builder
		}
		toolCalls := make(map[int]*tcAccum)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Parse SSE: look for "event:" and "data:" lines
			if strings.HasPrefix(line, "event: ") {
				eventType := strings.TrimPrefix(line, "event: ")

				if eventType == "message_stop" {
					// Emit accumulated tool calls if any
					if len(toolCalls) > 0 {
						var tcs []ToolCall
						for _, tc := range toolCalls {
							tcs = append(tcs, ToolCall{
								ID:   tc.ID,
								Type: "function",
								Function: FunctionCall{
									Name:      tc.Name,
									Arguments: json.RawMessage(tc.Args.String()),
								},
							})
						}
						select {
						case ch <- StreamChunk{ToolCalls: tcs, Done: true}:
						case <-ctx.Done():
							return
						}
					} else {
						select {
						case ch <- StreamChunk{Done: true}:
						case <-ctx.Done():
							return
						}
					}
					return
				}
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			var event anthropicSSEEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_start":
				if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					toolCalls[event.Index] = &tcAccum{
						ID:   event.ContentBlock.ID,
						Name: event.ContentBlock.Name,
					}
				}

			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				switch event.Delta.Type {
				case "text_delta":
					if event.Delta.Text != "" {
						select {
						case ch <- StreamChunk{Text: event.Delta.Text}:
						case <-ctx.Done():
							return
						}
					}
				case "input_json_delta":
					if acc, ok := toolCalls[event.Index]; ok {
						acc.Args.WriteString(event.Delta.PartialJSON)
					}
				}

			case "message_stop":
				// handled above via event line, but just in case
			}
		}
	}()

	return ch, nil
}
