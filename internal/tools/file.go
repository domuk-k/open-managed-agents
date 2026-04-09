package tools

import (
	"context"
	"encoding/json"

	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

// FileReadTool reads a file from the sandbox.
type FileReadTool struct{}

func (t *FileReadTool) Name() string        { return "file_read" }
func (t *FileReadTool) Description() string { return "Read a file from the sandbox filesystem" }
func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the file"}
		},
		"required": ["path"]
	}`)
}

func (t *FileReadTool) Execute(ctx context.Context, input json.RawMessage, sb sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	content, err := sb.ReadFile(ctx, params.Path)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"content": string(content)})
}

// FileWriteTool writes a file to the sandbox.
type FileWriteTool struct{}

func (t *FileWriteTool) Name() string        { return "file_write" }
func (t *FileWriteTool) Description() string { return "Write content to a file in the sandbox" }
func (t *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the file"},
			"content": {"type": "string", "description": "Content to write"}
		},
		"required": ["path", "content"]
	}`)
}

func (t *FileWriteTool) Execute(ctx context.Context, input json.RawMessage, sb sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if err := sb.WriteFile(ctx, params.Path, []byte(params.Content)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

// FileEditTool performs string replacement edits on a file.
type FileEditTool struct{}

func (t *FileEditTool) Name() string        { return "file_edit" }
func (t *FileEditTool) Description() string { return "Edit a file by replacing a string" }
func (t *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the file"},
			"old_string": {"type": "string", "description": "The text to find and replace"},
			"new_string": {"type": "string", "description": "The replacement text"}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *FileEditTool) Execute(ctx context.Context, input json.RawMessage, sb sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	content, err := sb.ReadFile(ctx, params.Path)
	if err != nil {
		return nil, err
	}

	updated := []byte(replaceFirst(string(content), params.OldString, params.NewString))
	if err := sb.WriteFile(ctx, params.Path, updated); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"status": "ok"})
}

func replaceFirst(s, old, new string) string {
	i := len(s)
	for j := 0; j+len(old) <= len(s); j++ {
		if s[j:j+len(old)] == old {
			i = j
			break
		}
	}
	if i == len(s) {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}
