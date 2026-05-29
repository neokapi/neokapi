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

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// TMResolver provides workspace-scoped translation memory access.
type TMResolver interface {
	GetTM(workspaceID string) (sievepen.TMStore, error)
}

// TBResolver provides workspace-scoped terminology access.
type TBResolver interface {
	GetTB(workspaceID string) (termbase.TBStore, error)
}

// ConnectorResolver provides workspace-scoped connector access.
type ConnectorResolver interface {
	GetConnector(id string) (connector.IntegrationConnector, error)
	Fetch(ctx context.Context, connectorID, projectID string, opts connector.FetchOptions) ([]*connector.ContentItem, error)
	Publish(ctx context.Context, connectorID, projectID string, opts connector.PublishOptions) error
	ConnectorStatus(ctx context.Context, connectorID string) (*connector.SyncStatus, error)
}

// SandboxExecutor runs code in an isolated environment.
type SandboxExecutor interface {
	Execute(ctx context.Context, req SandboxRequest) (*SandboxResult, error)
}

// SandboxRequest describes code to execute in a sandbox.
type SandboxRequest struct {
	Language string            // "python", "bash", "node"
	Code     string            // script source
	Files    map[string][]byte // input files
	Env      map[string]string // environment variables
}

// SandboxResult holds the output of a sandbox execution.
type SandboxResult struct {
	Stdout   string // captured stdout
	Stderr   string // captured stderr
	ExitCode int
}

// MCPServer wraps the MCP protocol server with brand voice resources and tools.
type MCPServer struct {
	brandStore   corebrand.BrandStore
	contentStore store.ContentStore
	tmResolver   TMResolver
	tbResolver   TBResolver
	connResolver ConnectorResolver
	sandbox      SandboxExecutor
	toolReg      *registry.ToolRegistry
	tracker      EventTracker
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

// Option configures optional MCPServer dependencies.
type Option func(*MCPServer)

// WithTMResolver adds workspace-scoped TM access.
func WithTMResolver(r TMResolver) Option {
	return func(s *MCPServer) { s.tmResolver = r }
}

// WithTBResolver adds workspace-scoped termbase access.
func WithTBResolver(r TBResolver) Option {
	return func(s *MCPServer) { s.tbResolver = r }
}

// WithConnectorResolver adds connector access.
func WithConnectorResolver(r ConnectorResolver) Option {
	return func(s *MCPServer) { s.connResolver = r }
}

// WithSandbox adds sandbox code execution.
func WithSandbox(e SandboxExecutor) Option {
	return func(s *MCPServer) { s.sandbox = e }
}

// WithToolRegistry adds the tool registry for flow resolution.
func WithToolRegistry(r *registry.ToolRegistry) Option {
	return func(s *MCPServer) { s.toolReg = r }
}

// NewMCPServer creates a new MCP server with brand voice capabilities.
func NewMCPServer(brandStore corebrand.BrandStore, cfg Config) (*MCPServer, error) {
	return NewMCPServerWithStore(brandStore, nil, cfg)
}

// NewMCPServerWithStore creates a new MCP server with brand voice and
// content/flow/TM/termbase/connector tools for @bravo agent access.
func NewMCPServerWithStore(brandStore corebrand.BrandStore, contentStore store.ContentStore, cfg Config, opts ...Option) (*MCPServer, error) {
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

	for _, opt := range opts {
		opt(ms)
	}

	// Register MCP capabilities.
	ms.registerResources()
	ms.registerPhase1Tools()
	ms.registerPhase2Tools()
	ms.registerLoopTools() // correction-learning loop: candidates, promote, blast-radius
	ms.registerPrompts()

	// Register expanded tools for @bravo agent (Bowrain AD-016).
	if contentStore != nil {
		ms.registerContentTools()
		ms.registerFlowTools()
		ms.registerTMTools()
		ms.registerTermbaseTools()
		ms.registerConnectorTools()
	}
	if ms.sandbox != nil {
		ms.registerSandboxTools()
	}

	// Install analytics middleware when a tracker is configured.
	if ms.tracker != nil {
		s.AddReceivingMiddleware(ms.analyticsMiddleware())
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
			return nil, fmt.Errorf("%w: %w", auth.ErrInvalidToken, err)
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
