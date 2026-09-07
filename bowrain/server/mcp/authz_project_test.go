package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	venueconn "github.com/neokapi/neokapi/core/venue/connector"
)

// The tenancy these tests model: user-a belongs to ws-a and to nothing else,
// ws-b is somebody else's workspace, and each holds one project.
const (
	tenantUser  = "user-a"
	tenantOwn   = "ws-a"
	tenantOther = "ws-b"
)

// callAs is the tool request an authenticated caller arrives with. Bearer
// token validation has already run by the time a handler sees it, so the user
// id in the token is the identity the membership check uses.
func callAs(userID string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: &auth.TokenInfo{UserID: userID}}}
}

// recordingConnectors is a ConnectorResolver that answers every call and
// remembers the project id it was handed, so a test can prove a refused call
// never reached the connector.
type recordingConnectors struct{ projects []string }

func (r *recordingConnectors) GetConnector(string, string) (connector.IntegrationConnector, error) {
	return nil, nil
}

func (r *recordingConnectors) Fetch(_ context.Context, _, _, projectID string, _ connector.FetchOptions) ([]*connector.ContentItem, error) {
	r.projects = append(r.projects, projectID)
	return nil, nil
}

func (r *recordingConnectors) Publish(_ context.Context, _, _, projectID string, _ connector.PublishOptions) error {
	r.projects = append(r.projects, projectID)
	return nil
}

func (r *recordingConnectors) ConnectorStatus(context.Context, string, string) (*venueconn.SyncStatus, error) {
	return &venueconn.SyncStatus{}, nil
}

// tenantFixture is an MCP server wired the way the platform wires it, with a
// membership checker, one project in the caller's workspace and one in a
// workspace the caller has nothing to do with.
type tenantFixture struct {
	ms         *MCPServer
	cs         store.ContentStore
	own        string // project id in ws-a
	other      string // project id in ws-b
	connectors *recordingConnectors
}

func newTenantFixture(t *testing.T) *tenantFixture {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	defs := bstore.NewFlowDefStore(db.DB)

	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	conns := &recordingConnectors{}
	voice := &memVoiceStore{profiles: []*coreprofile.VoiceProfile{{ID: "vp1", Name: "Voice", Scope: tenantOwn}}}
	ms, err := NewMCPServerWithStore(voice, cs, Config{},
		WithMembershipChecker(&fakeMembership{members: map[string]bool{tenantOwn + "/" + tenantUser: true}}),
		WithToolRegistry(reg),
		WithFlowCatalog(service.NewFlowCatalog(defs)),
		WithFlowRunner(service.NewFlowService(cs, nil, reg)),
		WithConnectorResolver(conns),
	)
	require.NoError(t, err)

	return &tenantFixture{
		ms:         ms,
		cs:         cs,
		own:        seedTenantProject(t, cs, tenantOwn, "Mine"),
		other:      seedTenantProject(t, cs, tenantOther, "Theirs"),
		connectors: conns,
	}
}

func seedTenantProject(t *testing.T, cs store.ContentStore, workspaceID, name string) string {
	t.Helper()
	ctx := context.Background()
	p := &store.Project{
		WorkspaceID:           workspaceID,
		Name:                  name,
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{"fr"},
	}
	require.NoError(t, cs.CreateProject(ctx, p))
	b := model.NewBlock("b1", "Hello there.")
	b.Translatable = true
	require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))
	return p.ID
}

// projectScopedTools is every MCP tool that takes a project_id, grouped by the
// family it belongs to. Each entry runs its tool against one project id.
func projectScopedTools() []struct {
	family string
	tool   string
	call   func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, projectID string) error
} {
	return []struct {
		family string
		tool   string
		call   func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, projectID string) error
	}{
		{"content", "get_project", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleGetProject(ctx, req, getProjectInput{ProjectID: id})
			return err
		}},
		{"content", "update_project", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleUpdateProject(ctx, req, updateProjectInput{ProjectID: id, Name: "Renamed"})
			return err
		}},
		{"content", "list_blocks", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleListBlocks(ctx, req, listBlocksInput{ProjectID: id})
			return err
		}},
		{"content", "get_block", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleGetBlock(ctx, req, getBlockInput{ProjectID: id, BlockID: "b1", Stream: "main"})
			return err
		}},
		{"content", "update_block", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleUpdateBlock(ctx, req, updateBlockInput{
				ProjectID: id, BlockID: "b1", TargetLocale: "fr", TargetText: "Bonjour",
			})
			return err
		}},
		{"content", "create_version", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleCreateVersion(ctx, req, createVersionInput{ProjectID: id, Stream: "main", Label: "v1"})
			return err
		}},
		{"content", "list_streams", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleListStreams(ctx, req, listStreamsInput{ProjectID: id})
			return err
		}},
		{"content", "diff_streams", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleDiffStreams(ctx, req, diffStreamsInput{ProjectID: id, StreamName: "main"})
			return err
		}},
		{"content", "merge_stream", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleMergeStream(ctx, req, mergeStreamInput{ProjectID: id, StreamName: "main", DryRun: true})
			return err
		}},
		{"flow", "list_flows", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleListFlows(ctx, req, listFlowsInput{ProjectID: id})
			return err
		}},
		{"flow", "run_flow", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleRunFlow(ctx, req, runFlowInput{ProjectID: id, FlowName: "pseudo-translate", TargetLocale: "fr"})
			return err
		}},
		{"loop", "evaluate_rule", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleEvaluateRule(ctx, req, evaluateRuleInput{ProfileID: "vp1", Term: "utilize", ProjectID: id})
			return err
		}},
		{"scoring", "score_voice_compliance", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleScoreVoiceCompliance(ctx, req, scoreVoiceComplianceInput{ProfileID: "vp1", Text: "hello", ProjectID: id})
			return err
		}},
		{"scoring", "suggest_corrections", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleSuggestCorrections(ctx, req, suggestCorrectionsInput{ProfileID: "vp1", Text: "hello", ProjectID: id})
			return err
		}},
		{"scoring", "rewrite_in_voice", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleRewriteInVoice(ctx, req, rewriteInVoiceInput{ProfileID: "vp1", Text: "hello", ProjectID: id})
			return err
		}},
		{"connector", "connector_pull", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleConnectorPull(ctx, req, connectorPullInput{
				WorkspaceID: tenantOwn, ProjectID: id, ConnectorID: "c1",
			})
			return err
		}},
		{"connector", "connector_push", func(ctx context.Context, ms *MCPServer, req *mcp.CallToolRequest, id string) error {
			_, _, err := ms.handleConnectorPush(ctx, req, connectorPushInput{
				WorkspaceID: tenantOwn, ProjectID: id, ConnectorID: "c1",
			})
			return err
		}},
	}
}

// TestProjectScopedTools_RefuseForeignProject proves every MCP tool taking a
// project_id refuses a project owned by a workspace the caller does not belong
// to, and answers "not found" rather than "forbidden" so the refusal does not
// confirm the project exists.
func TestProjectScopedTools_RefuseForeignProject(t *testing.T) {
	f := newTenantFixture(t)
	req := callAs(tenantUser)

	for _, tc := range projectScopedTools() {
		t.Run(tc.family+"/"+tc.tool, func(t *testing.T) {
			err := tc.call(t.Context(), f.ms, req, f.other)
			require.Error(t, err, "a project in another workspace must be refused")
			require.ErrorIs(t, err, ErrProjectNotFound)
			assert.NotContains(t, err.Error(), "forbidden")
			assert.NotContains(t, err.Error(), "not authorized")
		})
	}

	// The connector resolver was never reached for the foreign project.
	assert.Empty(t, f.connectors.projects, "a refused call must not reach the connector")
}

// TestProjectScopedTools_AllowOwnProject proves the same tools still work on a
// project in the caller's own workspace: the guard refuses a tenant boundary,
// not every call.
func TestProjectScopedTools_AllowOwnProject(t *testing.T) {
	f := newTenantFixture(t)
	req := callAs(tenantUser)

	for _, tc := range projectScopedTools() {
		t.Run(tc.family+"/"+tc.tool, func(t *testing.T) {
			err := tc.call(t.Context(), f.ms, req, f.own)
			require.NotErrorIs(t, err, ErrProjectNotFound,
				"the caller's own project must stay reachable")
		})
	}

	assert.Equal(t, []string{f.own, f.own}, f.connectors.projects,
		"the connector was handed the caller's own project, pull then push")
}

// TestProjectScopedTools_RefuseUnauthenticatedCaller proves a call carrying no
// token reaches nothing: the membership check has no identity to satisfy it.
func TestProjectScopedTools_RefuseUnauthenticatedCaller(t *testing.T) {
	f := newTenantFixture(t)

	_, _, err := f.ms.handleGetProject(t.Context(), nil, getProjectInput{ProjectID: f.own})
	require.ErrorIs(t, err, ErrProjectNotFound)

	_, _, err = f.ms.handleGetProject(t.Context(), callAs(""), getProjectInput{ProjectID: f.own})
	require.ErrorIs(t, err, ErrProjectNotFound)

	_, _, err = f.ms.handleGetProject(t.Context(), callAs("nobody"), getProjectInput{ProjectID: f.own})
	require.ErrorIs(t, err, ErrProjectNotFound)
}

// TestProjectByName_StaysInsideTheWorkspace proves the name lookup a
// project_id may go through cannot cross a workspace either: an agent naming
// another workspace's project by name gets the same "not found".
func TestProjectByName_StaysInsideTheWorkspace(t *testing.T) {
	f := newTenantFixture(t)
	req := callAs(tenantUser)

	_, out, err := f.ms.handleGetProject(t.Context(), req, getProjectInput{ProjectID: "Mine"})
	require.NoError(t, err, "the caller's own project resolves by name")
	assert.Equal(t, f.own, out.ID)

	_, _, err = f.ms.handleGetProject(t.Context(), req, getProjectInput{ProjectID: "Theirs"})
	require.ErrorIs(t, err, ErrProjectNotFound, "a foreign project's name resolves to nothing")

	_, _, err = f.ms.handleGetProject(t.Context(), req, getProjectInput{ProjectID: "no-such-project"})
	require.ErrorIs(t, err, ErrProjectNotFound)
}

// TestListProjects_ListsOnlyTheCallersWorkspaces proves list_projects is
// scoped by membership rather than by the client-supplied workspace_id, which
// it ignores.
func TestListProjects_ListsOnlyTheCallersWorkspaces(t *testing.T) {
	f := newTenantFixture(t)

	_, out, err := f.ms.handleListProjects(t.Context(), callAs(tenantUser), listProjectsInput{})
	require.NoError(t, err)
	require.Len(t, out.Projects, 1)
	assert.Equal(t, f.own, out.Projects[0].ID)

	// Naming the other workspace changes nothing: the field is ignored and
	// membership decides.
	_, out, err = f.ms.handleListProjects(t.Context(), callAs(tenantUser), listProjectsInput{WorkspaceID: tenantOther})
	require.NoError(t, err)
	require.Len(t, out.Projects, 1)
	assert.Equal(t, f.own, out.Projects[0].ID)

	// A caller belonging to no workspace sees nothing.
	_, out, err = f.ms.handleListProjects(t.Context(), callAs("nobody"), listProjectsInput{})
	require.NoError(t, err)
	assert.Empty(t, out.Projects)
}

// TestCreateProject_RefusesForeignWorkspace proves create_project lands a
// project only in a workspace the caller belongs to.
func TestCreateProject_RefusesForeignWorkspace(t *testing.T) {
	f := newTenantFixture(t)
	req := callAs(tenantUser)

	_, _, err := f.ms.handleCreateProject(t.Context(), req, createProjectInput{
		WorkspaceID: tenantOther, Name: "Planted", SourceLanguage: "en-US",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized for workspace")

	_, out, err := f.ms.handleCreateProject(t.Context(), req, createProjectInput{
		WorkspaceID: tenantOwn, Name: "Fine", SourceLanguage: "en-US",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
}

// TestGetSuggestedRules_RefusesForeignWorkspace covers the loop tool that
// takes a workspace_id: it was reading another workspace's corrections.
func TestGetSuggestedRules_RefusesForeignWorkspace(t *testing.T) {
	f := newTenantFixture(t)

	_, _, err := f.ms.handleGetSuggestedRules(t.Context(), callAs(tenantUser), getSuggestedRulesInput{
		WorkspaceID: tenantOther, ProfileID: "vp1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized for workspace")

	_, _, err = f.ms.handleGetSuggestedRules(t.Context(), callAs(tenantUser), getSuggestedRulesInput{
		WorkspaceID: tenantOwn, ProfileID: "vp1",
	})
	require.NoError(t, err)
}

// TestAuthorizeProject_WithoutMembershipChecker proves a deployment with no
// auth is unaffected: there is no tenant to cross, so every project resolves.
func TestAuthorizeProject_WithoutMembershipChecker(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ms, err := NewMCPServerWithStore(&memVoiceStore{}, cs, Config{})
	require.NoError(t, err)

	own := seedTenantProject(t, cs, tenantOwn, "Mine")
	other := seedTenantProject(t, cs, tenantOther, "Theirs")

	for _, id := range []string{own, other} {
		_, out, err := ms.handleGetProject(t.Context(), nil, getProjectInput{ProjectID: id})
		require.NoError(t, err)
		assert.Equal(t, id, out.ID)
	}
}

// TestAuthorizeProject_NoContentStoreFailsClosed proves a server that cannot
// read a project's workspace refuses rather than guessing, once a membership
// checker says the deployment has tenants at all.
func TestAuthorizeProject_NoContentStoreFailsClosed(t *testing.T) {
	guarded := &MCPServer{membership: &fakeMembership{members: map[string]bool{tenantOwn + "/" + tenantUser: true}}}
	_, err := guarded.authorizeProject(t.Context(), callAs(tenantUser), "p1")
	require.ErrorIs(t, err, ErrProjectNotFound)

	open := &MCPServer{}
	got, err := open.authorizeProject(t.Context(), nil, "p1")
	require.NoError(t, err)
	assert.Equal(t, "p1", got)
}

// TestAuthorizeOptionalProject_EmptyPassesThrough proves the tools whose
// project_id is optional still run without one.
func TestAuthorizeOptionalProject_EmptyPassesThrough(t *testing.T) {
	f := newTenantFixture(t)

	got, err := f.ms.authorizeOptionalProject(t.Context(), callAs(tenantUser), "")
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = f.ms.authorizeProject(t.Context(), callAs(tenantUser), "")
	require.ErrorIs(t, err, ErrProjectNotFound, "a required project_id may not be empty")

	_, out, err := f.ms.handleListFlows(t.Context(), callAs(tenantUser), listFlowsInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Flows, "the built-in catalog lists without a project")
}
