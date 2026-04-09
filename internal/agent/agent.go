package agent

import (
	"encoding/json"
	"time"
)

type Agent struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Model          ModelConfig       `json:"model"`
	System         *string           `json:"system,omitempty"`
	Tools          []ToolConfig      `json:"tools"`
	McpServers     []McpServerConfig `json:"mcp_servers,omitempty"`
	Skills         []SkillConfig     `json:"skills,omitempty"`
	CallableAgents []string          `json:"callable_agents,omitempty"`
	Outcomes       []Outcome         `json:"outcomes,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Version        int               `json:"version"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	ArchivedAt     *time.Time        `json:"archived_at,omitempty"`
}

type ModelConfig struct {
	ID    string `json:"id"`
	Speed string `json:"speed,omitempty"`
}

type ToolConfig struct {
	Type          string         `json:"type"`
	DefaultConfig *DefaultConfig `json:"default_config,omitempty"`
}

type DefaultConfig struct {
	PermissionPolicy *PermissionPolicy `json:"permission_policy,omitempty"`
}

type PermissionPolicy struct {
	Type   string      `json:"type"`
	Scopes []ToolScope `json:"scopes,omitempty"`
}

type ToolScope struct {
	Tool        string       `json:"tool"`
	Allow       bool         `json:"allow"`
	Constraints *Constraints `json:"constraints,omitempty"`
}

type Constraints struct {
	Paths    []string `json:"paths,omitempty"`
	Commands []string `json:"commands,omitempty"`
	Domains  []string `json:"domains,omitempty"`
}

type McpServerConfig struct {
	Name string          `json:"name"`
	URL  string          `json:"url"`
	Auth json.RawMessage `json:"auth,omitempty"`
}

type SkillConfig struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Outcome defines a success criterion for self-evaluation of agent sessions.
type Outcome struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Criteria    string `json:"criteria"`              // natural language success criteria
	MaxRetries  *int   `json:"max_retries,omitempty"` // per-outcome retry override
}

// CreateRequest is the payload for creating a new agent.
type CreateRequest struct {
	Name           string            `json:"name"`
	Model          ModelConfig       `json:"model"`
	System         *string           `json:"system,omitempty"`
	Tools          []ToolConfig      `json:"tools,omitempty"`
	McpServers     []McpServerConfig `json:"mcp_servers,omitempty"`
	Skills         []SkillConfig     `json:"skills,omitempty"`
	CallableAgents []string          `json:"callable_agents,omitempty"`
	Outcomes       []Outcome         `json:"outcomes,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// UpdateRequest is the payload for updating an agent. Version is required for optimistic locking.
type UpdateRequest struct {
	Version        int               `json:"version"`
	Name           *string           `json:"name,omitempty"`
	Model          *ModelConfig      `json:"model,omitempty"`
	System         *string           `json:"system,omitempty"`
	Tools          []ToolConfig      `json:"tools,omitempty"`
	McpServers     []McpServerConfig `json:"mcp_servers,omitempty"`
	Skills         []SkillConfig     `json:"skills,omitempty"`
	CallableAgents []string          `json:"callable_agents,omitempty"`
	Outcomes       []Outcome         `json:"outcomes,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}
