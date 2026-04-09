package store

import (
	"context"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
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
