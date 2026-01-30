package main

import (
	"fmt"

	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/formats"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Server is the REST API server for gokapi.
type Server struct {
	config         ServerConfig
	formatRegistry *registry.FormatRegistry
	toolRegistry   *registry.ToolRegistry
	echo           *echo.Echo
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)

	toolReg := registry.NewToolRegistry()

	return &Server{
		config:         cfg,
		formatRegistry: formatReg,
		toolRegistry:   toolReg,
	}
}

// SetupRoutes registers all API routes on the Echo instance.
func (s *Server) SetupRoutes(e *echo.Echo) {
	// Middleware
	e.Use(middleware.Logger())
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
