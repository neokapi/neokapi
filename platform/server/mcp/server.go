// Package mcp provides a cloud MCP (Model Context Protocol) server that
// exposes brand voice resources, tools, and prompts via Streamable HTTP.
package mcp

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	corebrand "github.com/neokapi/neokapi/core/brand"
)

// MCPServer wraps the MCP protocol server with brand voice resources and tools.
type MCPServer struct {
	brandStore corebrand.BrandStore
	server     *mcp.Server
	handler    http.Handler
}

// NewMCPServer creates a new MCP server with brand voice capabilities.
// It registers all resources, tools, and prompts, then creates a
// StreamableHTTP handler suitable for mounting on an HTTP server.
func NewMCPServer(brandStore corebrand.BrandStore) (*MCPServer, error) {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "bowrain",
			Version: "1.0.0",
		},
		nil,
	)

	ms := &MCPServer{
		brandStore: brandStore,
		server:     s,
	}

	// Register MCP capabilities.
	ms.registerResources()
	ms.registerPhase1Tools()
	ms.registerPhase2Tools()
	ms.registerPrompts()

	// Create Streamable HTTP handler.
	// TODO: Add OAuth 2.1 / Keycloak token validation middleware.
	ms.handler = mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return s },
		nil,
	)

	return ms, nil
}

// Handler returns the HTTP handler for mounting on Echo.
func (s *MCPServer) Handler() http.Handler {
	return s.handler
}

// RegisterRoutes mounts the MCP handler on the Echo server at /mcp/.
func (s *MCPServer) RegisterRoutes(e *echo.Echo) {
	e.Any("/mcp/*", echo.WrapHandler(http.StripPrefix("/mcp", s.handler)))
}
