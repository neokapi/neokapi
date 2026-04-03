package agenticmcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerReleaseTools registers release walkthrough and project onboarding tools.
func (s *Server) registerReleaseTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "walk_release",
		Description: "Advance a workspace to the next release tag. Only workspace_slug is required — project_id and tag are auto-resolved from plan.yaml if omitted. Returns the tag applied and block counts.",
	}, s.handleWalkRelease)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "onboard_project",
		Description: "Set up a new project for agentic testing: creates the workspace directory in the fleet repo, forks the upstream repository, configures languages, and generates a workspace plan.",
	}, s.handleOnboardProject)
}

// ── walk_release ─────────────────────────────────────────────────────────

type walkReleaseInput struct {
	WorkspaceSlug string `json:"workspace_slug" jsonschema:"target workspace slug"`
	ProjectID     string `json:"project_id,omitempty" jsonschema:"project ID (optional — auto-resolved from plan.yaml if omitted)"`
	Tag           string `json:"tag,omitempty" jsonschema:"specific tag to advance to; omit for next unprocessed"`
}

type walkReleaseOutput struct {
	Tag           string `json:"tag"`
	BlocksChanged int    `json:"blocks_changed"`
	BlocksAdded   int    `json:"blocks_added"`
	BlocksRemoved int    `json:"blocks_removed"`
}

func (s *Server) handleWalkRelease(ctx context.Context, req *mcp.CallToolRequest, input walkReleaseInput) (*mcp.CallToolResult, walkReleaseOutput, error) {
	if s.walker == nil {
		return nil, walkReleaseOutput{}, fmt.Errorf("release walker not configured")
	}

	result, err := s.walker.WalkRelease(ctx, input.WorkspaceSlug, input.ProjectID, input.Tag)
	if err != nil {
		return nil, walkReleaseOutput{}, fmt.Errorf("walk release: %w", err)
	}

	return nil, walkReleaseOutput{
		Tag:           result.Tag,
		BlocksChanged: result.BlocksChanged,
		BlocksAdded:   result.BlocksAdded,
		BlocksRemoved: result.BlocksRemoved,
	}, nil
}

// ── onboard_project ──────────────────────────────────────────────────────

type onboardProjectInput struct {
	UpstreamRepo    string        `json:"upstream_repo" jsonschema:"GitHub repo in owner/name format"`
	Name            string        `json:"name" jsonschema:"display name for the project"`
	SourceLanguage  string        `json:"source_language" jsonschema:"BCP-47 source language tag"`
	TargetLanguages []string      `json:"target_languages" jsonschema:"BCP-47 target language tags"`
	ContentPaths    []contentPath `json:"content_paths" jsonschema:"paths to localizable content"`
}

type contentPath struct {
	Path   string `json:"path" jsonschema:"glob pattern for content files"`
	Format string `json:"format" jsonschema:"content format (json, markdown, html, etc.)"`
}

type onboardProjectOutput struct {
	WorkspaceSlug string `json:"workspace_slug"`
	PlanPath      string `json:"plan_path"`
	Status        string `json:"status"`
}

func (s *Server) handleOnboardProject(ctx context.Context, req *mcp.CallToolRequest, input onboardProjectInput) (*mcp.CallToolResult, onboardProjectOutput, error) {
	if s.fleetRepo == nil {
		return nil, onboardProjectOutput{}, fmt.Errorf("fleet repo not configured")
	}

	// Generate workspace slug from project name.
	slug := generateSlug(input.Name)

	// Generate plan.yaml content.
	planContent := generatePlanYAML(input)

	// Commit the plan to the fleet repo.
	_, err := s.fleetRepo.CommitFile(ctx,
		fmt.Sprintf("workspaces/%s/plan.yaml", slug),
		planContent,
		fmt.Sprintf("onboard: %s (%s)", input.Name, input.UpstreamRepo),
	)
	if err != nil {
		return nil, onboardProjectOutput{}, fmt.Errorf("commit plan: %w", err)
	}

	// Create initial status.yaml.
	statusContent := "phase: planned\ncurrent_release: null\n"
	_, err = s.fleetRepo.CommitFile(ctx,
		fmt.Sprintf("workspaces/%s/status.yaml", slug),
		statusContent,
		fmt.Sprintf("onboard: initial status for %s", input.Name),
	)
	if err != nil {
		return nil, onboardProjectOutput{}, fmt.Errorf("commit status: %w", err)
	}

	return nil, onboardProjectOutput{
		WorkspaceSlug: slug,
		PlanPath:      fmt.Sprintf("workspaces/%s/plan.yaml", slug),
		Status:        "planned",
	}, nil
}

// generateSlug creates a workspace slug from a project name.
func generateSlug(name string) string {
	slug := ""
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			slug += string(c)
		case c >= 'A' && c <= 'Z':
			slug += string(c + 32) // lowercase
		case c == ' ', c == '_':
			slug += "-"
		}
	}
	return slug + "-l10n"
}

// generatePlanYAML creates a plan.yaml from onboarding input.
func generatePlanYAML(input onboardProjectInput) string {
	plan := fmt.Sprintf(`upstream:
  repo: %s

project:
  name: %s
  source_language: %s
  target_languages:`, input.UpstreamRepo, input.Name, input.SourceLanguage)

	for _, lang := range input.TargetLanguages {
		plan += fmt.Sprintf("\n    - %s", lang)
	}

	plan += "\n\ncontent:\n  paths:"
	for _, cp := range input.ContentPaths {
		plan += fmt.Sprintf("\n    - path: %q\n      format: %s", cp.Path, cp.Format)
	}

	plan += `

release_strategy:
  mode: accelerated-then-realtime
  start_tag: null
  end_tag: latest
`
	return plan
}
