package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
	"github.com/domuk-k/open-managed-agents/internal/tools"
)

// AgentRunner implements the core LLM + tool execution loop.
type AgentRunner struct {
	llm     llm.Provider
	tools   *tools.Registry
	sandbox sandbox.Sandbox
	events  *EventBus
	model   string
	system  string

	// agentID identifies the agent this runner is executing for.
	// Used by the delegate tool to enforce callable_agents permissions.
	agentID        string
	callableAgents []string

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
		llm:     provider,
		tools:   registry,
		sandbox: sb,
		events:  bus,
		model:   model,
		system:  system,
		inCh:    make(chan llm.Message, 16),
	}
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
// It returns when the LLM produces a response with no tool calls,
// or when an error / context cancellation occurs.
func (r *AgentRunner) Run(ctx context.Context, sessionID string, messages []llm.Message) error {
	toolDefs := r.tools.Definitions()

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

		// No tool calls → we are done.
		if len(resp.ToolCalls) == 0 {
			r.emit(sessionID, "session.status_idle", nil)
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

// emit publishes a typed event on the bus.
func (r *AgentRunner) emit(sessionID, eventType string, content interface{}) {
	var raw json.RawMessage
	if content != nil {
		raw, _ = json.Marshal(content)
	}
	r.events.Emit(sessionID, Event{
		Type:    eventType,
		Content: raw,
	})
}

// emitError publishes a session.error event.
func (r *AgentRunner) emitError(sessionID string, err error) {
	r.emit(sessionID, "session.error", map[string]string{"error": err.Error()})
}
