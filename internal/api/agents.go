package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/domuk-k/open-managed-agents/internal/agent"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func (s *Server) createAgent(c echo.Context) error {
	var req agent.CreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "invalid request body: "+err.Error()))
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "name is required"))
	}
	if req.Model.ID == "" {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "model.id is required"))
	}

	a := &agent.Agent{
		ID:             uuid.New().String(),
		Name:           req.Name,
		Model:          req.Model,
		System:         req.System,
		Tools:          req.Tools,
		McpServers:     req.McpServers,
		Skills:         req.Skills,
		CallableAgents: req.CallableAgents,
		Description:    req.Description,
		Metadata:       req.Metadata,
	}

	ctx := c.Request().Context()
	if err := s.store.CreateAgent(ctx, a); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	// Create version 1 snapshot
	configJSON, err := json.Marshal(a)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", "failed to marshal agent config"))
	}
	if err := s.store.CreateAgentVersion(ctx, a.ID, 1, configJSON); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", "failed to create agent version"))
	}

	return c.JSON(http.StatusCreated, a)
}

func (s *Server) listAgents(c echo.Context) error {
	agents, err := s.store.ListAgents(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, agents)
}

func (s *Server) getAgent(c echo.Context) error {
	a, err := s.store.GetAgent(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "agent not found"))
	}
	return c.JSON(http.StatusOK, a)
}

func (s *Server) updateAgent(c echo.Context) error {
	id := c.Param("id")

	var req agent.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "invalid request body: "+err.Error()))
	}

	if req.Version == 0 {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "version is required for optimistic locking"))
	}

	ctx := c.Request().Context()

	// Fetch current agent
	existing, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "agent not found"))
	}

	// Apply partial updates
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Model != nil {
		existing.Model = *req.Model
	}
	if req.System != nil {
		existing.System = req.System
	}
	if req.Tools != nil {
		existing.Tools = req.Tools
	}
	if req.McpServers != nil {
		existing.McpServers = req.McpServers
	}
	if req.Skills != nil {
		existing.Skills = req.Skills
	}
	if req.CallableAgents != nil {
		existing.CallableAgents = req.CallableAgents
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}

	if err := s.store.UpdateAgent(ctx, existing, req.Version); err != nil {
		if strings.Contains(err.Error(), "optimistic lock failed") {
			return c.JSON(http.StatusConflict, apiError("conflict", err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	// Create new version snapshot
	configJSON, err := json.Marshal(existing)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", "failed to marshal agent config"))
	}
	if err := s.store.CreateAgentVersion(ctx, existing.ID, existing.Version, configJSON); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", "failed to create agent version"))
	}

	return c.JSON(http.StatusOK, existing)
}

func (s *Server) archiveAgent(c echo.Context) error {
	id := c.Param("id")
	if err := s.store.ArchiveAgent(c.Request().Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.JSON(http.StatusNotFound, apiError("not_found", err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "archived"})
}

func (s *Server) getAgentVersions(c echo.Context) error {
	id := c.Param("id")
	versions, err := s.store.GetAgentVersions(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, versions)
}
