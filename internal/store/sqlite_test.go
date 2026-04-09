package store

import (
	"context"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/environment"
	"github.com/domuk-k/open-managed-agents/internal/session"
)

func TestCreateGetListAgents(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create an agent
	a := &agent.Agent{
		Name: "test-agent",
		Model: agent.ModelConfig{
			ID:    "claude-sonnet-4-20250514",
			Speed: "fast",
		},
		Tools: []agent.ToolConfig{
			{Type: "bash"},
		},
	}

	if err := store.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if a.ID == "" {
		t.Fatal("expected agent ID to be set")
	}
	if a.Version != 1 {
		t.Fatalf("expected version 1, got %d", a.Version)
	}

	// Get the agent back
	got, err := store.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.Name != "test-agent" {
		t.Fatalf("expected name 'test-agent', got %q", got.Name)
	}
	if got.Model.ID != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model ID 'claude-sonnet-4-20250514', got %q", got.Model.ID)
	}
	if len(got.Tools) != 1 || got.Tools[0].Type != "bash" {
		t.Fatalf("unexpected tools: %+v", got.Tools)
	}
	if got.Version != 1 {
		t.Fatalf("expected version 1, got %d", got.Version)
	}

	// Create a second agent
	a2 := &agent.Agent{
		Name:  "second-agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}
	if err := store.CreateAgent(ctx, a2); err != nil {
		t.Fatalf("CreateAgent (second): %v", err)
	}

	// List agents
	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	// Archive first agent, list should return 1
	if err := store.ArchiveAgent(ctx, a.ID); err != nil {
		t.Fatalf("ArchiveAgent: %v", err)
	}
	agents, err = store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents after archive: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent after archive, got %d", len(agents))
	}

	// GetAgent on archived should fail
	_, err = store.GetAgent(ctx, a.ID)
	if err == nil {
		t.Fatal("expected error getting archived agent")
	}
}

func TestUpdateAgentOptimisticLock(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	a := &agent.Agent{
		Name:  "lock-test",
		Model: agent.ModelConfig{ID: "m1"},
	}
	if err := store.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Update with correct version
	a.Name = "lock-test-updated"
	if err := store.UpdateAgent(ctx, a, 1); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if a.Version != 2 {
		t.Fatalf("expected version 2 after update, got %d", a.Version)
	}

	// Update with stale version should fail
	a.Name = "should-fail"
	if err := store.UpdateAgent(ctx, a, 1); err == nil {
		t.Fatal("expected optimistic lock error")
	}
}

func TestCreateGetListEnvironments(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	e := &environment.Environment{
		Name: "test-env",
		Config: environment.EnvironmentConfig{
			Type: "docker",
			Networking: environment.NetworkConfig{
				Type: "full",
			},
		},
	}

	if err := store.CreateEnvironment(ctx, e); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if e.ID == "" {
		t.Fatal("expected environment ID to be set")
	}

	// Get
	got, err := store.GetEnvironment(ctx, e.ID)
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if got.Name != "test-env" {
		t.Fatalf("expected name 'test-env', got %q", got.Name)
	}
	if got.Config.Type != "docker" {
		t.Fatalf("expected config type 'docker', got %q", got.Config.Type)
	}

	// Create second
	e2 := &environment.Environment{
		Name:   "second-env",
		Config: environment.EnvironmentConfig{Type: "local"},
	}
	if err := store.CreateEnvironment(ctx, e2); err != nil {
		t.Fatalf("CreateEnvironment (second): %v", err)
	}

	// List
	envs, err := store.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(envs))
	}

	// Archive first, list should return 1
	if err := store.ArchiveEnvironment(ctx, e.ID); err != nil {
		t.Fatalf("ArchiveEnvironment: %v", err)
	}
	envs, err = store.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments after archive: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 environment after archive, got %d", len(envs))
	}
}

func TestArchiveEnvironment(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	e := &environment.Environment{
		Name:   "archive-test",
		Config: environment.EnvironmentConfig{Type: "docker"},
	}
	if err := store.CreateEnvironment(ctx, e); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	if err := store.ArchiveEnvironment(ctx, e.ID); err != nil {
		t.Fatalf("ArchiveEnvironment: %v", err)
	}

	// GetEnvironment on archived should fail
	_, err = store.GetEnvironment(ctx, e.ID)
	if err == nil {
		t.Fatal("expected error getting archived environment")
	}

	// Not in list
	envs, err := store.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected 0 environments after archive, got %d", len(envs))
	}

	// Archive again should fail
	if err := store.ArchiveEnvironment(ctx, e.ID); err == nil {
		t.Fatal("expected error archiving already-archived environment")
	}
}

func TestCreateGetListSessions(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	title := "test session"
	sess := &session.Session{
		Agent:         "agent-1",
		AgentVersion:  1,
		EnvironmentID: "env-1",
		Title:         &title,
		Metadata:      map[string]string{"key": "value"},
	}

	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected session ID to be set")
	}
	if sess.Status != session.StatusStarting {
		t.Fatalf("expected status 'starting', got %q", sess.Status)
	}

	// Get
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Agent != "agent-1" {
		t.Fatalf("expected agent 'agent-1', got %q", got.Agent)
	}
	if got.Title == nil || *got.Title != "test session" {
		t.Fatalf("expected title 'test session', got %v", got.Title)
	}
	if got.Metadata["key"] != "value" {
		t.Fatalf("expected metadata key=value, got %v", got.Metadata)
	}

	// Create second
	sess2 := &session.Session{
		Agent:         "agent-2",
		AgentVersion:  1,
		EnvironmentID: "env-2",
	}
	if err := store.CreateSession(ctx, sess2); err != nil {
		t.Fatalf("CreateSession (second): %v", err)
	}

	// List
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Update status
	if err := store.UpdateSessionStatus(ctx, sess.ID, session.StatusRunning); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, err = store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession after status update: %v", err)
	}
	if got.Status != session.StatusRunning {
		t.Fatalf("expected status 'running', got %q", got.Status)
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	sess := &session.Session{
		Agent:         "agent-1",
		AgentVersion:  1,
		EnvironmentID: "env-1",
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Update through multiple statuses
	for _, status := range []session.SessionStatus{
		session.StatusRunning,
		session.StatusPaused,
		session.StatusCompleted,
	} {
		if err := store.UpdateSessionStatus(ctx, sess.ID, status); err != nil {
			t.Fatalf("UpdateSessionStatus to %s: %v", status, err)
		}
		got, err := store.GetSession(ctx, sess.ID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.Status != status {
			t.Fatalf("expected status %q, got %q", status, got.Status)
		}
	}
}

func TestSaveGetMessages(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	sessionID := "sess-msg-test"

	// Save messages
	msgs := []byte(`[{"role":"user","content":"hello"}]`)
	if err := store.SaveMessages(ctx, sessionID, msgs); err != nil {
		t.Fatalf("SaveMessages: %v", err)
	}

	// Get messages
	got, err := store.GetMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if string(got) != string(msgs) {
		t.Fatalf("expected %s, got %s", msgs, got)
	}

	// Upsert (save again with new data)
	msgs2 := []byte(`[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]`)
	if err := store.SaveMessages(ctx, sessionID, msgs2); err != nil {
		t.Fatalf("SaveMessages (upsert): %v", err)
	}

	got, err = store.GetMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetMessages after upsert: %v", err)
	}
	if string(got) != string(msgs2) {
		t.Fatalf("expected %s, got %s", msgs2, got)
	}

	// Get messages for non-existent session should fail
	_, err = store.GetMessages(ctx, "non-existent")
	if err == nil {
		t.Fatal("expected error getting messages for non-existent session")
	}
}

func TestInsertGetEvents(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	sessionID := "sess-event-test"

	// Insert events
	e1 := &StoredEvent{
		SessionID: sessionID,
		Type:      "message",
		Data:      []byte(`{"role":"user","content":"hello"}`),
	}
	if err := store.InsertEvent(ctx, e1); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if e1.ID == "" {
		t.Fatal("expected event ID to be set")
	}

	e2 := &StoredEvent{
		SessionID: sessionID,
		Type:      "tool_call",
		Data:      []byte(`{"tool":"bash","input":"ls"}`),
	}
	if err := store.InsertEvent(ctx, e2); err != nil {
		t.Fatalf("InsertEvent (second): %v", err)
	}

	// Insert event for different session
	e3 := &StoredEvent{
		SessionID: "other-session",
		Type:      "message",
		Data:      []byte(`{"role":"user","content":"other"}`),
	}
	if err := store.InsertEvent(ctx, e3); err != nil {
		t.Fatalf("InsertEvent (other session): %v", err)
	}

	// Get events by session ID
	events, err := store.GetSessionEvents(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSessionEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "message" {
		t.Fatalf("expected first event type 'message', got %q", events[0].Type)
	}
	if events[1].Type != "tool_call" {
		t.Fatalf("expected second event type 'tool_call', got %q", events[1].Type)
	}

	// Get events for other session
	otherEvents, err := store.GetSessionEvents(ctx, "other-session")
	if err != nil {
		t.Fatalf("GetSessionEvents (other): %v", err)
	}
	if len(otherEvents) != 1 {
		t.Fatalf("expected 1 event for other session, got %d", len(otherEvents))
	}

	// Empty session should return empty slice
	empty, err := store.GetSessionEvents(ctx, "no-events")
	if err != nil {
		t.Fatalf("GetSessionEvents (empty): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 events, got %d", len(empty))
	}
}
