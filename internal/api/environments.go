package api

import (
	"net/http"
	"strings"

	"github.com/domuk-k/open-managed-agents/internal/environment"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func (s *Server) createEnvironment(c echo.Context) error {
	var req environment.CreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "invalid request body: "+err.Error()))
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, apiError("invalid_request", "name is required"))
	}

	env := &environment.Environment{
		ID:     uuid.New().String(),
		Name:   req.Name,
		Config: req.Config,
	}

	if err := s.store.CreateEnvironment(c.Request().Context(), env); err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}

	return c.JSON(http.StatusCreated, env)
}

func (s *Server) listEnvironments(c echo.Context) error {
	envs, err := s.store.ListEnvironments(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, envs)
}

func (s *Server) getEnvironment(c echo.Context) error {
	e, err := s.store.GetEnvironment(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, apiError("not_found", "environment not found"))
	}
	return c.JSON(http.StatusOK, e)
}

func (s *Server) archiveEnvironment(c echo.Context) error {
	id := c.Param("id")
	if err := s.store.ArchiveEnvironment(c.Request().Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.JSON(http.StatusNotFound, apiError("not_found", err.Error()))
		}
		return c.JSON(http.StatusInternalServerError, apiError("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "archived"})
}
