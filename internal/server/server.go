package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/auth"
	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/event"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/service"
	"github.com/gokapi/gokapi/core/store"
	"github.com/gokapi/gokapi/formats"
	libtools "github.com/gokapi/gokapi/lib/tools"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Server is the REST API server for gokapi.
type Server struct {
	Config         ServerConfig
	FormatRegistry *registry.FormatRegistry
	ToolRegistry   *registry.ToolRegistry
	ConnectorReg   *connector.Registry
	ContentStore   store.ContentStore
	Services       *service.Services
	AuthStore      auth.AuthStore
	EventBus       *event.ChannelEventBus
	Echo           *echo.Echo
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	connReg := connector.NewRegistry()
	connector.RegisterAll(connReg, formatReg)

	s := &Server{
		Config:         cfg,
		FormatRegistry: formatReg,
		ToolRegistry:   toolReg,
		ConnectorReg:   connReg,
		EventBus:       event.NewChannelEventBus(),
	}

	// Initialize content store if a store path is configured.
	if cfg.StorePath != "" {
		cs, err := store.NewSQLiteStore(cfg.StorePath)
		if err != nil {
			log.Printf("WARNING: failed to open content store at %s: %v", cfg.StorePath, err)
		} else {
			s.ContentStore = cs
			s.Services = service.NewServices(cs, connReg, formatReg, toolReg)
		}
	}

	return s
}

// SetupRoutes registers all API routes on the Echo instance.
func (s *Server) SetupRoutes(e *echo.Echo) {
	// Middleware
	logger := log.New(os.Stdout, "", log.LstdFlags)
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod: true,
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logger.Printf("%s %s %d\n", v.Method, v.URI, v.Status)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API v1 routes
	v1 := e.Group("/api/v1")
	v1.GET("/health", s.HandleHealth)
	v1.GET("/config", s.HandleConfig)
	v1.GET("/formats", s.HandleListFormats)
	v1.GET("/tools", s.HandleListTools)

	// Project endpoints (backward-compatible flat routes)
	v1.POST("/projects", s.HandleCreateProject)
	v1.GET("/projects", s.HandleListProjects)
	v1.GET("/projects/:id", s.HandleGetProject)
	v1.PUT("/projects/:id", s.HandleUpdateProject)
	v1.DELETE("/projects/:id", s.HandleDeleteProject)
	v1.POST("/projects/:id/blocks", s.HandleStoreBlocks)
	v1.GET("/projects/:id/blocks", s.HandleGetBlocks)
	v1.POST("/projects/:id/versions", s.HandleCreateVersion)
	v1.GET("/projects/:id/versions", s.HandleListVersions)

	// Connector endpoints
	v1.GET("/connectors/types", s.HandleListConnectorTypes)
	v1.GET("/connectors", s.HandleListActiveConnectors)
	v1.POST("/connectors", s.HandleAddConnector)
	v1.DELETE("/connectors/:id", s.HandleRemoveConnector)
	v1.GET("/connectors/:id/status", s.HandleSyncStatus)
	v1.POST("/pull", s.HandlePull)
	v1.POST("/push", s.HandlePush)

	// Auth endpoints (only for multi-user mode)
	if !s.Config.LocalMode && s.Config.JWTSecret != "" {
		authGroup := v1.Group("/auth")
		authGroup.POST("/device/start", s.HandleDeviceAuthStart)
		authGroup.POST("/device/poll", s.HandleDeviceAuthPoll)
		authGroup.GET("/callback", s.HandleAuthCallback)
		authGroup.GET("/me", s.HandleAuthMe)
		authGroup.POST("/logout", s.HandleAuthLogout)

		// Workspace endpoints
		wsGroup := v1.Group("/workspaces")
		wsGroup.POST("", s.HandleCreateWorkspace)
		wsGroup.GET("", s.HandleListWorkspaces)
		wsGroup.GET("/:ws", s.HandleGetWorkspace)
		wsGroup.PUT("/:ws", s.HandleUpdateWorkspace)
		wsGroup.DELETE("/:ws", s.HandleDeleteWorkspace)
		wsGroup.GET("/:ws/members", s.HandleListMembers)
		wsGroup.POST("/:ws/members", s.HandleAddMember)
		wsGroup.PUT("/:ws/members/:uid/role", s.HandleUpdateMemberRole)
		wsGroup.DELETE("/:ws/members/:uid", s.HandleRemoveMember)

		// Workspace-scoped project routes
		wsGroup.GET("/:ws/projects", s.HandleListWorkspaceProjects)
		wsGroup.POST("/:ws/projects", s.HandleCreateWorkspaceProject)
	}

	// Web UI static file serving
	if s.Config.WebUIDir != "" {
		e.Static("/", s.Config.WebUIDir)
		// SPA fallback: serve index.html for non-API routes
		e.GET("/*", func(c echo.Context) error {
			return c.File(filepath.Join(s.Config.WebUIDir, "index.html"))
		})
	}
}

// Start initializes the Echo server and starts listening.
func (s *Server) Start(addr string) error {
	e := echo.New()
	e.HideBanner = true
	s.Echo = e

	s.SetupRoutes(e)

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)
	}

	return e.Start(addr)
}

// GetEcho returns the underlying Echo instance. Useful for testing.
func (s *Server) GetEcho() *echo.Echo {
	if s.Echo == nil {
		s.Echo = echo.New()
		s.Echo.HideBanner = true
		s.SetupRoutes(s.Echo)
	}
	return s.Echo
}
