// Package mcp provides a cloud MCP (Model Context Protocol) server that
// exposes voice resources, tools, and prompts via Streamable HTTP.
package mcp

import (
	"context"
	"fmt"
	"net/http"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"

	"github.com/labstack/echo/v4"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/core/flow"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// MemoryResolver provides workspace-scoped content memory access.
type MemoryResolver interface {
	GetMemory(workspaceID string) (memory.Store, error)
}

// TermsResolver provides workspace-scoped terminology access.
type TermsResolver interface {
	GetTB(workspaceID string) (terms.Store, error)
}

// ConnectorResolver provides workspace-scoped connector access.
type ConnectorResolver interface {
	GetConnector(workspaceID, id string) (connector.IntegrationConnector, error)
	Fetch(ctx context.Context, workspaceID, connectorID, projectID string, opts connector.FetchOptions) ([]*connector.ContentItem, error)
	Publish(ctx context.Context, workspaceID, connectorID, projectID string, opts connector.PublishOptions) error
	ConnectorStatus(ctx context.Context, workspaceID, connectorID string) (*venueconn.SyncStatus, error)
}

// MembershipChecker validates that an authenticated principal belongs to a
// workspace. It gates the client-supplied workspace_id that workspace-scoped
// tools accept, so a caller cannot target a workspace they are not a member of.
type MembershipChecker interface {
	IsMember(ctx context.Context, workspaceID, userID string) bool
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

// MCPServer wraps the MCP protocol server with voice resources and tools.
type MCPServer struct {
	voiceStore     coreprofile.Store
	contentStore   store.ContentStore
	wsDefault      voicescope.WorkspaceDefault
	memoryResolver MemoryResolver
	tbResolver     TermsResolver
	connResolver   ConnectorResolver
	membership     MembershipChecker
	sandbox        SandboxExecutor
	toolReg        *registry.ToolRegistry
	flowCatalog    FlowCatalog
	flowRunner     FlowRunner
	tracker        EventTracker
	server         *mcp.Server
	handler        http.Handler
	metadata       *oauthex.ProtectedResourceMetadata
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

// WithMemoryResolver adds workspace-scoped content memory access.
func WithMemoryResolver(r MemoryResolver) Option {
	return func(s *MCPServer) { s.memoryResolver = r }
}

// WithTermsResolver adds workspace-scoped terms access.
func WithTermsResolver(r TermsResolver) Option {
	return func(s *MCPServer) { s.tbResolver = r }
}

// WithConnectorResolver adds connector access.
func WithConnectorResolver(r ConnectorResolver) Option {
	return func(s *MCPServer) { s.connResolver = r }
}

// WithMembershipChecker enables workspace-membership enforcement on the
// workspace-scoped tools (content memory, terms, connector). When unset (single-user /
// no-auth deployments), no enforcement is applied.
func WithMembershipChecker(m MembershipChecker) Option {
	return func(s *MCPServer) { s.membership = m }
}

// authorizeWorkspace rejects a workspace-scoped tool call whose authenticated
// principal is not a member of workspaceID, so a client-supplied workspace_id
// is validated against the caller's identity rather than trusted.
func (s *MCPServer) authorizeWorkspace(ctx context.Context, req mcp.Request, workspaceID string) error {
	return s.authorizeWorkspaceForUser(ctx, workspaceID, extractUserID(req))
}

func (s *MCPServer) authorizeWorkspaceForUser(ctx context.Context, workspaceID, userID string) error {
	if s.membership == nil {
		return nil // no-auth / single-user deployment: no enforcement
	}
	if userID == "" || workspaceID == "" || !s.membership.IsMember(ctx, workspaceID, userID) {
		return fmt.Errorf("not authorized for workspace %q", workspaceID)
	}
	return nil
}

// WithSandbox adds sandbox code execution.
func WithSandbox(e SandboxExecutor) Option {
	return func(s *MCPServer) { s.sandbox = e }
}

// WithWorkspaceDefault supplies the workspace-level default voice profile
// lookup, forming the base rung of the scoring tools' resolution ladder. When
// unset, the workspace default is skipped and resolution stops at the project.
func WithWorkspaceDefault(wd voicescope.WorkspaceDefault) Option {
	return func(s *MCPServer) { s.wsDefault = wd }
}

// WithToolRegistry adds the tool registry for flow resolution.
func WithToolRegistry(r *registry.ToolRegistry) Option {
	return func(s *MCPServer) { s.toolReg = r }
}

// FlowCatalog resolves the flows a project can run: the built-in catalog plus
// the project's stored definitions. *service.FlowCatalog satisfies it.
type FlowCatalog interface {
	List(ctx context.Context, projectID string) ([]flow.FlowDefinition, error)
	Get(ctx context.Context, projectID, flowID string) (*flow.FlowDefinition, error)
}

// FlowRunner runs a resolved flow over a project's stored content.
// *service.FlowService satisfies it.
type FlowRunner interface {
	RunFlow(ctx context.Context, run service.FlowRun) (service.FlowRunResult, error)
}

// WithFlowCatalog sets the catalog the flow tools resolve flow ids through.
func WithFlowCatalog(c FlowCatalog) Option {
	return func(s *MCPServer) { s.flowCatalog = c }
}

// WithFlowRunner sets the runner behind the run_flow tool.
func WithFlowRunner(r FlowRunner) Option {
	return func(s *MCPServer) { s.flowRunner = r }
}

// NewMCPServer creates a new MCP server with voice capabilities.
func NewMCPServer(voiceStore coreprofile.Store, cfg Config) (*MCPServer, error) {
	return NewMCPServerWithStore(voiceStore, nil, cfg)
}

// NewMCPServerWithStore creates a new MCP server with voice and
// content/flow/content memory/terms/connector tools for @bravo agent access.
func NewMCPServerWithStore(voiceStore coreprofile.Store, contentStore store.ContentStore, cfg Config, opts ...Option) (*MCPServer, error) {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "bowrain",
			Version: "1.0.0",
		},
		nil,
	)

	ms := &MCPServer{
		voiceStore:   voiceStore,
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
		ms.registerMemoryTools()
		ms.registerTermsTools()
		ms.registerConnectorTools()
	}
	if ms.sandbox != nil {
		ms.registerSandboxTools()
	}

	// Every MCP method gets a span inside the HTTP request's transaction, so a
	// slow agent call is attributable to the method rather than to "the MCP
	// endpoint". Installed unconditionally — it is a no-op without Sentry.
	s.AddReceivingMiddleware(ms.tracingMiddleware())

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
		ResourceName:           "Bowrain Voice MCP Server",
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
