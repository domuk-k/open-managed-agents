package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

type BashTool struct{}

func (t *BashTool) Name() string        { return "bash" }
func (t *BashTool) Description() string { return "Run a shell command in the sandbox" }
func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Shell command to execute"},
			"timeout": {"type": "integer", "description": "Timeout in seconds", "default": 120}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage, sb sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	timeout := time.Duration(params.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	result, err := sb.Exec(ctx, params.Command, sandbox.ExecOpts{Timeout: timeout})
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}
