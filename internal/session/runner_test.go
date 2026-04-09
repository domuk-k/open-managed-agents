package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
	"github.com/domuk-k/open-managed-agents/internal/tools"
)

// ---------------------------------------------------------------------------
// Mock LLM Provider
// ---------------------------------------------------------------------------

type mockProvider struct {
	chunks []llm.StreamChunk
}

func (m *mockProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	return &llm.Response{Content: "hello"}, nil
}

func (m *mockProvider) Stream(_ context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, len(m.chunks))
	for _, c := range m.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// ---------------------------------------------------------------------------
// Mock Sandbox
// ---------------------------------------------------------------------------

type mockSandbox struct{}

func (s *mockSandbox) ID() string { return "mock-sandbox" }

func (s *mockSandbox) Exec(_ context.Context, _ string, _ sandbox.ExecOpts) (*sandbox.ExecResult, error) {
	return &sandbox.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}

func (s *mockSandbox) WriteFile(_ context.Context, _ string, _ []byte) error { return nil }
func (s *mockSandbox) ReadFile(_ context.Context, _ string) ([]byte, error) {
	return []byte("content"), nil
}
func (s *mockSandbox) Glob(_ context.Context, _ string) ([]string, error)                { return nil, nil }
func (s *mockSandbox) Grep(_ context.Context, _ string, _ string) ([]sandbox.GrepMatch, error) {
	return nil, nil
}
func (s *mockSandbox) Destroy(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunnerTextOnly(t *testing.T) {
	// A mock provider that returns two text chunks then done.
	provider := &mockProvider{
		chunks: []llm.StreamChunk{
			{Text: "Hello "},
			{Text: "World"},
			{Done: true},
		},
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	sb := &mockSandbox{}

	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "you are helpful")

	sessionID := "test-session-1"
	ch := bus.Subscribe(sessionID)

	// Run in background.
	errCh := make(chan error, 1)
	go func() {
		msgs := []llm.Message{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		}
		errCh <- runner.Run(context.Background(), sessionID, msgs)
	}()

	// Collect events with a timeout.
	var events []Event
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
			if ev.Type == "session.status_idle" {
				break loop
			}
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}

	// Check runner returned no error.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runner returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not finish")
	}

	// Verify we got agent.message events and session.status_idle.
	var gotMessage, gotIdle bool
	for _, ev := range events {
		switch ev.Type {
		case "agent.message":
			gotMessage = true
		case "session.status_idle":
			gotIdle = true
		}
	}

	if !gotMessage {
		t.Error("expected at least one agent.message event")
	}
	if !gotIdle {
		t.Error("expected session.status_idle event")
	}
}

func TestRunnerWithToolCalls(t *testing.T) {
	// First call: LLM returns a tool call.
	// Second call: LLM returns text only (no tool calls).
	callCount := 0
	provider := &multiCallProvider{
		calls: [][]llm.StreamChunk{
			// First response: tool call
			{
				{Text: "Let me check."},
				{
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      "mock_tool",
								Arguments: json.RawMessage(`{"q":"test"}`),
							},
						},
					},
					Done: true,
				},
			},
			// Second response: text only
			{
				{Text: "Done."},
				{Done: true},
			},
		},
		callCount: &callCount,
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	registry.Register(&mockTool{})
	sb := &mockSandbox{}

	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "you are helpful")

	sessionID := "test-session-2"
	ch := bus.Subscribe(sessionID)

	errCh := make(chan error, 1)
	go func() {
		msgs := []llm.Message{
			{Role: "user", Content: json.RawMessage(`"Do something"`)},
		}
		errCh <- runner.Run(context.Background(), sessionID, msgs)
	}()

	var events []Event
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev := <-ch:
			events = append(events, ev)
			if ev.Type == "session.status_idle" {
				break loop
			}
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("runner returned error: %v", err)
	}

	// We should see: agent.message, agent.tool_use, agent.tool_result, agent.message, session.status_idle
	typeOrder := make([]string, len(events))
	for i, ev := range events {
		typeOrder[i] = ev.Type
	}

	wantTypes := map[string]bool{
		"agent.message":     false,
		"agent.tool_use":    false,
		"agent.tool_result": false,
		"session.status_idle": false,
	}
	for _, ev := range events {
		wantTypes[ev.Type] = true
	}
	for tp, found := range wantTypes {
		if !found {
			t.Errorf("missing event type %q", tp)
		}
	}
}

func TestRunnerContextCancellation(t *testing.T) {
	provider := &mockProvider{
		chunks: []llm.StreamChunk{
			{Text: "Hello"},
			{Done: true},
		},
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	sb := &mockSandbox{}
	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "system")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := runner.Run(ctx, "test-cancelled", nil)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// multiCallProvider returns different stream chunks on successive calls.
type multiCallProvider struct {
	calls     [][]llm.StreamChunk
	callCount *int
}

func (m *multiCallProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	return nil, nil
}

func (m *multiCallProvider) Stream(_ context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	idx := *m.callCount
	*m.callCount++
	chunks := m.calls[idx]

	ch := make(chan llm.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// mockTool implements tools.Tool for testing.
type mockTool struct{}

func (t *mockTool) Name() string            { return "mock_tool" }
func (t *mockTool) Description() string      { return "a mock tool" }
func (t *mockTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
}
func (t *mockTool) Execute(_ context.Context, _ json.RawMessage, _ sandbox.Sandbox) (json.RawMessage, error) {
	return json.RawMessage(`{"result":"mock output"}`), nil
}
