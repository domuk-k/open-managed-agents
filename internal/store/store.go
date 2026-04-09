package store

import (
	"context"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/environment"
	"github.com/domuk-k/open-managed-agents/internal/session"
)

type Store interface {
	// Agents
	CreateAgent(ctx context.Context, a *agent.Agent) error
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
	ListAgents(ctx context.Context) ([]*agent.Agent, error)
	UpdateAgent(ctx context.Context, a *agent.Agent, expectedVersion int) error
	ArchiveAgent(ctx context.Context, id string) error
	CreateAgentVersion(ctx context.Context, agentID string, version int, config []byte) error
	GetAgentVersions(ctx context.Context, agentID string) ([]AgentVersion, error)

	// Environments
	CreateEnvironment(ctx context.Context, e *environment.Environment) error
	GetEnvironment(ctx context.Context, id string) (*environment.Environment, error)
	ListEnvironments(ctx context.Context) ([]*environment.Environment, error)
	ArchiveEnvironment(ctx context.Context, id string) error

	// Sessions
	CreateSession(ctx context.Context, s *session.Session) error
	GetSession(ctx context.Context, id string) (*session.Session, error)
	ListSessions(ctx context.Context) ([]*session.Session, error)
	UpdateSessionStatus(ctx context.Context, id string, status session.SessionStatus) error

	// Events
	InsertEvent(ctx context.Context, e *StoredEvent) error
	GetSessionEvents(ctx context.Context, sessionID string) ([]*StoredEvent, error)

	Close() error
}

type AgentVersion struct {
	AgentID   string `json:"agent_id"`
	Version   int    `json:"version"`
	Config    []byte `json:"config"`
	CreatedAt string `json:"created_at"`
}

type StoredEvent struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Type      string `json:"type"`
	Data      []byte `json:"data"`
	CreatedAt string `json:"created_at"`
}
