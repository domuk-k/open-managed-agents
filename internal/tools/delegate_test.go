package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

// ---------------------------------------------------------------------------
// Mock AgentResolver
// ---------------------------------------------------------------------------

type mockResolver struct {
	agents map[string]*agent.Agent
}

func (m *mockResolver) GetAgent(_ context.Context, id string) (*agent.Agent, error) {
	ag, ok := m.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return ag, nil
}

// ---------------------------------------------------------------------------
// Helper to build a simple SubRunnerFactory
// ---------------------------------------------------------------------------

func cannedSubRunner(response string) SubRunnerFactory {
	return func(_ context.Context, ag *agent.Agent, message string) (string, error) {
		return response, nil
	}
}

func failingSubRunner(errMsg string) SubRunnerFactory {
	return func(_ context.Context, ag *agent.Agent, message string) (string, error) {
		return "", fmt.Errorf("%s", errMsg)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDelegateTool_Name(t *testing.T) {
	dt := NewDelegateTool("caller", nil, nil, nil)
	if dt.Name() != "delegate_to_agent" {
		t.Errorf("expected name 'delegate_to_agent', got %q", dt.Name())
	}
}

func TestDelegateTool_SuccessfulDelegation(t *testing.T) {
	system := "You are a helper."
	targetAgent := &agent.Agent{
		ID:    "agent-b",
		Name:  "Helper",
		Model: agent.ModelConfig{ID: "gpt-4"},
		System: &system,
	}

	resolver := &mockResolver{
		agents: map[string]*agent.Agent{
			"agent-b": targetAgent,
		},
	}

	dt := NewDelegateTool(
		"agent-a",
		[]string{"agent-b"},
		resolver,
		cannedSubRunner("The answer is 42."),
	)

	input, _ := json.Marshal(map[string]string{
		"agent_id": "agent-b",
		"message":  "What is the meaning of life?",
	})

	result, err := dt.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if resp["agent_id"] != "agent-b" {
		t.Errorf("expected agent_id 'agent-b', got %q", resp["agent_id"])
	}
	if resp["response"] != "The answer is 42." {
		t.Errorf("expected response 'The answer is 42.', got %q", resp["response"])
	}
}

func TestDelegateTool_UnauthorizedDelegation(t *testing.T) {
	resolver := &mockResolver{
		agents: map[string]*agent.Agent{
			"agent-c": {ID: "agent-c", Name: "Secret Agent"},
		},
	}

	dt := NewDelegateTool(
		"agent-a",
		[]string{"agent-b"}, // only agent-b is allowed
		resolver,
		cannedSubRunner("should not reach here"),
	)

	input, _ := json.Marshal(map[string]string{
		"agent_id": "agent-c",
		"message":  "Tell me secrets",
	})

	_, err := dt.Execute(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error for unauthorized delegation, got nil")
	}
	if !strings.Contains(err.Error(), "not authorized") {
		t.Errorf("expected 'not authorized' in error, got: %v", err)
	}
}

func TestDelegateTool_EmptyCallableAgents(t *testing.T) {
	resolver := &mockResolver{
		agents: map[string]*agent.Agent{
			"agent-b": {ID: "agent-b", Name: "Helper"},
		},
	}

	dt := NewDelegateTool(
		"agent-a",
		nil, // no callable agents
		resolver,
		cannedSubRunner("nope"),
	)

	input, _ := json.Marshal(map[string]string{
		"agent_id": "agent-b",
		"message":  "Hello",
	})

	_, err := dt.Execute(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error when callable_agents is empty")
	}
}

func TestDelegateTool_AgentNotFound(t *testing.T) {
	resolver := &mockResolver{
		agents: map[string]*agent.Agent{}, // empty store
	}

	dt := NewDelegateTool(
		"agent-a",
		[]string{"agent-b"},
		resolver,
		cannedSubRunner("nope"),
	)

	input, _ := json.Marshal(map[string]string{
		"agent_id": "agent-b",
		"message":  "Hello",
	})

	_, err := dt.Execute(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error when target agent not found")
	}
	if !strings.Contains(err.Error(), "failed to resolve") {
		t.Errorf("expected 'failed to resolve' in error, got: %v", err)
	}
}

func TestDelegateTool_SubRunnerError(t *testing.T) {
	targetAgent := &agent.Agent{
		ID:    "agent-b",
		Name:  "Broken Agent",
		Model: agent.ModelConfig{ID: "gpt-4"},
	}

	resolver := &mockResolver{
		agents: map[string]*agent.Agent{
			"agent-b": targetAgent,
		},
	}

	dt := NewDelegateTool(
		"agent-a",
		[]string{"agent-b"},
		resolver,
		failingSubRunner("model overloaded"),
	)

	input, _ := json.Marshal(map[string]string{
		"agent_id": "agent-b",
		"message":  "Hello",
	})

	_, err := dt.Execute(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error from failing sub-runner")
	}
	if !strings.Contains(err.Error(), "model overloaded") {
		t.Errorf("expected 'model overloaded' in error, got: %v", err)
	}
}

func TestDelegateTool_InvalidInput(t *testing.T) {
	dt := NewDelegateTool("agent-a", []string{"agent-b"}, nil, nil)

	_, err := dt.Execute(context.Background(), json.RawMessage(`{invalid`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
}

func TestDelegateTool_MissingFields(t *testing.T) {
	dt := NewDelegateTool("agent-a", []string{"agent-b"}, nil, nil)

	// Missing agent_id
	input, _ := json.Marshal(map[string]string{"message": "hello"})
	_, err := dt.Execute(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error for missing agent_id")
	}

	// Missing message
	input, _ = json.Marshal(map[string]string{"agent_id": "agent-b"})
	_, err = dt.Execute(context.Background(), input, nil)
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestDelegateTool_ImplementsToolInterface(t *testing.T) {
	var _ Tool = (*DelegateTool)(nil)
}

func TestNewFullToolsetWithDelegate(t *testing.T) {
	dt := NewDelegateTool("agent-a", []string{"agent-b"}, nil, nil)
	registry := NewFullToolsetWithDelegate(dt)

	// Verify the delegate tool is registered.
	tool, ok := registry.Get("delegate_to_agent")
	if !ok {
		t.Fatal("expected delegate_to_agent tool in registry")
	}
	if tool.Name() != "delegate_to_agent" {
		t.Errorf("unexpected tool name: %s", tool.Name())
	}

	// Verify standard tools are still present.
	for _, name := range []string{"bash", "file_read", "file_write", "file_edit", "glob", "grep"} {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("expected tool %q in registry", name)
		}
	}
}

// Ensure DelegateTool ignores the sandbox parameter (it's not needed for delegation).
func TestDelegateTool_IgnoresSandbox(t *testing.T) {
	system := "test"
	resolver := &mockResolver{
		agents: map[string]*agent.Agent{
			"agent-b": {ID: "agent-b", Name: "B", Model: agent.ModelConfig{ID: "m"}, System: &system},
		},
	}

	dt := NewDelegateTool("agent-a", []string{"agent-b"}, resolver, cannedSubRunner("ok"))

	// Pass a non-nil sandbox — it should be ignored without error.
	sb := &noopSandbox{}
	input, _ := json.Marshal(map[string]string{
		"agent_id": "agent-b",
		"message":  "test",
	})

	result, err := dt.Execute(context.Background(), input, sb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// noopSandbox is a minimal sandbox implementation for testing.
type noopSandbox struct{}

func (s *noopSandbox) ID() string { return "noop" }
func (s *noopSandbox) Exec(_ context.Context, _ string, _ sandbox.ExecOpts) (*sandbox.ExecResult, error) {
	return &sandbox.ExecResult{}, nil
}
func (s *noopSandbox) WriteFile(_ context.Context, _ string, _ []byte) error   { return nil }
func (s *noopSandbox) ReadFile(_ context.Context, _ string) ([]byte, error)    { return nil, nil }
func (s *noopSandbox) Glob(_ context.Context, _ string) ([]string, error)      { return nil, nil }
func (s *noopSandbox) Grep(_ context.Context, _ string, _ string) ([]sandbox.GrepMatch, error) {
	return nil, nil
}
func (s *noopSandbox) Destroy(_ context.Context) error { return nil }
