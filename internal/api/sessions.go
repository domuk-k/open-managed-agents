package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/domuk-k/open-managed-agents/internal/llm"
	"github.com/domuk-k/open-managed-agents/internal/mcp"
	"github.com/domuk-k/open-managed-agents/internal/sandbox"
	"github.com/domuk-k/open-managed-agents/internal/session"
	"github.com/domuk-k/open-managed-agents/internal/store"
	"github.com/domuk-k/open-managed-agents/internal/tools"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func (s *Server) createSession(c echo.Context) error {
	var req session.CreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "invalid request body: "+err.Error()))
	}

	if req.AgentID == "" {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "agent_id is required"))
	}
	if req.EnvironmentID == "" {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "environment_id is required"))
	}

	ctx := c.Request().Context()

	// Validate agent exists
	ag, err := s.store.GetAgent(ctx, req.AgentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", fmt.Sprintf("agent not found: %s", req.AgentID)))
	}

	// Validate environment exists
	if _, err := s.store.GetEnvironment(ctx, req.EnvironmentID); err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", fmt.Sprintf("environment not found: %s", req.EnvironmentID)))
	}

	sess := &session.Session{
		ID:            uuid.New().String(),
		Agent:         req.AgentID,
		AgentVersion:  ag.Version,
		EnvironmentID: req.EnvironmentID,
		Title:         req.Title,
		Status:        session.StatusStarting,
		Metadata:      req.Metadata,
	}

	if err := s.store.CreateSession(ctx, sess); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	// Start the AgentRunner for this session
	if err := s.startRunner(sess, ag); err != nil {
		// Session created but runner failed — still return the session
		s.store.UpdateSessionStatus(ctx, sess.ID, session.StatusFailed)
		return c.JSON(http.StatusCreated, sess)
	}
	s.store.UpdateSessionStatus(ctx, sess.ID, session.StatusRunning)
	sess.Status = session.StatusRunning

	return c.JSON(http.StatusCreated, sess)
}

func (s *Server) startRunner(sess *session.Session, ag *agent.Agent) error {
	// Create LLM provider based on config
	var provider llm.Provider
	if s.config.LLM.IsAnthropic() {
		provider = llm.NewAnthropicProvider(s.config.LLM.BaseURL, s.config.LLM.APIKey)
	} else {
		provider = llm.NewOpenAIProvider(s.config.LLM.BaseURL, s.config.LLM.APIKey)
	}

	// Create sandbox for this session
	var sb sandbox.Sandbox
	if s.sandboxProvider != nil {
		env, err := s.store.GetEnvironment(context.Background(), sess.EnvironmentID)
		if err == nil {
			created, createErr := s.sandboxProvider.Create(context.Background(), env.Config)
			if createErr != nil {
				fmt.Printf("[OMA] sandbox creation failed: %v (tools requiring sandbox will be unavailable)\n", createErr)
			} else {
				sb = created
			}
		}
	}

	// Build tool registry — with MCP tools and delegate tool if configured
	registry := tools.NewFullToolset()

	// Register MCP tools if agent has mcp_servers
	if len(ag.McpServers) > 0 {
		if err := mcp.RegisterMCPTools(context.Background(), registry, ag.McpServers); err != nil {
			fmt.Printf("[OMA] MCP tool registration failed: %v\n", err)
		}
	}

	// Register delegate tool if agent has callable_agents
	if len(ag.CallableAgents) > 0 {
		delegate := tools.NewDelegateTool(
			ag.ID,
			ag.CallableAgents,
			&storeAgentResolver{store: s.store},
			tools.MakeSubRunnerFactory(provider, sb, &eventBusAdapter{bus: s.eventBus}),
		)
		registry.Register(delegate)
	}

	// Determine model and system prompt
	model := ag.Model.ID
	systemPrompt := ""
	if ag.System != nil {
		systemPrompt = *ag.System
	}

	// Create runner with all features wired
	runner := session.NewAgentRunner(provider, registry, sb, s.eventBus, model, systemPrompt).
		WithInteractive()

	// Wire permission policy from agent tools config
	for _, tc := range ag.Tools {
		if tc.DefaultConfig != nil && tc.DefaultConfig.PermissionPolicy != nil {
			runner.WithPermissionPolicy(tc.DefaultConfig.PermissionPolicy)
			break
		}
	}

	// Wire outcomes evaluation
	if len(ag.Outcomes) > 0 {
		evaluator := session.NewEvaluator(provider, model)
		runner.WithOutcomes(ag.Outcomes, evaluator)
	}

	// Wire checkpointing
	runner.WithCheckpoint(func(ctx context.Context, sessionID string, messages []llm.Message) error {
		data, err := json.Marshal(messages)
		if err != nil {
			return err
		}
		return s.store.SaveMessages(ctx, sessionID, data)
	})

	return s.engine.StartSession(context.Background(), sess.ID, runner, []llm.Message{})
}

// storeAgentResolver adapts the Store interface for the DelegateTool's AgentResolver.
type storeAgentResolver struct {
	store store.Store
}

func (r *storeAgentResolver) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	return r.store.GetAgent(ctx, id)
}

// eventBusAdapter wraps EventBus to satisfy the interface{Emit(string, interface{})} constraint.
type eventBusAdapter struct {
	bus *session.EventBus
}

func (a *eventBusAdapter) Emit(sessionID string, content interface{}) {
	if evt, ok := content.(session.Event); ok {
		a.bus.Emit(sessionID, evt)
	}
}

func (s *Server) listSessions(c echo.Context) error {
	sessions, err := s.store.ListSessions(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, sessions)
}

func (s *Server) getSession(c echo.Context) error {
	sess, err := s.store.GetSession(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}
	return c.JSON(http.StatusOK, sess)
}

func (s *Server) postSessionEvent(c echo.Context) error {
	sessionID := c.Param("id")
	ctx := c.Request().Context()

	// Validate session exists
	if _, err := s.store.GetSession(ctx, sessionID); err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}

	var evt session.UserMessageEvent
	if err := c.Bind(&evt); err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "invalid event body: "+err.Error()))
	}

	if evt.Type == "" {
		evt.Type = "user_message"
	}

	// Serialize and store the event
	data, err := json.Marshal(evt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", "failed to marshal event"))
	}

	storedEvt := &store.StoredEvent{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Type:      evt.Type,
		Data:      data,
	}

	if err := s.store.InsertEvent(ctx, storedEvt); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	// Emit to EventBus for SSE subscribers
	busEvent := session.Event{
		Type:    evt.Type,
		Content: data,
	}
	s.eventBus.Emit(sessionID, busEvent)

	// Forward user message to the running AgentRunner
	if evt.Type == "user.message" || evt.Type == "user_message" {
		if err := s.engine.SendMessage(ctx, sessionID, evt.Content); err != nil {
			fmt.Printf("[OMA] SendMessage error for session %s: %v\n", sessionID, err)
		}
	}

	return c.JSON(http.StatusAccepted, storedEvt)
}

func (s *Server) pauseSession(c echo.Context) error {
	sessionID := c.Param("id")
	ctx := c.Request().Context()

	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}

	if sess.Status != session.StatusRunning {
		return c.JSON(http.StatusConflict, apiError("invalid_state", fmt.Sprintf("session is %s, not running", sess.Status)))
	}

	if err := s.engine.StopSession(sessionID); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	if err := s.store.UpdateSessionStatus(ctx, sessionID, session.StatusPaused); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	sess.Status = session.StatusPaused
	return c.JSON(http.StatusOK, sess)
}

func (s *Server) resumeSession(c echo.Context) error {
	sessionID := c.Param("id")
	ctx := c.Request().Context()

	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}

	if sess.Status != session.StatusPaused {
		return c.JSON(http.StatusConflict, apiError("invalid_state", fmt.Sprintf("session is %s, not paused", sess.Status)))
	}

	// Load the agent to rebuild the runner
	ag, err := s.store.GetAgent(ctx, sess.Agent)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", "agent not found for resume"))
	}

	// Restart the runner with saved messages
	if err := s.startRunnerWithResume(sess, ag); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	if err := s.store.UpdateSessionStatus(ctx, sessionID, session.StatusRunning); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	sess.Status = session.StatusRunning
	return c.JSON(http.StatusOK, sess)
}

func (s *Server) startRunnerWithResume(sess *session.Session, ag *agent.Agent) error {
	// Load saved messages
	savedData, err := s.store.GetMessages(context.Background(), sess.ID)
	if err != nil {
		// No saved messages, start fresh
		return s.startRunner(sess, ag)
	}

	var messages []llm.Message
	if err := json.Unmarshal(savedData, &messages); err != nil {
		return s.startRunner(sess, ag)
	}

	// Create the same runner setup as startRunner but with initial messages
	var provider llm.Provider
	if s.config.LLM.IsAnthropic() {
		provider = llm.NewAnthropicProvider(s.config.LLM.BaseURL, s.config.LLM.APIKey)
	} else {
		provider = llm.NewOpenAIProvider(s.config.LLM.BaseURL, s.config.LLM.APIKey)
	}

	var sb sandbox.Sandbox
	if s.sandboxProvider != nil {
		env, err := s.store.GetEnvironment(context.Background(), sess.EnvironmentID)
		if err == nil {
			if created, createErr := s.sandboxProvider.Create(context.Background(), env.Config); createErr == nil {
				sb = created
			}
		}
	}

	registry := tools.NewFullToolset()

	model := ag.Model.ID
	systemPrompt := ""
	if ag.System != nil {
		systemPrompt = *ag.System
	}

	runner := session.NewAgentRunner(provider, registry, sb, s.eventBus, model, systemPrompt).
		WithInteractive()

	return s.engine.StartSession(context.Background(), sess.ID, runner, messages)
}

func (s *Server) streamSession(c echo.Context) error {
	sessionID := c.Param("id")
	ctx := c.Request().Context()

	// Validate session exists
	if _, err := s.store.GetSession(ctx, sessionID); err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}

	// Set SSE headers
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	w.Flush()

	// Subscribe to events
	ch := s.eventBus.Subscribe(sessionID)
	defer s.eventBus.Unsubscribe(sessionID, ch)

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return nil
			}
			w.Flush()
		}
	}
}

func (s *Server) getSessionEvents(c echo.Context) error {
	sessionID := c.Param("id")
	ctx := c.Request().Context()

	// Validate session exists
	if _, err := s.store.GetSession(ctx, sessionID); err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}

	events, err := s.store.GetSessionEvents(ctx, sessionID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, events)
}

func (s *Server) getSessionEvaluation(c echo.Context) error {
	sessionID := c.Param("id")
	ctx := c.Request().Context()

	// Validate session exists
	if _, err := s.store.GetSession(ctx, sessionID); err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "session not found"))
	}

	// Get all events for this session and filter for evaluation events.
	events, err := s.store.GetSessionEvents(ctx, sessionID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	var evalEvents []*store.StoredEvent
	for _, evt := range events {
		if evt.Type == "session.evaluation" || evt.Type == "session.evaluation_complete" {
			evalEvents = append(evalEvents, evt)
		}
	}

	if len(evalEvents) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"session_id": sessionID,
			"status":     "no_evaluation",
			"results":    []interface{}{},
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"status":     "evaluated",
		"events":     evalEvents,
	})
}
