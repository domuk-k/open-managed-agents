package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
)

// Evaluator uses an LLM to evaluate whether a session met its defined outcomes.
type Evaluator struct {
	llm   llm.Provider
	model string
}

// EvalResult holds the evaluation result for a single outcome.
type EvalResult struct {
	Outcome string `json:"outcome"`
	Pass    bool   `json:"pass"`
	Reason  string `json:"reason"`
}

// NewEvaluator creates an Evaluator backed by the given LLM provider and model.
func NewEvaluator(provider llm.Provider, model string) *Evaluator {
	return &Evaluator{
		llm:   provider,
		model: model,
	}
}

// Evaluate checks each outcome against the session transcript and returns results.
func (e *Evaluator) Evaluate(ctx context.Context, outcomes []agent.Outcome, sessionEvents []Event) ([]EvalResult, error) {
	if len(outcomes) == 0 {
		return nil, nil
	}

	summary := buildSessionSummary(sessionEvents)

	results := make([]EvalResult, 0, len(outcomes))
	for _, oc := range outcomes {
		result, err := e.evaluateOne(ctx, oc, summary)
		if err != nil {
			return nil, fmt.Errorf("evaluate outcome %q: %w", oc.Name, err)
		}
		results = append(results, *result)
	}

	return results, nil
}

const evalSystemPrompt = `You are an evaluation judge. You will be given a session transcript and a success criterion.
Your job is to determine whether the criterion was met based on the transcript.

You MUST respond with ONLY a JSON object in this exact format (no markdown, no extra text):
{"pass": true, "reason": "brief explanation"}
or
{"pass": false, "reason": "brief explanation of why it failed"}

Do not include any text outside the JSON object.`

func (e *Evaluator) evaluateOne(ctx context.Context, outcome agent.Outcome, summary string) (*EvalResult, error) {
	userMsg := fmt.Sprintf(
		"## Session Transcript\n\n%s\n\n## Criterion\n\nOutcome: %s\nDescription: %s\nSuccess Criteria: %s\n\nDid the agent meet this criterion? Respond with JSON only.",
		summary, outcome.Name, outcome.Description, outcome.Criteria,
	)

	content, _ := json.Marshal(userMsg)
	resp, err := e.llm.Chat(ctx, llm.ChatRequest{
		Model:  e.model,
		System: evalSystemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: content},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	var parsed struct {
		Pass   bool   `json:"pass"`
		Reason string `json:"reason"`
	}

	// The response content may be a raw JSON string or plain text containing JSON.
	responseText := resp.Content

	// Try to extract JSON from the response if it's wrapped in other text.
	jsonStart := strings.Index(responseText, "{")
	jsonEnd := strings.LastIndex(responseText, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		responseText = responseText[jsonStart : jsonEnd+1]
	}

	if err := json.Unmarshal([]byte(responseText), &parsed); err != nil {
		return nil, fmt.Errorf("parse eval response %q: %w", resp.Content, err)
	}

	return &EvalResult{
		Outcome: outcome.Name,
		Pass:    parsed.Pass,
		Reason:  parsed.Reason,
	}, nil
}

// buildSessionSummary constructs a human-readable transcript from session events.
func buildSessionSummary(events []Event) string {
	var sb strings.Builder

	for _, evt := range events {
		switch evt.Type {
		case "agent.message":
			var msg map[string]string
			if json.Unmarshal(evt.Content, &msg) == nil {
				sb.WriteString(fmt.Sprintf("[Agent Message] %s\n", msg["text"]))
			}
		case "agent.tool_use":
			var tu ToolUseEvent
			if json.Unmarshal(evt.Content, &tu) == nil {
				sb.WriteString(fmt.Sprintf("[Tool Use] %s (id=%s) input=%s\n", tu.Name, tu.ID, string(tu.Input)))
			}
		case "agent.tool_result":
			var tr map[string]json.RawMessage
			if json.Unmarshal(evt.Content, &tr) == nil {
				sb.WriteString(fmt.Sprintf("[Tool Result] id=%s content=%s\n", string(tr["tool_use_id"]), string(tr["content"])))
			}
		case "user_message":
			var um UserMessageEvent
			if json.Unmarshal(evt.Content, &um) == nil {
				for _, block := range um.Content {
					sb.WriteString(fmt.Sprintf("[User] %s\n", block.Text))
				}
			}
		case "session.error":
			var e map[string]string
			if json.Unmarshal(evt.Content, &e) == nil {
				sb.WriteString(fmt.Sprintf("[Error] %s\n", e["error"]))
			}
		default:
			// Include other event types as-is for completeness.
			if evt.Content != nil {
				sb.WriteString(fmt.Sprintf("[%s] %s\n", evt.Type, string(evt.Content)))
			}
		}
	}

	if sb.Len() == 0 {
		return "(empty session - no events recorded)"
	}

	return sb.String()
}
