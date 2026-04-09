package mcp

import (
	"context"
	"encoding/json"

	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

// MCPToolAdapter wraps an MCP server tool so it satisfies the tools.Tool interface.
type MCPToolAdapter struct {
	client      *Client
	name        string
	description string
	schema      json.RawMessage
}

// NewMCPToolAdapter creates a new adapter for the given MCP tool.
func NewMCPToolAdapter(client *Client, name, description string, schema json.RawMessage) *MCPToolAdapter {
	return &MCPToolAdapter{
		client:      client,
		name:        name,
		description: description,
		schema:      schema,
	}
}

func (a *MCPToolAdapter) Name() string              { return a.name }
func (a *MCPToolAdapter) Description() string        { return a.description }
func (a *MCPToolAdapter) InputSchema() json.RawMessage { return a.schema }

// Execute delegates to the MCP client's CallTool method.
// The sandbox parameter is unused since execution happens on the remote MCP server.
func (a *MCPToolAdapter) Execute(ctx context.Context, input json.RawMessage, _ sandbox.Sandbox) (json.RawMessage, error) {
	return a.client.CallTool(ctx, a.name, input)
}
