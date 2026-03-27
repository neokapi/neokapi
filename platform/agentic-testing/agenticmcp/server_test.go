package agenticmcp

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock implementations ─────────────────────────────────────────────────

type mockFleetRepo struct {
	workspaces []WorkspaceMeta
	plans      map[string]*WorkspacePlan
	commits    []commitRecord
}

type commitRecord struct {
	Path, Content, Message string
}

func (m *mockFleetRepo) ListWorkspaces(_ context.Context) ([]WorkspaceMeta, error) {
	return m.workspaces, nil
}

func (m *mockFleetRepo) GetWorkspacePlan(_ context.Context, slug string) (*WorkspacePlan, error) {
	if p, ok := m.plans[slug]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("workspace %q not found", slug)
}

func (m *mockFleetRepo) CommitFile(_ context.Context, path, content, message string) (string, error) {
	m.commits = append(m.commits, commitRecord{path, content, message})
	return "abc1234", nil
}

func (m *mockFleetRepo) ListMemoryLog(_ context.Context, _ int) ([]MemoryLogEntry, error) {
	return nil, nil
}

func (m *mockFleetRepo) GetWorkspaceStatus(_ context.Context, slug string) (*WorkspaceStatus, error) {
	return &WorkspaceStatus{Phase: "active", CurrentRelease: "v0.14.0"}, nil
}

func (m *mockFleetRepo) ReadAgentFile(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

type mockWalker struct {
	calls []struct{ Workspace, Project, Tag string }
}

func (m *mockWalker) WalkRelease(_ context.Context, ws, proj, tag string) (*ReleaseResult, error) {
	m.calls = append(m.calls, struct{ Workspace, Project, Tag string }{ws, proj, tag})
	return &ReleaseResult{
		Tag:           "v0.18.1",
		BlocksChanged: 42,
		BlocksAdded:   38,
		BlocksRemoved: 4,
	}, nil
}

type mockIssueTracker struct {
	issues []struct{ Title, Body string }
}

func (m *mockIssueTracker) FileIssue(_ context.Context, title, body string, _ []string) (string, int, error) {
	m.issues = append(m.issues, struct{ Title, Body string }{title, body})
	return "https://github.com/neokapi/agent-feedback/issues/1", 1, nil
}

// ── Test data ────────────────────────────────────────────────────────────

func testWorkspaces() []WorkspaceMeta {
	return []WorkspaceMeta{
		{
			Slug:            "excalidraw-l10n",
			Phase:           "active",
			ProjectName:     "Excalidraw",
			UpstreamRepo:    "excalidraw/excalidraw",
			TargetLanguages: []string{"fr-FR", "de-DE", "ja-JP"},
			Mode:            "real-time",
			ProjectCount:    1,
			Health:          "healthy",
			UntranslatedBlocks: map[string]int{
				"fr-FR": 142,
				"de-DE": 89,
			},
		},
		{
			Slug:            "docusaurus-l10n",
			Phase:           "active",
			ProjectName:     "Docusaurus",
			UpstreamRepo:    "facebook/docusaurus",
			TargetLanguages: []string{"fr-FR", "de-DE"},
			Mode:            "accelerated",
			ProjectCount:    1,
			Health:          "healthy",
		},
	}
}

func testPlan() *WorkspacePlan {
	return &WorkspacePlan{
		ProjectName:     "Excalidraw",
		SourceLanguage:  "en-US",
		TargetLanguages: []string{"fr-FR", "de-DE", "ja-JP"},
		UpstreamRepo:    "excalidraw/excalidraw",
		Mode:            "real-time",
	}
}

// ── Test helpers ─────────────────────────────────────────────────────────

func newTestServer(t *testing.T, opts ...Option) *Server {
	t.Helper()
	s, err := NewServer(Config{}, opts...)
	require.NoError(t, err)
	return s
}

func newFullTestServer(t *testing.T) (*Server, *mockFleetRepo, *mockWalker, *mockIssueTracker) {
	t.Helper()
	fleet := &mockFleetRepo{
		workspaces: testWorkspaces(),
		plans:      map[string]*WorkspacePlan{"excalidraw-l10n": testPlan()},
	}
	walker := &mockWalker{}
	issues := &mockIssueTracker{}

	s := newTestServer(t,
		WithFleetRepo(fleet),
		WithReleaseWalker(walker),
		WithIssueTracker(issues),
	)
	return s, fleet, walker, issues
}

// ── Protocol-level tests ─────────────────────────────────────────────────

func TestServerInitialize(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()
}

func TestServerListTools(t *testing.T) {
	s, _, _, _ := newFullTestServer(t)
	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	var toolNames []string
	for _, tool := range result.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	expected := []string{
		"get_fleet_summary",
		"get_workspace_status",
		"list_workspaces",
		"list_agent_executions",
		"list_agent_events",
		"walk_release",
		"onboard_project",
		"file_feedback_issue",
		"commit_memory",
	}
	for _, name := range expected {
		assert.Contains(t, toolNames, name)
	}
	assert.Len(t, result.Tools, 12, "expected exactly 12 tools (9 fleet + 3 task)")
}

// ── Handler-level tests (direct, no transport) ───────────────────────────

func TestHandleGetFleetSummary(t *testing.T) {
	s, _, _, _ := newFullTestServer(t)
	ctx := context.Background()

	_, out, err := s.handleGetFleetSummary(ctx, nil, getFleetSummaryInput{})
	require.NoError(t, err)

	assert.Len(t, out.Workspaces, 2)
	assert.Equal(t, "excalidraw-l10n", out.Workspaces[0].Slug)
	assert.Equal(t, "docusaurus-l10n", out.Workspaces[1].Slug)
	assert.Equal(t, 2, out.Global.TotalWorkspaces)

	// First workspace should have untranslated blocks.
	assert.Equal(t, 142, out.Workspaces[0].UntranslatedBlocks["fr-FR"])

	// No exec store wired — last session and active sessions should be empty.
	assert.Nil(t, out.Workspaces[0].LastAgentSession)
	assert.Equal(t, 0, out.Global.ActiveSessions)
}

func TestHandleGetFleetSummary_NoFleetRepo(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	_, _, err := s.handleGetFleetSummary(ctx, nil, getFleetSummaryInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fleet repo not configured")
}

func TestHandleGetWorkspaceStatus(t *testing.T) {
	s, _, _, _ := newFullTestServer(t)
	ctx := context.Background()

	_, out, err := s.handleGetWorkspaceStatus(ctx, nil, getWorkspaceStatusInput{
		WorkspaceSlug: "excalidraw-l10n",
	})
	require.NoError(t, err)

	assert.Equal(t, "excalidraw-l10n", out.Slug)
	require.Len(t, out.Projects, 1)
	assert.Equal(t, "Excalidraw", out.Projects[0].Name)
	assert.Equal(t, "en-US", out.Projects[0].SourceLanguage)
}

func TestHandleGetWorkspaceStatus_NotFound(t *testing.T) {
	s, _, _, _ := newFullTestServer(t)
	ctx := context.Background()

	_, _, err := s.handleGetWorkspaceStatus(ctx, nil, getWorkspaceStatusInput{
		WorkspaceSlug: "nonexistent",
	})
	assert.Error(t, err)
}

func TestHandleListWorkspaces(t *testing.T) {
	s, _, _, _ := newFullTestServer(t)
	ctx := context.Background()

	_, out, err := s.handleListWorkspaces(ctx, nil, listWorkspacesInput{})
	require.NoError(t, err)

	assert.Len(t, out.Workspaces, 2)
	assert.Equal(t, "excalidraw-l10n", out.Workspaces[0].Slug)
	assert.Equal(t, "Excalidraw", out.Workspaces[0].ProjectName)
	assert.Equal(t, "real-time", out.Workspaces[0].Mode)
	assert.Equal(t, "docusaurus-l10n", out.Workspaces[1].Slug)
}

func TestHandleListAgentExecutions_NoStore(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	_, _, err := s.handleListAgentExecutions(ctx, nil, listAgentExecutionsInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution store not configured")
}

func TestHandleListAgentEvents_NoStore(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	_, _, err := s.handleListAgentEvents(ctx, nil, listAgentEventsInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution store not configured")
}

func TestHandleWalkRelease(t *testing.T) {
	s, _, walker, _ := newFullTestServer(t)
	ctx := context.Background()

	_, out, err := s.handleWalkRelease(ctx, nil, walkReleaseInput{
		WorkspaceSlug: "excalidraw-l10n",
		ProjectID:     "proj_abc123",
		Tag:           "v0.18.1",
	})
	require.NoError(t, err)

	assert.Equal(t, "v0.18.1", out.Tag)
	assert.Equal(t, 42, out.BlocksChanged)
	assert.Equal(t, 38, out.BlocksAdded)
	assert.Equal(t, 4, out.BlocksRemoved)

	require.Len(t, walker.calls, 1)
	assert.Equal(t, "excalidraw-l10n", walker.calls[0].Workspace)
	assert.Equal(t, "proj_abc123", walker.calls[0].Project)
}

func TestHandleWalkRelease_NoTag(t *testing.T) {
	s, _, walker, _ := newFullTestServer(t)
	ctx := context.Background()

	_, _, err := s.handleWalkRelease(ctx, nil, walkReleaseInput{
		WorkspaceSlug: "excalidraw-l10n",
		ProjectID:     "proj_abc123",
	})
	require.NoError(t, err)

	require.Len(t, walker.calls, 1)
	assert.Empty(t, walker.calls[0].Tag, "empty tag means advance to next")
}

func TestHandleOnboardProject(t *testing.T) {
	fleet := &mockFleetRepo{plans: map[string]*WorkspacePlan{}}
	s := newTestServer(t, WithFleetRepo(fleet))
	ctx := context.Background()

	_, out, err := s.handleOnboardProject(ctx, nil, onboardProjectInput{
		UpstreamRepo:    "excalidraw/excalidraw",
		Name:            "Excalidraw",
		SourceLanguage:  "en-US",
		TargetLanguages: []string{"fr-FR", "de-DE"},
		ContentPaths: []contentPath{
			{Path: "packages/excalidraw/locales/*.json", Format: "json"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "excalidraw-l10n", out.WorkspaceSlug)
	assert.Contains(t, out.PlanPath, "plan.yaml")
	assert.Equal(t, "planned", out.Status)

	// Should have committed plan.yaml and status.yaml.
	require.Len(t, fleet.commits, 2)
	assert.Contains(t, fleet.commits[0].Path, "plan.yaml")
	assert.Contains(t, fleet.commits[0].Content, "excalidraw/excalidraw")
	assert.Contains(t, fleet.commits[0].Content, "fr-FR")
	assert.Contains(t, fleet.commits[1].Path, "status.yaml")
	assert.Contains(t, fleet.commits[1].Content, "phase: planned")
}

func TestHandleFileFeedbackIssue(t *testing.T) {
	s, _, _, issues := newFullTestServer(t)
	ctx := context.Background()

	_, out, err := s.handleFileFeedbackIssue(ctx, nil, fileFeedbackIssueInput{
		Title:  "MDX parser fails on custom components",
		Body:   "Seen in excalidraw-l10n and docusaurus-l10n.",
		Labels: []string{"bug", "format-parser"},
	})
	require.NoError(t, err)

	assert.Contains(t, out.IssueURL, "issues/1")
	assert.Equal(t, 1, out.IssueNumber)

	require.Len(t, issues.issues, 1)
	assert.Equal(t, "MDX parser fails on custom components", issues.issues[0].Title)
}

func TestHandleCommitMemory(t *testing.T) {
	fleet := &mockFleetRepo{plans: map[string]*WorkspacePlan{}}
	s := newTestServer(t, WithFleetRepo(fleet))
	ctx := context.Background()

	_, out, err := s.handleCommitMemory(ctx, nil, commitMemoryInput{
		Path:    "coordinator/memory/observation-01.md",
		Content: "Excalidraw fr-FR translations score lower on quality.",
		Message: "memory: quality observation for excalidraw fr-FR",
	})
	require.NoError(t, err)

	assert.Equal(t, "abc1234", out.CommitSHA)

	require.Len(t, fleet.commits, 1)
	assert.Equal(t, "coordinator/memory/observation-01.md", fleet.commits[0].Path)
	assert.Contains(t, fleet.commits[0].Content, "score lower on quality")
}

// ── Unit tests ───────────────────────────────────────────────────────────

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"Excalidraw", "excalidraw-l10n"},
		{"Home Assistant", "home-assistant-l10n"},
		{"React_Native", "react-native-l10n"},
		{"VS Code", "vs-code-l10n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, generateSlug(tc.name))
		})
	}
}

func TestGeneratePlanYAML(t *testing.T) {
	plan := generatePlanYAML(onboardProjectInput{
		UpstreamRepo:    "excalidraw/excalidraw",
		Name:            "Excalidraw",
		SourceLanguage:  "en-US",
		TargetLanguages: []string{"fr-FR", "de-DE"},
		ContentPaths: []contentPath{
			{Path: "locales/*.json", Format: "json"},
		},
	})

	assert.Contains(t, plan, "excalidraw/excalidraw")
	assert.Contains(t, plan, "Excalidraw")
	assert.Contains(t, plan, "en-US")
	assert.Contains(t, plan, "fr-FR")
	assert.Contains(t, plan, "de-DE")
	assert.Contains(t, plan, "locales/*.json")
	assert.Contains(t, plan, "accelerated-then-realtime")
}
