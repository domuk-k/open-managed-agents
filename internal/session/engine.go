package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/llm"
)

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

// ---------------------------------------------------------------------------
// SessionEngine — manages running agent sessions.
// ---------------------------------------------------------------------------

type sessionEntry struct {
	runner *AgentRunner
	cancel context.CancelFunc
}

// SessionEngine manages active agent sessions and their lifecycle.
type SessionEngine struct {
	bus      *EventBus
	sessions map[string]*sessionEntry
	mu       sync.RWMutex
}

// NewSessionEngine creates a SessionEngine backed by the given event bus.
func NewSessionEngine(bus *EventBus) *SessionEngine {
	return &SessionEngine{
		bus:      bus,
		sessions: make(map[string]*sessionEntry),
	}
}

// StartSession launches an AgentRunner in a background goroutine.
func (e *SessionEngine) StartSession(ctx context.Context, sessionID string, runner *AgentRunner, initialMessages []llm.Message) error {
	e.mu.Lock()
	if _, exists := e.sessions[sessionID]; exists {
		e.mu.Unlock()
		return fmt.Errorf("session %s already running", sessionID)
	}

	runCtx, cancel := context.WithCancel(ctx)
	e.sessions[sessionID] = &sessionEntry{runner: runner, cancel: cancel}
	e.mu.Unlock()

	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.sessions, sessionID)
			e.mu.Unlock()
		}()

		_ = runner.Run(runCtx, sessionID, initialMessages)
	}()

	return nil
}

// Subscribe returns an event channel for the given session.
func (e *SessionEngine) Subscribe(sessionID string) <-chan Event {
	return e.bus.Subscribe(sessionID)
}

// Unsubscribe removes the channel from the session's subscribers.
func (e *SessionEngine) Unsubscribe(sessionID string, ch <-chan Event) {
	e.bus.Unsubscribe(sessionID, ch)
}

// SendMessage injects a user message into an active runner session.
func (e *SessionEngine) SendMessage(_ context.Context, sessionID string, content []ContentBlock) error {
	e.mu.RLock()
	entry, ok := e.sessions[sessionID]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not running", sessionID)
	}

	raw, _ := json.Marshal(content)
	msg := llm.Message{
		Role:    "user",
		Content: raw,
	}

	select {
	case entry.runner.InCh() <- msg:
		return nil
	default:
		return fmt.Errorf("session %s message channel full", sessionID)
	}
}

// StopSession cancels a running session.
func (e *SessionEngine) StopSession(sessionID string) error {
	e.mu.RLock()
	entry, ok := e.sessions[sessionID]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not running", sessionID)
	}
	entry.cancel()
	return nil
}

// ResumeSession loads persisted messages and relaunches the agent runner.
func (e *SessionEngine) ResumeSession(ctx context.Context, sessionID string, runner *AgentRunner, loadMessages func(ctx context.Context, sessionID string) ([]llm.Message, error)) error {
	messages, err := loadMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load messages for session %s: %w", sessionID, err)
	}
	return e.StartSession(ctx, sessionID, runner, messages)
}
