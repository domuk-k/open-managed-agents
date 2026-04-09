package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/domuk-k/open-managed-agents/internal/session"
	"github.com/domuk-k/open-managed-agents/internal/store"
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

	return c.JSON(http.StatusCreated, sess)
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

	if err := s.store.UpdateSessionStatus(ctx, sessionID, session.StatusRunning); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	sess.Status = session.StatusRunning
	return c.JSON(http.StatusOK, sess)
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
