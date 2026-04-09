package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
	"github.com/domuk-k/open-managed-agents/internal/tools"
)

// CheckpointFunc is called after each LLM+tool cycle with the current messages.
type CheckpointFunc func(ctx context.Context, sessionID string, messages []llm.Message) error

// AgentRunner implements the core LLM + tool execution loop.
type AgentRunner struct {
	llm     llm.Provider
	tools   *tools.Registry
	sandbox sandbox.Sandbox
	events  *EventBus
	model   string
	system  string

	// permChecker enforces scoped permission policies on tool calls.
	permChecker *tools.PermissionChecker

	// agentID identifies the agent this runner is executing for.
	// Used by the delegate tool to enforce callable_agents permissions.
	agentID        string
	callableAgents []string

	// checkpoint is called after each LLM+tool execution cycle to persist messages.
	checkpoint CheckpointFunc

	// outcomes holds the success criteria to evaluate after the session completes.
	outcomes []agent.Outcome

	// evaluator runs outcome evaluation after the main loop finishes.
	evaluator *Evaluator

	// collectedEvents stores events emitted during the session for evaluation.
	collectedEvents []Event

	// maxRetries is the maximum number of retry attempts when evaluation fails.
	maxRetries int

	// inCh receives user messages while the runner is active.
	inCh chan llm.Message
}

// NewAgentRunner creates an AgentRunner wired to the given provider, tool
// registry, sandbox, event bus, model name and system prompt.
func NewAgentRunner(
	provider llm.Provider,
	registry *tools.Registry,
	sb sandbox.Sandbox,
	bus *EventBus,
	model, system string,
) *AgentRunner {
	return &AgentRunner{
		llm:         provider,
		tools:       registry,
		sandbox:     sb,
		events:      bus,
		model:       model,
		system:      system,
		permChecker: tools.NewPermissionChecker(nil), // default: allow all
		maxRetries:  2,
		inCh:        make(chan llm.Message, 16),
	}
}

// WithCheckpoint sets the checkpoint callback for persisting messages after each cycle.
func (r *AgentRunner) WithCheckpoint(fn CheckpointFunc) *AgentRunner {
	r.checkpoint = fn
	return r
}

// WithPermissionPolicy configures the runner to enforce the given permission policy
// on all tool calls. If policy is nil, all tools are allowed.
func (r *AgentRunner) WithPermissionPolicy(policy *agent.PermissionPolicy) *AgentRunner {
	r.permChecker = tools.NewPermissionChecker(policy)
	return r
}

// WithOutcomes configures the runner with outcome criteria and an evaluator
// for post-session self-evaluation.
func (r *AgentRunner) WithOutcomes(outcomes []agent.Outcome, evaluator *Evaluator) *AgentRunner {
	r.outcomes = outcomes
	r.evaluator = evaluator
	return r
}

// WithMaxRetries sets the maximum number of evaluation retry attempts.
// When set to 0, evaluation runs once with no retries.
func (r *AgentRunner) WithMaxRetries(n int) *AgentRunner {
	r.maxRetries = n
	return r
}

// WithAgentContext sets the agent ID and callable agents on the runner,
// enabling the delegate_to_agent tool to enforce permissions.
func (r *AgentRunner) WithAgentContext(agentID string, callableAgents []string) *AgentRunner {
	r.agentID = agentID
	r.callableAgents = callableAgents
	return r
}

// AgentID returns the agent ID associated with this runner.
func (r *AgentRunner) AgentID() string { return r.agentID }

// CallableAgents returns the list of agent IDs this runner is allowed to delegate to.
func (r *AgentRunner) CallableAgents() []string { return r.callableAgents }

// InCh returns the channel used to inject user messages into a running session.
func (r *AgentRunner) InCh() chan llm.Message {
	return r.inCh
}

// Run executes the agentic loop: call LLM, execute tools, repeat.
// It returns when the LLM produces a response with no tool calls
// and all outcomes pass (or retries are exhausted),
// or when an error / context cancellation occurs.
func (r *AgentRunner) Run(ctx context.Context, sessionID string, messages []llm.Message) error {
	toolDefs := r.tools.Definitions()
	retryAttempt := 0

	for {
		// Check context before each LLM call.
		if err := ctx.Err(); err != nil {
			r.emitError(sessionID, err)
			return err
		}

		stream, err := r.llm.Stream(ctx, llm.ChatRequest{
			Model:    r.model,
			System:   r.system,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			r.emitError(sessionID, err)
			return fmt.Errorf("llm stream: %w", err)
		}

		// Consume the stream, accumulating a full Response.
		resp := &llm.Response{}
		for chunk := range stream {
			if ctx.Err() != nil {
				r.emitError(sessionID, ctx.Err())
				return ctx.Err()
			}

			// Emit text fragments as agent.message events.
			if chunk.Text != "" {
				r.emit(sessionID, "agent.message", map[string]string{
					"type": "text",
					"text": chunk.Text,
				})
			}

			resp.Accumulate(chunk)

			// On the final chunk, capture accumulated tool calls.
			if chunk.Done && len(chunk.ToolCalls) > 0 {
				resp.ToolCalls = chunk.ToolCalls
			}
		}

		// No tool calls -> agent is idle; evaluate outcomes if configured.
		if len(resp.ToolCalls) == 0 {
			// Checkpoint: persist the complete conversation.
			if r.checkpoint != nil {
				finalMessages := append(messages, resp.ToAssistantMessage())
				_ = r.checkpoint(ctx, sessionID, finalMessages)
			}
			messages = append(messages, resp.ToAssistantMessage())
			r.emit(sessionID, "session.status_idle", nil)

			// Run outcome evaluation if configured.
			if len(r.outcomes) > 0 && r.evaluator != nil {
				feedback, evalErr := r.runEvaluationWithRetry(ctx, sessionID)
				if evalErr != nil {
					r.emit(sessionID, "session.warning", map[string]string{
						"warning": "evaluation failed: " + evalErr.Error(),
					})
					return nil
				}

				// If there are failures and we have retries left, inject feedback and continue.
				if feedback != "" && retryAttempt < r.maxRetries {
					retryAttempt++
					r.emit(sessionID, "session.retry_count", map[string]int{
						"attempt": retryAttempt,
						"max":     r.maxRetries,
					})
					r.emit(sessionID, "session.retry", map[string]string{
						"content": feedback,
					})

					// Append feedback as a user message to the conversation.
					feedbackContent, _ := json.Marshal(feedback)
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: feedbackContent,
					})
					continue // re-enter the main loop for another attempt
				}

				// All outcomes passed or retries exhausted.
				if feedback != "" {
					r.emit(sessionID, "session.evaluation_final", map[string]string{
						"status": "retries_exhausted",
					})
				}
			}

			return nil
		}

		// Append the assistant message to conversation history.
		messages = append(messages, resp.ToAssistantMessage())

		// Execute tool calls in parallel.
		type toolResultEntry struct {
			idx    int
			result llm.ToolResult
		}

		results := make([]toolResultEntry, len(resp.ToolCalls))
		var wg sync.WaitGroup

		for i, tc := range resp.ToolCalls {
			// Emit tool_use event.
			r.emit(sessionID, "agent.tool_use", ToolUseEvent{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: tc.Function.Arguments,
			})

			wg.Add(1)
			go func(idx int, tc llm.ToolCall) {
				defer wg.Done()

				// Enforce permission policy before executing the tool.
				if permErr := r.permChecker.Check(tc.Function.Name, tc.Function.Arguments); permErr != nil {
					content, _ := json.Marshal(map[string]string{"error": permErr.Error()})
					results[idx] = toolResultEntry{
						idx: idx,
						result: llm.ToolResult{
							ID:      tc.ID,
							Content: content,
						},
					}
					r.emit(sessionID, "agent.tool_result", map[string]interface{}{
						"tool_use_id": tc.ID,
						"content":     json.RawMessage(content),
					})
					return
				}

				out, execErr := r.tools.Execute(ctx, tc.Function.Name, tc.Function.Arguments, r.sandbox)
				var content json.RawMessage
				if execErr != nil {
					content, _ = json.Marshal(map[string]string{"error": execErr.Error()})
				} else {
					content = out
				}

				results[idx] = toolResultEntry{
					idx: idx,
					result: llm.ToolResult{
						ID:      tc.ID,
						Content: content,
					},
				}

				// Emit tool_result event.
				r.emit(sessionID, "agent.tool_result", map[string]interface{}{
					"tool_use_id": tc.ID,
					"content":     json.RawMessage(content),
				})
			}(i, tc)
		}

		wg.Wait()

		// Append tool result messages in order.
		for _, entry := range results {
			messages = append(messages, toolResultToMessage(entry.result))
		}

		// Checkpoint: persist messages after each LLM+tool cycle.
		if r.checkpoint != nil {
			if cpErr := r.checkpoint(ctx, sessionID, messages); cpErr != nil {
				r.emit(sessionID, "session.warning", map[string]string{
					"warning": "checkpoint failed: " + cpErr.Error(),
				})
			}
		}
	}
}

// toolResultToMessage converts a ToolResult into a tool-role Message.
func toolResultToMessage(tr llm.ToolResult) llm.Message {
	return llm.Message{
		Role:       "tool",
		Content:    tr.Content,
		ToolCallID: tr.ID,
	}
}

// emit publishes a typed event on the bus and collects it for evaluation.
func (r *AgentRunner) emit(sessionID, eventType string, content interface{}) {
	var raw json.RawMessage
	if content != nil {
		raw, _ = json.Marshal(content)
	}
	evt := Event{
		Type:    eventType,
		Content: raw,
	}
	r.collectedEvents = append(r.collectedEvents, evt)
	r.events.Emit(sessionID, evt)
}

// emitError publishes a session.error event.
func (r *AgentRunner) emitError(sessionID string, err error) {
	r.emit(sessionID, "session.error", map[string]string{"error": err.Error()})
}

// runEvaluationWithRetry executes outcome evaluation and emits results as events.
// It returns a non-empty feedback string when any outcome fails, or "" when all pass.
func (r *AgentRunner) runEvaluationWithRetry(ctx context.Context, sessionID string) (string, error) {
	results, err := r.evaluator.Evaluate(ctx, r.outcomes, r.collectedEvents)
	if err != nil {
		return "", err
	}

	r.emit(sessionID, "session.evaluation", results)

	// Check for failures and build feedback.
	var failures []EvalResult
	for _, res := range results {
		if !res.Pass {
			failures = append(failures, res)
		}
	}

	r.emit(sessionID, "session.evaluation_complete", map[string]interface{}{
		"total":  len(results),
		"passed": len(results) - len(failures),
		"failed": len(failures),
	})

	if len(failures) == 0 {
		return "", nil
	}

	// Build feedback message listing only the failed outcomes.
	var sb strings.Builder
	sb.WriteString("The following outcomes were not met:\n")
	for _, f := range failures {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", f.Outcome, f.Reason))
	}
	sb.WriteString("Please address these issues.")

	feedback := sb.String()

	r.emit(sessionID, "session.evaluation_feedback", map[string]string{
		"feedback": feedback,
	})

	return feedback, nil
}
