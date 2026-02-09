package main

import (
	"fmt"
	"log"
	"os"

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
	config         ServerConfig
	formatRegistry *registry.FormatRegistry
	toolRegistry   *registry.ToolRegistry
	connectorReg   *connector.Registry
	contentStore   store.ContentStore
	services       *service.Services
	eventBus       *event.ChannelEventBus
	echo           *echo.Echo
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
		config:         cfg,
		formatRegistry: formatReg,
		toolRegistry:   toolReg,
		connectorReg:   connReg,
		eventBus:       event.NewChannelEventBus(),
	}

	// Initialize content store if a store path is configured.
	if cfg.StorePath != "" {
		cs, err := store.NewSQLiteStore(cfg.StorePath)
		if err != nil {
			log.Printf("WARNING: failed to open content store at %s: %v", cfg.StorePath, err)
		} else {
			s.contentStore = cs
			s.services = service.NewServices(cs, connReg, formatReg, toolReg)
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
	v1.GET("/health", s.handleHealth)
	v1.GET("/formats", s.handleListFormats)
	v1.GET("/tools", s.handleListTools)
	v1.GET("/flows", s.handleListFlows)
	v1.POST("/convert", s.handleConvert)
	v1.POST("/translate", s.handleTranslate)
	v1.POST("/flow/execute", s.handleFlowExecute)

	// Project endpoints
	v1.POST("/projects", s.handleCreateProject)
	v1.GET("/projects", s.handleListProjects)
	v1.GET("/projects/:id", s.handleGetProject)
	v1.PUT("/projects/:id", s.handleUpdateProject)
	v1.DELETE("/projects/:id", s.handleDeleteProject)
	v1.POST("/projects/:id/blocks", s.handleStoreBlocks)
	v1.GET("/projects/:id/blocks", s.handleGetBlocks)
	v1.POST("/projects/:id/versions", s.handleCreateVersion)
	v1.GET("/projects/:id/versions", s.handleListVersions)

	// Connector endpoints
	v1.GET("/connectors/types", s.handleListConnectorTypes)
	v1.GET("/connectors", s.handleListActiveConnectors)
	v1.POST("/connectors", s.handleAddConnector)
	v1.DELETE("/connectors/:id", s.handleRemoveConnector)
	v1.GET("/connectors/:id/status", s.handleSyncStatus)
	v1.POST("/pull", s.handlePull)
	v1.POST("/push", s.handlePush)
}

// Start initializes the Echo server and starts listening.
func (s *Server) Start(addr string) error {
	e := echo.New()
	e.HideBanner = true
	s.echo = e

	s.SetupRoutes(e)

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	}

	return e.Start(addr)
}

// Echo returns the underlying Echo instance. Useful for testing.
func (s *Server) Echo() *echo.Echo {
	if s.echo == nil {
		s.echo = echo.New()
		s.echo.HideBanner = true
		s.SetupRoutes(s.echo)
	}
	return s.echo
}
