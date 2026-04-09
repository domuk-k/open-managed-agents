package api

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed web/*
var webFS embed.FS

func (s *Server) registerDashboard() {
	// Create a sub-filesystem rooted at "web/"
	subFS, _ := fs.Sub(webFS, "web")
	fileServer := http.FileServer(http.FS(subFS))

	// Serve static files under /dashboard/
	s.echo.GET("/dashboard/*", echo.WrapHandler(
		http.StripPrefix("/dashboard/", fileServer),
	))

	// Redirect /dashboard to /dashboard/
	s.echo.GET("/dashboard", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/dashboard/")
	})

	// Root redirects to dashboard
	s.echo.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, "/dashboard/")
	})
}
