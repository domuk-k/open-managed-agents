package api

import (
	"context"

	"github.com/domuk-k/open-managed-agents/internal/config"
	"github.com/domuk-k/open-managed-agents/internal/session"
	"github.com/domuk-k/open-managed-agents/internal/store"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	echo     *echo.Echo
	store    store.Store
	eventBus *session.EventBus
	engine   *session.SessionEngine
	config   *config.Config
}

func NewServer(cfg *config.Config, s store.Store) *Server {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	bus := session.NewEventBus()
	srv := &Server{
		echo:     e,
		store:    s,
		eventBus: bus,
		engine:   session.NewSessionEngine(bus),
		config:   cfg,
	}

	srv.registerRoutes()
	srv.registerDashboard()
	return srv
}

func (s *Server) registerRoutes() {
	v1 := s.echo.Group("/v1")
	v1.Use(apiKeyAuth(s.config.APIKey))

	// Agents
	v1.POST("/agents", s.createAgent)
	v1.GET("/agents", s.listAgents)
	v1.GET("/agents/:id", s.getAgent)
	v1.POST("/agents/:id", s.updateAgent)
	v1.POST("/agents/:id/archive", s.archiveAgent)
	v1.GET("/agents/:id/versions", s.getAgentVersions)

	// Environments
	v1.POST("/environments", s.createEnvironment)
	v1.GET("/environments", s.listEnvironments)
	v1.GET("/environments/:id", s.getEnvironment)
	v1.POST("/environments/:id/archive", s.archiveEnvironment)

	// Sessions
	v1.POST("/sessions", s.createSession)
	v1.GET("/sessions", s.listSessions)
	v1.GET("/sessions/:id", s.getSession)
	v1.POST("/sessions/:id/events", s.postSessionEvent)
	v1.POST("/sessions/:id/pause", s.pauseSession)
	v1.POST("/sessions/:id/resume", s.resumeSession)
	v1.GET("/sessions/:id/stream", s.streamSession)
	v1.GET("/sessions/:id/events", s.getSessionEvents)
	v1.GET("/sessions/:id/evaluation", s.getSessionEvaluation)
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}
