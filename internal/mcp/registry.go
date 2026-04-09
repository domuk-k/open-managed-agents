package mcp

import (
	"context"
	"fmt"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/tools"
)

// RegisterMCPTools discovers tools from each MCP server and registers them
// in the given tool registry. Each tool is registered with a prefixed name:
// mcp_{serverName}_{toolName}.
func RegisterMCPTools(ctx context.Context, registry *tools.Registry, servers []agent.McpServerConfig) error {
	for _, serverCfg := range servers {
		client, err := NewClient(serverCfg)
		if err != nil {
			return fmt.Errorf("mcp: create client for %q: %w", serverCfg.Name, err)
		}

		toolDefs, err := client.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("mcp: list tools from %q: %w", serverCfg.Name, err)
		}

		for _, td := range toolDefs {
			prefixedName := fmt.Sprintf("mcp_%s_%s", serverCfg.Name, td.Function.Name)
			adapter := NewMCPToolAdapter(
				client,
				prefixedName,
				td.Function.Description,
				td.Function.Parameters,
			)
			registry.Register(adapter)
		}
	}

	return nil
}
