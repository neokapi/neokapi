// Package agenticmcp provides the Agentic Testing MCP server that exposes
// fleet management tools for the coordinator agent. It runs as a standalone
// server separate from bowrain-server.
package agenticmcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// FleetRepo provides access to the git-ops fleet state repository.
type FleetRepo interface {
	// ListWorkspaces returns metadata for all workspaces in the fleet repo.
	ListWorkspaces(ctx context.Context) ([]WorkspaceMeta, error)

	// GetWorkspacePlan reads and parses a workspace's plan.yaml.
	GetWorkspacePlan(ctx context.Context, slug string) (*WorkspacePlan, error)

	// GetWorkspaceStatus reads and parses a workspace's status.yaml.
	GetWorkspaceStatus(ctx context.Context, slug string) (*WorkspaceStatus, error)

	// CommitFile writes a file to the fleet repo and commits it.
	CommitFile(ctx context.Context, path, content, message string) (string, error)

	// ListMemoryLog returns recent git commits that touched agent memory files.
	ListMemoryLog(ctx context.Context, limit int) ([]MemoryLogEntry, error)

	// ReadAgentFile reads a file from an agent's directory in the fleet repo.
	ReadAgentFile(ctx context.Context, workspace, agent, filename string) (string, error)
}

// ExecutionFilter controls which executions to return.
type ExecutionFilter struct {
	WorkspaceSlug string
	Agent         string
	Since         string // ISO timestamp
	Limit         int
}

// Execution represents a single agent session record.
type Execution struct {
	ID          string `json:"id"`
	Workspace   string `json:"workspace"`
	Agent       string `json:"agent"`
	Role        string `json:"role"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Status      string `json:"status"` // completed | failed | running
	Task        string `json:"task"`
	Locale      string `json:"locale,omitempty"`
	Summary     string `json:"result_summary,omitempty"`
	TokensUsed  int    `json:"ai_tokens_used,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ReleaseWalker advances projects through their release history.
type ReleaseWalker interface {
	// WalkRelease advances a project to the specified (or next) release tag.
	WalkRelease(ctx context.Context, workspaceSlug, projectID, tag string) (*ReleaseResult, error)
}

// ReleaseResult describes the outcome of advancing to a release tag.
type ReleaseResult struct {
	Tag           string `json:"tag"`
	BlocksChanged int    `json:"blocks_changed"`
	BlocksAdded   int    `json:"blocks_added"`
	BlocksRemoved int    `json:"blocks_removed"`
}

// IssueTracker files feedback issues.
type IssueTracker interface {
	// FileIssue creates a GitHub issue in the agent-feedback repo.
	FileIssue(ctx context.Context, title, body string, labels []string) (string, int, error)
}

// Server wraps the MCP protocol server with agentic testing fleet tools.
type Server struct {
	fleetRepo    FleetRepo
	execStore    *PostgresExecutionStore
	eventHub     *EventHub
	walker       ReleaseWalker
	issues       IssueTracker
	bowrainURL   string
	bowrainToken string
	server       *mcp.Server
	handler      http.Handler
}

// Config holds configuration for the Agentic Testing MCP server.
type Config struct {
	JWTSecret string
	PublicURL string
}

// Option configures optional Server dependencies.
type Option func(*Server)

// WithFleetRepo adds fleet repo access for workspace discovery.
func WithFleetRepo(r FleetRepo) Option {
	return func(s *Server) { s.fleetRepo = r }
}

// WithExecutionStore adds agent execution history access.
func WithExecutionStore(es *PostgresExecutionStore) Option {
	return func(s *Server) { s.execStore = es }
}

// WithEventHub adds real-time event broadcasting for dashboard WebSocket.
func WithEventHub(hub *EventHub) Option {
	return func(s *Server) { s.eventHub = hub }
}

// WithReleaseWalker adds release walkthrough capability.
func WithReleaseWalker(w ReleaseWalker) Option {
	return func(s *Server) { s.walker = w }
}

// WithIssueTracker adds GitHub issue filing capability.
func WithIssueTracker(t IssueTracker) Option {
	return func(s *Server) { s.issues = t }
}

// WithBowrain configures direct Bowrain API access for task tools.
func WithBowrain(url, token string) Option {
	return func(s *Server) { s.bowrainURL = url; s.bowrainToken = token }
}

// NewServer creates a new Agentic Testing MCP server.
func NewServer(cfg Config, opts ...Option) (*Server, error) {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "bowrain-agentic",
			Version: "1.0.0",
		},
		nil,
	)

	as := &Server{server: s}
	for _, opt := range opts {
		opt(as)
	}

	// Register fleet management and task tools.
	as.registerFleetTools()
	as.registerExecutionTools()
	as.registerReleaseTools()
	as.registerFeedbackTools()
	as.registerTaskTools()

	// Create Streamable HTTP handler.
	streamableHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return s },
		nil,
	)

	// Wrap with bearer token validation when JWTSecret is configured.
	if cfg.JWTSecret != "" {
		verifier := keycloakTokenVerifier(cfg.JWTSecret)
		authMiddleware := auth.RequireBearerToken(verifier, nil)
		as.handler = authMiddleware(streamableHandler)
	} else {
		as.handler = streamableHandler
	}

	return as, nil
}

// keycloakTokenVerifier validates Bowrain JWT tokens.
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

// EventHub returns the event hub for dashboard WebSocket relay, or nil.
func (s *Server) EventHub() *EventHub { return s.eventHub }

// ExecStore returns the execution store, or nil.
func (s *Server) ExecStore() *PostgresExecutionStore { return s.execStore }

// FleetRepo returns the fleet repo, or nil.
func (s *Server) FleetRepo() FleetRepo { return s.fleetRepo }

// IssueTracker returns the GitHub issue tracker, or nil.
func (s *Server) IssueTracker() *GitHubIssueTracker {
	if t, ok := s.issues.(*GitHubIssueTracker); ok {
		return t
	}
	return nil
}

// Handler returns the HTTP handler for mounting the MCP server.
func (s *Server) Handler() http.Handler { return s.handler }
