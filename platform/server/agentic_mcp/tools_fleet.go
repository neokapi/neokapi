package agentic_mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerFleetTools registers fleet overview and workspace tools.
func (s *Server) registerFleetTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_fleet_summary",
		Description: "Get aggregated status across all testing workspaces: untranslated block counts, last agent sessions, active sessions, daily budget usage. Primary decision-making input for coordination cycles.",
	}, s.handleGetFleetSummary)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_workspace_status",
		Description: "Get detailed status for a single workspace including per-project breakdowns, per-locale translation stats, recent activity log, and agent session history.",
	}, s.handleGetWorkspaceStatus)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_workspaces",
		Description: "List all testing workspaces registered in the fleet repo with basic metadata: slug, phase, project name, upstream repo, target languages, operating mode.",
	}, s.handleListWorkspaces)
}

// ── get_fleet_summary ────────────────────────────────────────────────────

type getFleetSummaryInput struct{}

type fleetSummaryOutput struct {
	Workspaces []workspaceSummary `json:"workspaces"`
	Global     globalStats        `json:"global"`
}

type workspaceSummary struct {
	Slug               string            `json:"slug"`
	ProjectCount       int               `json:"project_count"`
	PendingPushes      int               `json:"pending_pushes"`
	UntranslatedBlocks map[string]int    `json:"untranslated_blocks"`
	LastAgentSession   *sessionSummary   `json:"last_agent_session,omitempty"`
	ActiveSessions     []sessionSummary  `json:"active_sessions"`
	Health             string            `json:"health"`
	Mode               string            `json:"mode"`
}

type sessionSummary struct {
	Agent            string `json:"agent"`
	Role             string `json:"role"`
	StartedAt        string `json:"started_at"`
	Status           string `json:"status"`
	BlocksTranslated int    `json:"blocks_translated,omitempty"`
	BlocksPushed     int    `json:"blocks_pushed,omitempty"`
}

type globalStats struct {
	TotalWorkspaces  int     `json:"total_workspaces"`
	ActiveSessions   int     `json:"active_sessions"`
	SessionsToday    int     `json:"sessions_today"`
	DailyBudget      int     `json:"daily_budget"`
	AISpendTodayUSD  float64 `json:"ai_spend_today_usd"`
}

func (s *Server) handleGetFleetSummary(ctx context.Context, req *mcp.CallToolRequest, input getFleetSummaryInput) (*mcp.CallToolResult, fleetSummaryOutput, error) {
	if s.fleetRepo == nil {
		return nil, fleetSummaryOutput{}, fmt.Errorf("fleet repo not configured")
	}

	workspaces, err := s.fleetRepo.ListWorkspaces(ctx)
	if err != nil {
		return nil, fleetSummaryOutput{}, fmt.Errorf("list workspaces: %w", err)
	}

	var summaries []workspaceSummary
	var totalActive int

	for _, ws := range workspaces {
		summary := workspaceSummary{
			Slug:               ws.Slug,
			ProjectCount:       ws.ProjectCount,
			UntranslatedBlocks: ws.UntranslatedBlocks,
			Health:             ws.Health,
			Mode:               ws.Mode,
		}

		// Enrich with execution history if available.
		if s.execStore != nil {
			execs, _ := s.execStore.ListExecutions(ctx, ExecutionFilter{
				WorkspaceSlug: ws.Slug,
				Limit:         1,
			})
			if len(execs) > 0 {
				last := execs[0]
				summary.LastAgentSession = &sessionSummary{
					Agent:     last.Agent,
					Role:      last.Role,
					StartedAt: last.StartedAt,
					Status:    last.Status,
				}
			}

			// Count active sessions.
			activeExecs, _ := s.execStore.ListExecutions(ctx, ExecutionFilter{
				WorkspaceSlug: ws.Slug,
			})
			for _, e := range activeExecs {
				if e.Status == "running" {
					totalActive++
					summary.ActiveSessions = append(summary.ActiveSessions, sessionSummary{
						Agent:     e.Agent,
						Role:      e.Role,
						StartedAt: e.StartedAt,
						Status:    e.Status,
					})
				}
			}
		}

		summaries = append(summaries, summary)
	}

	return nil, fleetSummaryOutput{
		Workspaces: summaries,
		Global: globalStats{
			TotalWorkspaces: len(workspaces),
			ActiveSessions:  totalActive,
		},
	}, nil
}

// ── get_workspace_status ─────────────────────────────────────────────────

type getWorkspaceStatusInput struct {
	WorkspaceSlug string `json:"workspace_slug" jsonschema:"the workspace slug to get status for"`
}

type workspaceStatusOutput struct {
	Slug           string           `json:"slug"`
	Projects       []projectStatus  `json:"projects"`
	RecentActivity []activityEntry  `json:"recent_activity"`
	AgentHistory   []agentHistory   `json:"agent_history"`
}

type projectStatus struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	SourceLanguage  string            `json:"source_language"`
	TargetLanguages []string          `json:"target_languages"`
	BlockStats      blockStats        `json:"block_stats"`
	LastPush        string            `json:"last_push,omitempty"`
}

type blockStats struct {
	Total      int            `json:"total"`
	Translated map[string]int `json:"translated"`
	Reviewed   map[string]int `json:"reviewed"`
}

type activityEntry struct {
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Project   string `json:"project,omitempty"`
}

type agentHistory struct {
	Agent              string  `json:"agent"`
	LastSession        string  `json:"last_session"`
	SessionsThisWeek   int     `json:"sessions_this_week"`
	AvgDurationMinutes float64 `json:"avg_duration_minutes"`
}

func (s *Server) handleGetWorkspaceStatus(ctx context.Context, req *mcp.CallToolRequest, input getWorkspaceStatusInput) (*mcp.CallToolResult, workspaceStatusOutput, error) {
	if s.fleetRepo == nil {
		return nil, workspaceStatusOutput{}, fmt.Errorf("fleet repo not configured")
	}

	plan, err := s.fleetRepo.GetWorkspacePlan(ctx, input.WorkspaceSlug)
	if err != nil {
		return nil, workspaceStatusOutput{}, fmt.Errorf("get workspace plan: %w", err)
	}

	output := workspaceStatusOutput{
		Slug: input.WorkspaceSlug,
		Projects: []projectStatus{{
			Name:            plan.ProjectName,
			SourceLanguage:  plan.SourceLanguage,
			TargetLanguages: plan.TargetLanguages,
		}},
	}

	// Enrich with execution history.
	if s.execStore != nil {
		execs, _ := s.execStore.ListExecutions(ctx, ExecutionFilter{
			WorkspaceSlug: input.WorkspaceSlug,
			Limit:         20,
		})
		for _, e := range execs {
			output.RecentActivity = append(output.RecentActivity, activityEntry{
				Timestamp: e.StartedAt,
				Actor:     "agent-" + e.Agent,
				Action:    e.Summary,
			})
		}
	}

	return nil, output, nil
}

// ── list_workspaces ──────────────────────────────────────────────────────

type listWorkspacesInput struct{}

type listWorkspacesOutput struct {
	Workspaces []workspaceMeta `json:"workspaces"`
}

type workspaceMeta struct {
	Slug            string   `json:"slug"`
	Phase           string   `json:"phase"`
	ProjectName     string   `json:"project_name"`
	UpstreamRepo    string   `json:"upstream_repo"`
	TargetLanguages []string `json:"target_languages"`
	Mode            string   `json:"mode"`
}

func (s *Server) handleListWorkspaces(ctx context.Context, req *mcp.CallToolRequest, input listWorkspacesInput) (*mcp.CallToolResult, listWorkspacesOutput, error) {
	if s.fleetRepo == nil {
		return nil, listWorkspacesOutput{}, fmt.Errorf("fleet repo not configured")
	}

	workspaces, err := s.fleetRepo.ListWorkspaces(ctx)
	if err != nil {
		return nil, listWorkspacesOutput{}, fmt.Errorf("list workspaces: %w", err)
	}

	var out []workspaceMeta
	for _, ws := range workspaces {
		out = append(out, workspaceMeta{
			Slug:            ws.Slug,
			Phase:           ws.Phase,
			ProjectName:     ws.ProjectName,
			UpstreamRepo:    ws.UpstreamRepo,
			TargetLanguages: ws.TargetLanguages,
			Mode:            ws.Mode,
		})
	}

	return nil, listWorkspacesOutput{Workspaces: out}, nil
}
