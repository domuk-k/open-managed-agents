package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
)

// Client communicates with an MCP server over HTTP (streamable HTTP transport)
// using JSON-RPC 2.0.
type Client struct {
	name    string
	baseURL string
	apiKey  string
	http    *http.Client
	nextID  atomic.Int64
}

// NewClient creates an MCP client from the given server configuration.
func NewClient(config agent.McpServerConfig) (*Client, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("mcp: server URL is required")
	}

	c := &Client{
		name:    config.Name,
		baseURL: config.URL,
		http:    &http.Client{},
	}

	// Extract API key from auth config if present.
	if len(config.Auth) > 0 {
		var auth struct {
			APIKey string `json:"api_key"`
		}
		if err := json.Unmarshal(config.Auth, &auth); err == nil {
			c.apiKey = auth.APIKey
		}
	}

	return c, nil
}

// Name returns the configured server name.
func (c *Client) Name() string { return c.name }

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	ID      int64       `json:"id"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// call sends a JSON-RPC 2.0 request and returns the result.
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      id,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mcp: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcp: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcp: server returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

// mcpTool represents a tool as returned by the MCP tools/list method.
type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// toolsListResult represents the result of the tools/list method.
type toolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

// ListTools calls the MCP tools/list method and returns the tools converted
// to the OMA ToolDef format.
func (c *Client) ListTools(ctx context.Context) ([]llm.ToolDef, error) {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: list tools: %w", err)
	}

	var listResult toolsListResult
	if err := json.Unmarshal(result, &listResult); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal tools list: %w", err)
	}

	defs := make([]llm.ToolDef, 0, len(listResult.Tools))
	for _, t := range listResult.Tools {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return defs, nil
}

// callToolParams represents the parameters for the tools/call method.
type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// mcpContent represents a content item in the tools/call response.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callToolResult represents the result of the tools/call method.
type callToolResult struct {
	Content []mcpContent `json:"content"`
}

// CallTool calls the MCP tools/call method with the given tool name and arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	params := callToolParams{
		Name:      name,
		Arguments: arguments,
	}

	result, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: call tool %q: %w", name, err)
	}

	var callResult callToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal tool result: %w", err)
	}

	// Collect all text content pieces.
	var texts []string
	for _, c := range callResult.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}

	// Return as a JSON object with the content.
	output, err := json.Marshal(map[string]interface{}{
		"content": texts,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal output: %w", err)
	}

	return output, nil
}
