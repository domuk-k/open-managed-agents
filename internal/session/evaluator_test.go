package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
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

// ---------------------------------------------------------------------------
// Tests
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
		found := false
		for _, line := range []byte(summary) {
			_ = line // iterate to check
		}
		if !contains(summary, check) {
			t.Errorf("summary missing expected content %q", check)
		}
		_ = found
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
