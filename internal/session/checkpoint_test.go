package session

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
	"github.com/domuk-k/open-managed-agents/internal/tools"
)

// echoTool is a trivial tool that returns its input as output.
type echoTool struct{}

func (e *echoTool) Name() string                { return "echo_tool" }
func (e *echoTool) Description() string          { return "echo" }
func (e *echoTool) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (e *echoTool) Execute(_ context.Context, input json.RawMessage, _ sandbox.Sandbox) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true}`), nil
}

func newTestToolRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(&echoTool{})
	return r
}

// fakeProvider is a minimal LLM provider for testing.
type fakeProvider struct {
	mu        sync.Mutex
	responses []llm.Response
	callCount int
}

func (f *fakeProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.callCount >= len(f.responses) {
		return &llm.Response{}, nil
	}
	resp := f.responses[f.callCount]
	f.callCount++
	return &resp, nil
}

func (f *fakeProvider) Stream(_ context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	f.mu.Lock()
	var resp llm.Response
	if f.callCount < len(f.responses) {
		resp = f.responses[f.callCount]
		f.callCount++
	}
	f.mu.Unlock()

	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{
		Text:      resp.Content,
		ToolCalls: resp.ToolCalls,
		Done:      true,
	}
	close(ch)
	return ch, nil
}

func TestCheckpointCalledAfterToolCycle(t *testing.T) {
	bus := NewEventBus()

	// Provider returns one tool call then a final response with no tool calls.
	provider := &fakeProvider{
		responses: []llm.Response{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "echo_tool",
							Arguments: json.RawMessage(`{}`),
						},
					},
				},
			},
			{Content: "done"},
		},
	}

	registry := newTestToolRegistry()
	runner := NewAgentRunner(provider, registry, nil, bus, "test-model", "system prompt")

	var checkpointCalls int32
	var allCheckpoints [][]llm.Message
	var cpMu sync.Mutex

	runner.WithCheckpoint(func(_ context.Context, sessionID string, messages []llm.Message) error {
		atomic.AddInt32(&checkpointCalls, 1)
		cpMu.Lock()
		cp := make([]llm.Message, len(messages))
		copy(cp, messages)
		allCheckpoints = append(allCheckpoints, cp)
		cpMu.Unlock()
		return nil
	})

	ctx := context.Background()
	err := runner.Run(ctx, "test-session", []llm.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	calls := atomic.LoadInt32(&checkpointCalls)
	// Expect 2 checkpoints: one after tool cycle, one on final (no-tool-call) response.
	if calls != 2 {
		t.Fatalf("expected checkpoint to be called 2 times, got %d", calls)
	}

	cpMu.Lock()
	defer cpMu.Unlock()

	// First checkpoint: user + assistant (tool call) + tool result = 3 messages
	first := allCheckpoints[0]
	if len(first) < 3 {
		t.Errorf("expected at least 3 messages in first checkpoint, got %d", len(first))
	}
	if first[0].Role != "user" {
		t.Errorf("first message should be user, got %s", first[0].Role)
	}

	// Second checkpoint (final): should have assistant as last message
	second := allCheckpoints[1]
	lastMsg := second[len(second)-1]
	if lastMsg.Role != "assistant" {
		t.Errorf("final checkpoint last message should be assistant, got %s", lastMsg.Role)
	}
}

func TestResumeSession(t *testing.T) {
	bus := NewEventBus()
	engine := NewSessionEngine(bus)

	// Provider returns a final response (no tool calls).
	provider := &fakeProvider{
		responses: []llm.Response{
			{Content: "resumed response"},
		},
	}

	registry := newTestToolRegistry()
	runner := NewAgentRunner(provider, registry, nil, bus, "test-model", "system prompt")

	existingMessages := []llm.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"hi"`)},
	}

	loadMessages := func(_ context.Context, _ string) ([]llm.Message, error) {
		return existingMessages, nil
	}

	ctx := context.Background()
	err := engine.ResumeSession(ctx, "resume-test", runner, loadMessages)
	if err != nil {
		t.Fatalf("ResumeSession returned error: %v", err)
	}

	// Verify the provider was actually called by waiting a bit.
	// The runner runs in a goroutine, so we need to give it time.
	done := make(chan struct{})
	go func() {
		for {
			provider.mu.Lock()
			c := provider.callCount
			provider.mu.Unlock()
			if c > 0 {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		// success
	case <-context.Background().Done():
		t.Fatal("timed out waiting for provider call")
	}
}

func TestSessionEnginePauseStop(t *testing.T) {
	bus := NewEventBus()
	engine := NewSessionEngine(bus)

	// Provider that blocks until context is cancelled (simulates a long-running session).
	blockingProvider := &blockingLLMProvider{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}

	registry := newTestToolRegistry()
	runner := NewAgentRunner(blockingProvider, registry, nil, bus, "test-model", "system")

	ctx := context.Background()
	err := engine.StartSession(ctx, "pause-test", runner, []llm.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Wait for the session to actually start streaming before stopping.
	<-blockingProvider.started

	// StopSession should succeed.
	if err := engine.StopSession("pause-test"); err != nil {
		t.Fatalf("StopSession: %v", err)
	}

	// Wait for cleanup.
	<-blockingProvider.done
}

// blockingLLMProvider blocks on Stream until context is cancelled.
type blockingLLMProvider struct {
	started chan struct{}
	done    chan struct{}
}

func (b *blockingLLMProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	return &llm.Response{}, nil
}

func (b *blockingLLMProvider) Stream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	go func() {
		close(b.started)
		<-ctx.Done()
		close(ch)
		close(b.done)
	}()
	return ch, nil
}
