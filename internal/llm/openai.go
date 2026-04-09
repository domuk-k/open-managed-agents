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

// OpenAIProvider implements the Provider interface using the OpenAI-compatible API.
// This covers OpenAI, LM Studio, Ollama, vLLM, OpenRouter, Together, Groq, etc.
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAIProvider(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{},
	}
}

// --- OpenAI API request/response types ---

type openaiRequest struct {
	Model    string           `json:"model"`
	Messages []openaiMessage  `json:"messages"`
	Tools    []ToolDef        `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

type openaiMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message openaiAssistantMessage `json:"message"`
}

type openaiAssistantMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Streaming types

type openaiStreamChunk struct {
	Choices []openaiStreamChoice `json:"choices"`
}

type openaiStreamChoice struct {
	Delta openaiDelta `json:"delta"`
}

type openaiDelta struct {
	Role      string              `json:"role,omitempty"`
	Content   *string             `json:"content,omitempty"`
	ToolCalls []openaiDeltaTC     `json:"tool_calls,omitempty"`
}

type openaiDeltaTC struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function openaiDeltaFunc  `json:"function,omitempty"`
}

type openaiDeltaFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// --- Conversion helpers ---

func (p *OpenAIProvider) buildMessages(req ChatRequest) []openaiMessage {
	var msgs []openaiMessage

	if req.System != "" {
		sysContent, _ := json.Marshal(req.System)
		msgs = append(msgs, openaiMessage{
			Role:    "system",
			Content: sysContent,
		})
	}

	for _, m := range req.Messages {
		msgs = append(msgs, openaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
		})
	}

	return msgs
}

func (p *OpenAIProvider) buildRequest(req ChatRequest, stream bool) (openaiRequest, error) {
	return openaiRequest{
		Model:    req.Model,
		Messages: p.buildMessages(req),
		Tools:    req.Tools,
		Stream:   stream,
	}, nil
}

func (p *OpenAIProvider) doHTTP(ctx context.Context, body []byte) (*http.Response, error) {
	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	return p.client.Do(httpReq)
}

// --- Chat (non-streaming) ---

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*Response, error) {
	oaiReq, err := p.buildRequest(req, false)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(oaiReq)
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
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var oaiResp openaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(oaiResp.Choices) == 0 {
		return &Response{}, nil
	}

	choice := oaiResp.Choices[0].Message
	result := &Response{
		ToolCalls: choice.ToolCalls,
	}
	if choice.Content != nil {
		result.Content = *choice.Content
	}

	return result, nil
}

// --- Stream (SSE) ---

func (p *OpenAIProvider) Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	oaiReq, err := p.buildRequest(req, true)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(oaiReq)
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
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		// Accumulate tool calls across deltas by index
		type tcAccum struct {
			ID       string
			Type     string
			Name     string
			Args     strings.Builder
		}
		toolCalls := make(map[int]*tcAccum)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				// Emit accumulated tool calls if any
				if len(toolCalls) > 0 {
					var tcs []ToolCall
					for _, tc := range toolCalls {
						tcs = append(tcs, ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
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

			var chunk openaiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// Handle text content
			if delta.Content != nil && *delta.Content != "" {
				select {
				case ch <- StreamChunk{Text: *delta.Content}:
				case <-ctx.Done():
					return
				}
			}

			// Accumulate tool call deltas
			for _, dtc := range delta.ToolCalls {
				acc, ok := toolCalls[dtc.Index]
				if !ok {
					acc = &tcAccum{}
					toolCalls[dtc.Index] = acc
				}
				if dtc.ID != "" {
					acc.ID = dtc.ID
				}
				if dtc.Type != "" {
					acc.Type = dtc.Type
				}
				if dtc.Function.Name != "" {
					acc.Name = dtc.Function.Name
				}
				if dtc.Function.Arguments != "" {
					acc.Args.WriteString(dtc.Function.Arguments)
				}
			}
		}
	}()

	return ch, nil
}
