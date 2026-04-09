package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (s *Server) createEnvironment(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func (s *Server) listEnvironments(c echo.Context) error {
	envs, err := s.store.ListEnvironments(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, envs)
}

func (s *Server) getEnvironment(c echo.Context) error {
	e, err := s.store.GetEnvironment(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "environment not found"})
	}
	return c.JSON(http.StatusOK, e)
}

func (s *Server) archiveEnvironment(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}
