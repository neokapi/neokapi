// Package mcp provides a cloud MCP (Model Context Protocol) server that
// exposes brand voice resources, tools, and prompts via Streamable HTTP.
package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	corebrand "github.com/neokapi/neokapi/core/brand"
	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/neokapi/neokapi/platform/store"
)

// MCPServer wraps the MCP protocol server with brand voice resources and tools.
type MCPServer struct {
	brandStore   corebrand.BrandStore
	contentStore store.ContentStore
	server       *mcp.Server
	handler      http.Handler
	metadata     *oauthex.ProtectedResourceMetadata
}

// Config holds configuration for the MCP server.
type Config struct {
	// JWTSecret is the secret used to validate Bowrain JWT tokens.
	// When empty, the MCP server runs without authentication.
	JWTSecret string

	// OIDCIssuerURL is the Keycloak issuer URL (e.g., "https://auth.bowrain.cloud/realms/bowrain").
	// Used in the OAuth 2.0 protected resource metadata.
	OIDCIssuerURL string

	// PublicURL is the public-facing URL of the Bowrain server
	// (e.g., "https://api.bowrain.cloud"). Used as the resource identifier
	// in OAuth metadata.
	PublicURL string
}

// NewMCPServer creates a new MCP server with brand voice capabilities.
// It registers all resources, tools, and prompts, then creates a
// StreamableHTTP handler with optional OAuth 2.1 token validation.
func NewMCPServer(brandStore corebrand.BrandStore, cfg Config) (*MCPServer, error) {
	return NewMCPServerWithStore(brandStore, nil, cfg)
}

// NewMCPServerWithStore creates a new MCP server with brand voice and
// content/flow/TM/termbase/connector tools for @bravo agent access.
func NewMCPServerWithStore(brandStore corebrand.BrandStore, contentStore store.ContentStore, cfg Config) (*MCPServer, error) {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "bowrain",
			Version: "1.0.0",
		},
		nil,
	)

	ms := &MCPServer{
		brandStore:   brandStore,
		contentStore: contentStore,
		server:       s,
	}

	// Register MCP capabilities.
	ms.registerResources()
	ms.registerPhase1Tools()
	ms.registerPhase2Tools()
	ms.registerPrompts()

	// Register expanded tools for @bravo agent (AD-028).
	if contentStore != nil {
		ms.registerContentTools()
		ms.registerFlowTools()
		ms.registerTMTools()
		ms.registerTermbaseTools()
		ms.registerConnectorTools()
	}

	// Create Streamable HTTP handler.
	streamableHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return s },
		nil,
	)

	// Build OAuth 2.0 protected resource metadata (RFC 9728).
	resourceURL := cfg.PublicURL
	if resourceURL == "" {
		resourceURL = "https://localhost:8080"
	}
	ms.metadata = &oauthex.ProtectedResourceMetadata{
		Resource:               resourceURL + "/mcp/",
		ResourceName:           "Bowrain Brand Voice MCP Server",
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{"brand:read", "brand:write"},
	}
	if cfg.OIDCIssuerURL != "" {
		ms.metadata.AuthorizationServers = []string{cfg.OIDCIssuerURL}
	}

	// Wrap with OAuth 2.1 bearer token validation when JWTSecret is configured.
	if cfg.JWTSecret != "" {
		verifier := keycloakTokenVerifier(cfg.JWTSecret)
		authMiddleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
			ResourceMetadataURL: resourceURL + "/.well-known/oauth-protected-resource",
		})
		ms.handler = authMiddleware(streamableHandler)
	} else {
		ms.handler = streamableHandler
	}

	return ms, nil
}

// keycloakTokenVerifier returns an auth.TokenVerifier that validates Bowrain
// JWT tokens (the same tokens used by the REST API).
func keycloakTokenVerifier(jwtSecret string) auth.TokenVerifier {
	return func(ctx context.Context, token string, req *http.Request) (*auth.TokenInfo, error) {
		claims, err := platauth.ValidateToken(token, jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		return &auth.TokenInfo{
			UserID:     claims.Subject,
			Expiration: claims.ExpiresAt.Time,
			Extra: map[string]any{
				"email": claims.Email,
				"name":  claims.Name,
			},
		}, nil
	}
}

// Handler returns the HTTP handler for mounting on Echo.
func (s *MCPServer) Handler() http.Handler {
	return s.handler
}

// RegisterRoutes mounts the MCP handler and OAuth metadata on the Echo server.
func (s *MCPServer) RegisterRoutes(e *echo.Echo) {
	// MCP Streamable HTTP endpoint (with optional auth).
	e.Any("/mcp/*", echo.WrapHandler(http.StripPrefix("/mcp", s.handler)))

	// OAuth 2.0 protected resource metadata (RFC 9728).
	e.GET("/.well-known/oauth-protected-resource",
		echo.WrapHandler(auth.ProtectedResourceMetadataHandler(s.metadata)))
}
