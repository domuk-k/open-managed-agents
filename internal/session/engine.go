package session

import "time"

type Session struct {
	ID            string            `json:"id"`
	Agent         string            `json:"agent"`
	AgentVersion  int               `json:"agent_version"`
	EnvironmentID string            `json:"environment_id"`
	Title         *string           `json:"title,omitempty"`
	Status        SessionStatus     `json:"status"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
}

type SessionStatus string

const (
	StatusStarting  SessionStatus = "starting"
	StatusRunning   SessionStatus = "running"
	StatusIdle      SessionStatus = "idle"
	StatusPaused    SessionStatus = "paused"
	StatusCompleted SessionStatus = "completed"
	StatusFailed    SessionStatus = "failed"
)

type CreateRequest struct {
	AgentID       string            `json:"agent_id"`
	EnvironmentID string            `json:"environment_id"`
	Title         *string           `json:"title,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}
