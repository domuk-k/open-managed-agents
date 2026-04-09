package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
)

// AgentResolver looks up an agent by ID. This is a subset of the store
// interface to avoid a circular dependency on the full Store type.
type AgentResolver interface {
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
}

// SubRunnerFactory creates and runs a sub-agent runner, returning its final
// text response. This decouples the delegate tool from the session package.
type SubRunnerFactory func(ctx context.Context, ag *agent.Agent, message string) (string, error)

// DelegateTool allows an agent to delegate a task to another agent listed
// in its callable_agents field. The sub-agent runs a full LLM+tool loop and
// returns its final text response.
type DelegateTool struct {
	callerAgentID  string
	callableAgents []string
	resolver       AgentResolver
	runSub         SubRunnerFactory
}

// NewDelegateTool creates a DelegateTool configured for a specific calling agent.
func NewDelegateTool(
	callerAgentID string,
	callableAgents []string,
	resolver AgentResolver,
	runSub SubRunnerFactory,
) *DelegateTool {
	return &DelegateTool{
		callerAgentID:  callerAgentID,
		callableAgents: callableAgents,
		resolver:       resolver,
		runSub:         runSub,
	}
}

func (t *DelegateTool) Name() string { return "delegate_to_agent" }

func (t *DelegateTool) Description() string {
	return "Delegate a task to another agent and get their response"
}

func (t *DelegateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_id": {
				"type": "string",
				"description": "The ID of the agent to delegate to"
			},
			"message": {
				"type": "string",
				"description": "The task or message to send to the target agent"
			}
		},
		"required": ["agent_id", "message"]
	}`)
}

func (t *DelegateTool) Execute(ctx context.Context, input json.RawMessage, _ sandbox.Sandbox) (json.RawMessage, error) {
	var params struct {
		AgentID string `json:"agent_id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if params.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if params.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	// Check permission: the target agent must be in the caller's callable_agents.
	if !t.isCallable(params.AgentID) {
		return nil, fmt.Errorf(
			"agent %q is not authorized to delegate to agent %q",
			t.callerAgentID, params.AgentID,
		)
	}

	// Resolve the target agent.
	ag, err := t.resolver.GetAgent(ctx, params.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent %q: %w", params.AgentID, err)
	}

	// Run the sub-agent and collect the response.
	response, err := t.runSub(ctx, ag, params.Message)
	if err != nil {
		return nil, fmt.Errorf("sub-agent %q failed: %w", params.AgentID, err)
	}

	return json.Marshal(map[string]string{
		"agent_id": params.AgentID,
		"response": response,
	})
}

// isCallable checks whether the target agent ID is in the caller's allowed list.
func (t *DelegateTool) isCallable(targetID string) bool {
	for _, id := range t.callableAgents {
		if id == targetID {
			return true
		}
	}
	return false
}

// MakeSubRunnerFactory creates a SubRunnerFactory that builds a full AgentRunner
// for the target agent using the provided dependencies. The factory captures the
// LLM provider, sandbox, and event bus so each sub-run uses consistent infra.
func MakeSubRunnerFactory(
	provider llm.Provider,
	sb sandbox.Sandbox,
	eventBus interface{ Emit(string, interface{}) },
) SubRunnerFactory {
	// This is a reference implementation. In production, the session layer
	// would provide a more complete factory that wires up tool registries,
	// event forwarding, etc. For now, we use the Chat (non-streaming) path
	// to keep the sub-runner simple.
	return func(ctx context.Context, ag *agent.Agent, message string) (string, error) {
		system := ""
		if ag.System != nil {
			system = *ag.System
		}

		messages := []llm.Message{
			{
				Role:    "user",
				Content: mustMarshal(message),
			},
		}

		// Use non-streaming Chat for sub-agent calls to simplify collection.
		resp, err := provider.Chat(ctx, llm.ChatRequest{
			Model:    ag.Model.ID,
			System:   system,
			Messages: messages,
		})
		if err != nil {
			return "", err
		}

		return resp.Content, nil
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
