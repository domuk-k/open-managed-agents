package session

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/tools"
)

// ---------------------------------------------------------------------------
// Mock LLM Provider for evaluation tests
// ---------------------------------------------------------------------------

type evalMockProvider struct {
	response string
}

func (m *evalMockProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	return &llm.Response{Content: m.response}, nil
}

func (m *evalMockProvider) Stream(_ context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Text: m.response, Done: true}
	close(ch)
	return ch, nil
}

// sequentialEvalProvider returns different responses on successive Chat calls.
type sequentialEvalProvider struct {
	mu        sync.Mutex
	responses []string
	callIdx   int
}

func (m *sequentialEvalProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.callIdx
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1 // repeat last response if called too many times
	}
	m.callIdx++
	return &llm.Response{Content: m.responses[idx]}, nil
}

func (m *sequentialEvalProvider) Stream(_ context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	m.mu.Lock()
	idx := m.callIdx
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	resp := m.responses[idx]
	m.callIdx++
	m.mu.Unlock()

	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Text: resp, Done: true}
	close(ch)
	return ch, nil
}

func (m *sequentialEvalProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callIdx
}

// ---------------------------------------------------------------------------
// Tests for Evaluator
// ---------------------------------------------------------------------------

func TestEvaluatePassingOutcomes(t *testing.T) {
	provider := &evalMockProvider{
		response: `{"pass": true, "reason": "met criteria"}`,
	}
	evaluator := NewEvaluator(provider, "test-model")

	outcomes := []agent.Outcome{
		{
			Name:        "code_quality",
			Description: "Code should be well-structured",
			Criteria:    "The agent produced clean, well-organized code",
		},
		{
			Name:        "task_completion",
			Description: "Task should be completed",
			Criteria:    "The agent completed the requested task",
		},
	}

	events := []Event{
		{Type: "agent.message", Content: json.RawMessage(`{"type":"text","text":"I will write some code."}`)},
		{Type: "agent.tool_use", Content: json.RawMessage(`{"type":"tool_use","id":"call_1","name":"file_write","input":{"path":"main.go","content":"package main"}}`)},
		{Type: "agent.tool_result", Content: json.RawMessage(`{"tool_use_id":"call_1","content":{"success":true}}`)},
		{Type: "agent.message", Content: json.RawMessage(`{"type":"text","text":"Done writing the code."}`)},
	}

	results, err := evaluator.Evaluate(context.Background(), outcomes, events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, res := range results {
		if !res.Pass {
			t.Errorf("result[%d] expected pass=true, got false", i)
		}
		if res.Reason != "met criteria" {
			t.Errorf("result[%d] expected reason 'met criteria', got %q", i, res.Reason)
		}
		if res.Outcome != outcomes[i].Name {
			t.Errorf("result[%d] expected outcome %q, got %q", i, outcomes[i].Name, res.Outcome)
		}
	}
}

func TestEvaluateFailingOutcome(t *testing.T) {
	provider := &evalMockProvider{
		response: `{"pass": false, "reason": "did not complete the task"}`,
	}
	evaluator := NewEvaluator(provider, "test-model")

	outcomes := []agent.Outcome{
		{
			Name:        "task_completion",
			Description: "Task should be completed",
			Criteria:    "The agent completed the requested task",
		},
	}

	events := []Event{
		{Type: "agent.message", Content: json.RawMessage(`{"type":"text","text":"I encountered an error."}`)},
		{Type: "session.error", Content: json.RawMessage(`{"error":"timeout"}`)},
	}

	results, err := evaluator.Evaluate(context.Background(), outcomes, events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Pass {
		t.Error("expected pass=false, got true")
	}
	if results[0].Reason != "did not complete the task" {
		t.Errorf("expected reason 'did not complete the task', got %q", results[0].Reason)
	}
}

func TestEvaluateEmptyOutcomes(t *testing.T) {
	provider := &evalMockProvider{response: "{}"}
	evaluator := NewEvaluator(provider, "test-model")

	results, err := evaluator.Evaluate(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty outcomes, got %v", results)
	}
}

func TestBuildSessionSummary(t *testing.T) {
	events := []Event{
		{Type: "agent.message", Content: json.RawMessage(`{"type":"text","text":"Hello world"}`)},
		{Type: "agent.tool_use", Content: json.RawMessage(`{"type":"tool_use","id":"call_1","name":"bash","input":{"command":"ls"}}`)},
		{Type: "agent.tool_result", Content: json.RawMessage(`{"tool_use_id":"call_1","content":"file1.go\nfile2.go"}`)},
		{Type: "session.error", Content: json.RawMessage(`{"error":"something went wrong"}`)},
	}

	summary := buildSessionSummary(events)

	if summary == "" {
		t.Fatal("expected non-empty summary")
	}

	// Check that key elements are present in the summary.
	checks := []string{
		"Hello world",
		"bash",
		"call_1",
		"something went wrong",
	}
	for _, check := range checks {
		if !contains(summary, check) {
			t.Errorf("summary missing expected content %q", check)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBuildSessionSummaryEmpty(t *testing.T) {
	summary := buildSessionSummary(nil)
	if summary != "(empty session - no events recorded)" {
		t.Errorf("expected empty session message, got %q", summary)
	}
}

func TestEvaluateWithWrappedJSON(t *testing.T) {
	// Some LLMs wrap JSON in markdown or extra text.
	provider := &evalMockProvider{
		response: `Here is my evaluation: {"pass": true, "reason": "everything looks good"} done.`,
	}
	evaluator := NewEvaluator(provider, "test-model")

	outcomes := []agent.Outcome{
		{
			Name:        "quality",
			Description: "Quality check",
			Criteria:    "Code is good",
		},
	}

	results, err := evaluator.Evaluate(context.Background(), outcomes, []Event{
		{Type: "agent.message", Content: json.RawMessage(`{"type":"text","text":"Done."}`)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 || !results[0].Pass {
		t.Errorf("expected 1 passing result, got %+v", results)
	}
}

// ---------------------------------------------------------------------------
// Tests for Retry Logic
// ---------------------------------------------------------------------------

// retryStreamProvider supports both Chat (for evaluation) and Stream (for runner).
// Chat calls use sequential responses for evaluation.
// Stream calls use sequential responses for the main agent loop.
type retryStreamProvider struct {
	mu            sync.Mutex
	chatResponses []string
	chatIdx       int
	streamCalls   [][]llm.StreamChunk
	streamIdx     int
}

func (m *retryStreamProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.chatIdx
	if idx >= len(m.chatResponses) {
		idx = len(m.chatResponses) - 1
	}
	m.chatIdx++
	return &llm.Response{Content: m.chatResponses[idx]}, nil
}

func (m *retryStreamProvider) Stream(_ context.Context, _ llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	m.mu.Lock()
	idx := m.streamIdx
	if idx >= len(m.streamCalls) {
		idx = len(m.streamCalls) - 1
	}
	chunks := m.streamCalls[idx]
	m.streamIdx++
	m.mu.Unlock()

	ch := make(chan llm.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (m *retryStreamProvider) ChatCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chatIdx
}

func (m *retryStreamProvider) StreamCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streamIdx
}

func collectEvents(bus *EventBus, sessionID string, timeout time.Duration) []Event {
	ch := bus.Subscribe(sessionID)
	var events []Event
	timer := time.After(timeout)
	done := false
	for !done {
		select {
		case ev := <-ch:
			events = append(events, ev)
		case <-timer:
			done = true
		}
	}
	return events
}

func hasEventType(events []Event, eventType string) bool {
	for _, ev := range events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func countEventType(events []Event, eventType string) int {
	count := 0
	for _, ev := range events {
		if ev.Type == eventType {
			count++
		}
	}
	return count
}

func TestRetryOnFailureThenPass(t *testing.T) {
	// Evaluation: 1 outcome. First Chat call returns fail, second returns pass.
	// Stream: first call returns idle (no tool calls), second returns idle again after retry.
	provider := &retryStreamProvider{
		chatResponses: []string{
			`{"pass": false, "reason": "missing tests"}`,
			`{"pass": true, "reason": "tests added"}`,
		},
		streamCalls: [][]llm.StreamChunk{
			// First idle response
			{{Text: "I wrote the code.", Done: true}},
			// After retry feedback, second idle response
			{{Text: "I added the tests.", Done: true}},
		},
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	sb := &mockSandbox{}

	evaluator := NewEvaluator(provider, "test-model")
	outcomes := []agent.Outcome{
		{Name: "has_tests", Description: "Tests exist", Criteria: "Code has tests"},
	}

	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "system").
		WithOutcomes(outcomes, evaluator).
		WithMaxRetries(2)

	sessionID := "retry-pass-test"
	evtCh := bus.Subscribe(sessionID)

	errCh := make(chan error, 1)
	go func() {
		msgs := []llm.Message{{Role: "user", Content: json.RawMessage(`"Write code with tests"`)}}
		errCh <- runner.Run(context.Background(), sessionID, msgs)
	}()

	// Collect events.
	var events []Event
	timer := time.After(3 * time.Second)
	done := false
	for !done {
		select {
		case ev := <-evtCh:
			events = append(events, ev)
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runner error: %v", err)
			}
			// Drain remaining events briefly.
			drainTimer := time.After(100 * time.Millisecond)
			for {
				select {
				case ev := <-evtCh:
					events = append(events, ev)
				case <-drainTimer:
					done = true
				}
				if done {
					break
				}
			}
		case <-timer:
			t.Fatal("timed out")
		}
	}

	// Should have 2 evaluation rounds.
	evalCount := countEventType(events, "session.evaluation")
	if evalCount != 2 {
		t.Errorf("expected 2 evaluation events, got %d", evalCount)
	}

	// Should have exactly 1 retry event.
	retryCount := countEventType(events, "session.retry")
	if retryCount != 1 {
		t.Errorf("expected 1 retry event, got %d", retryCount)
	}

	// Should have retry_count event.
	if !hasEventType(events, "session.retry_count") {
		t.Error("expected session.retry_count event")
	}

	// Should NOT have evaluation_final (because it passed on retry).
	if hasEventType(events, "session.evaluation_final") {
		t.Error("unexpected session.evaluation_final event (should have passed)")
	}

	// Provider should have received 2 Stream calls and 2 Chat calls.
	if provider.StreamCallCount() != 2 {
		t.Errorf("expected 2 stream calls, got %d", provider.StreamCallCount())
	}
	if provider.ChatCallCount() != 2 {
		t.Errorf("expected 2 chat calls, got %d", provider.ChatCallCount())
	}
}

func TestMaxRetriesExhausted(t *testing.T) {
	// Evaluation always fails. maxRetries=2 means 3 total evaluations (initial + 2 retries).
	provider := &retryStreamProvider{
		chatResponses: []string{
			`{"pass": false, "reason": "still broken"}`,
		},
		streamCalls: [][]llm.StreamChunk{
			{{Text: "attempt 1", Done: true}},
			{{Text: "attempt 2", Done: true}},
			{{Text: "attempt 3", Done: true}},
		},
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	sb := &mockSandbox{}

	evaluator := NewEvaluator(provider, "test-model")
	outcomes := []agent.Outcome{
		{Name: "quality", Description: "Quality check", Criteria: "Good quality"},
	}

	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "system").
		WithOutcomes(outcomes, evaluator).
		WithMaxRetries(2)

	sessionID := "retry-exhausted-test"
	evtCh := bus.Subscribe(sessionID)

	errCh := make(chan error, 1)
	go func() {
		msgs := []llm.Message{{Role: "user", Content: json.RawMessage(`"Do something"`)}}
		errCh <- runner.Run(context.Background(), sessionID, msgs)
	}()

	var events []Event
	timer := time.After(3 * time.Second)
	done := false
	for !done {
		select {
		case ev := <-evtCh:
			events = append(events, ev)
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runner error: %v", err)
			}
			drainTimer := time.After(100 * time.Millisecond)
			for {
				select {
				case ev := <-evtCh:
					events = append(events, ev)
				case <-drainTimer:
					done = true
				}
				if done {
					break
				}
			}
		case <-timer:
			t.Fatal("timed out")
		}
	}

	// Should have 3 evaluation events (initial + 2 retries).
	evalCount := countEventType(events, "session.evaluation")
	if evalCount != 3 {
		t.Errorf("expected 3 evaluation events, got %d", evalCount)
	}

	// Should have 2 retry events.
	retryCount := countEventType(events, "session.retry")
	if retryCount != 2 {
		t.Errorf("expected 2 retry events, got %d", retryCount)
	}

	// Should have evaluation_final with retries_exhausted.
	if !hasEventType(events, "session.evaluation_final") {
		t.Error("expected session.evaluation_final event")
	}

	// 3 stream calls (1 initial + 2 retries).
	if provider.StreamCallCount() != 3 {
		t.Errorf("expected 3 stream calls, got %d", provider.StreamCallCount())
	}
}

func TestPartialFailureRetry(t *testing.T) {
	// Two outcomes: one always passes, one fails first then passes.
	// Chat responses alternate per evaluation call (2 outcomes per round).
	provider := &retryStreamProvider{
		chatResponses: []string{
			// Round 1: outcome1=pass, outcome2=fail
			`{"pass": true, "reason": "good"}`,
			`{"pass": false, "reason": "missing docs"}`,
			// Round 2: outcome1=pass, outcome2=pass
			`{"pass": true, "reason": "good"}`,
			`{"pass": true, "reason": "docs added"}`,
		},
		streamCalls: [][]llm.StreamChunk{
			{{Text: "first attempt", Done: true}},
			{{Text: "second attempt with docs", Done: true}},
		},
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	sb := &mockSandbox{}

	evaluator := NewEvaluator(provider, "test-model")
	outcomes := []agent.Outcome{
		{Name: "code_quality", Description: "Code quality", Criteria: "Good code"},
		{Name: "documentation", Description: "Docs exist", Criteria: "Has documentation"},
	}

	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "system").
		WithOutcomes(outcomes, evaluator).
		WithMaxRetries(2)

	sessionID := "partial-fail-test"
	evtCh := bus.Subscribe(sessionID)

	errCh := make(chan error, 1)
	go func() {
		msgs := []llm.Message{{Role: "user", Content: json.RawMessage(`"Write documented code"`)}}
		errCh <- runner.Run(context.Background(), sessionID, msgs)
	}()

	var events []Event
	timer := time.After(3 * time.Second)
	done := false
	for !done {
		select {
		case ev := <-evtCh:
			events = append(events, ev)
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runner error: %v", err)
			}
			drainTimer := time.After(100 * time.Millisecond)
			for {
				select {
				case ev := <-evtCh:
					events = append(events, ev)
				case <-drainTimer:
					done = true
				}
				if done {
					break
				}
			}
		case <-timer:
			t.Fatal("timed out")
		}
	}

	// Check feedback only mentions the failing outcome.
	for _, ev := range events {
		if ev.Type == "session.evaluation_feedback" {
			var fb map[string]string
			if json.Unmarshal(ev.Content, &fb) == nil {
				if contains(fb["feedback"], "code_quality") {
					t.Error("feedback should not mention passing outcome 'code_quality'")
				}
				if !contains(fb["feedback"], "documentation") {
					t.Error("feedback should mention failing outcome 'documentation'")
				}
			}
		}
	}

	// Should have exactly 1 retry (partial fail then all pass).
	retryCount := countEventType(events, "session.retry")
	if retryCount != 1 {
		t.Errorf("expected 1 retry event, got %d", retryCount)
	}

	// No evaluation_final since it passed on retry.
	if hasEventType(events, "session.evaluation_final") {
		t.Error("unexpected session.evaluation_final (should pass on retry)")
	}
}

func TestZeroRetriesNoRetry(t *testing.T) {
	// maxRetries=0: evaluation runs once, no retry even on failure.
	provider := &retryStreamProvider{
		chatResponses: []string{
			`{"pass": false, "reason": "failed"}`,
		},
		streamCalls: [][]llm.StreamChunk{
			{{Text: "done", Done: true}},
		},
	}

	bus := NewEventBus()
	registry := tools.NewRegistry()
	sb := &mockSandbox{}

	evaluator := NewEvaluator(provider, "test-model")
	outcomes := []agent.Outcome{
		{Name: "task", Description: "Task done", Criteria: "Complete"},
	}

	runner := NewAgentRunner(provider, registry, sb, bus, "test-model", "system").
		WithOutcomes(outcomes, evaluator).
		WithMaxRetries(0)

	sessionID := "zero-retry-test"
	evtCh := bus.Subscribe(sessionID)

	errCh := make(chan error, 1)
	go func() {
		msgs := []llm.Message{{Role: "user", Content: json.RawMessage(`"Do task"`)}}
		errCh <- runner.Run(context.Background(), sessionID, msgs)
	}()

	var events []Event
	timer := time.After(3 * time.Second)
	done := false
	for !done {
		select {
		case ev := <-evtCh:
			events = append(events, ev)
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runner error: %v", err)
			}
			drainTimer := time.After(100 * time.Millisecond)
			for {
				select {
				case ev := <-evtCh:
					events = append(events, ev)
				case <-drainTimer:
					done = true
				}
				if done {
					break
				}
			}
		case <-timer:
			t.Fatal("timed out")
		}
	}

	// Only 1 evaluation (no retries).
	evalCount := countEventType(events, "session.evaluation")
	if evalCount != 1 {
		t.Errorf("expected 1 evaluation event, got %d", evalCount)
	}

	// No retry events.
	retryCount := countEventType(events, "session.retry")
	if retryCount != 0 {
		t.Errorf("expected 0 retry events, got %d", retryCount)
	}

	// Should have evaluation_final since retries exhausted (0 retries, failure present).
	if !hasEventType(events, "session.evaluation_final") {
		t.Error("expected session.evaluation_final event when retries=0 and failure present")
	}

	// Only 1 stream call.
	if provider.StreamCallCount() != 1 {
		t.Errorf("expected 1 stream call, got %d", provider.StreamCallCount())
	}
}
