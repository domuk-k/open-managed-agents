package tools

import (
	"context"
	"encoding/json"

	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

type GlobTool struct{}

func (t *GlobTool) Name() string        { return "glob" }
func (t *GlobTool) Description() string { return "Find files matching a glob pattern" }
func (t *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.go)"}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) Execute(ctx context.Context, input json.RawMessage, sb sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	matches, err := sb.Glob(ctx, params.Pattern)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string][]string{"files": matches})
}

type GrepTool struct{}

func (t *GrepTool) Name() string        { return "grep" }
func (t *GrepTool) Description() string { return "Search file contents with a regex pattern" }
func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Regex pattern to search for"},
			"path": {"type": "string", "description": "Directory or file to search in", "default": "."}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage, sb sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Path == "" {
		params.Path = "."
	}
	matches, err := sb.Grep(ctx, params.Pattern, params.Path)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"matches": matches})
}
