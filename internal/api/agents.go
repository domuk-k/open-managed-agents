package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (s *Server) createAgent(c echo.Context) error {
	// TODO: implement
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (s *Server) listAgents(c echo.Context) error {
	agents, err := s.store.ListAgents(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, agents)
}

func (s *Server) getAgent(c echo.Context) error {
	a, err := s.store.GetAgent(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
	}
	return c.JSON(http.StatusOK, a)
}

func (s *Server) updateAgent(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (s *Server) archiveAgent(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (s *Server) getAgentVersions(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}
