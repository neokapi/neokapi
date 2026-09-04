package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFlowToolServer wires the flow tools the way the server wires them: the
// catalog over a project flow store and the store-backed flow service as the
// runner, with the built-in tools registered.
func newFlowToolServer(t *testing.T) (*MCPServer, store.ContentStore, *bstore.FlowDefStore) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	defs := bstore.NewFlowDefStore(db.DB)

	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	ms, err := NewMCPServerWithStore(&memVoiceStore{}, cs, Config{},
		WithToolRegistry(reg),
		WithFlowCatalog(service.NewFlowCatalog(defs)),
		WithFlowRunner(service.NewFlowService(cs, nil, reg)),
	)
	require.NoError(t, err)
	return ms, cs, defs
}

func seedFlowProject(t *testing.T, cs store.ContentStore) string {
	t.Helper()
	ctx := context.Background()
	p := &store.Project{Name: "Flow tools", DefaultSourceLanguage: model.LocaleEnglish, TargetLanguages: []model.LocaleID{"fr"}}
	require.NoError(t, cs.CreateProject(ctx, p))
	blocks := []*model.Block{model.NewBlock("b1", "Hello there."), model.NewBlock("b2", "Good morning.")}
	for _, b := range blocks {
		b.Translatable = true
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", "en.json", blocks))
	return p.ID
}

// TestHandleRunFlow_ProcessesStoredBlocks proves run_flow streams the
// project's blocks through the flow and writes the result back: every block
// carries the pseudo target afterwards, and the count reported matches.
func TestHandleRunFlow_ProcessesStoredBlocks(t *testing.T) {
	ms, cs, _ := newFlowToolServer(t)
	ctx := t.Context()
	projID := seedFlowProject(t, cs)

	_, out, err := ms.handleRunFlow(ctx, nil, runFlowInput{
		ProjectID:    projID,
		FlowName:     "pseudo-translate",
		TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", out.Status)
	assert.Equal(t, 2, out.BlocksUpdated)
	assert.Empty(t, out.Message)

	blocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projID, Stream: "main"})
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	for _, sb := range blocks {
		assert.Contains(t, sb.TargetText("fr"), "▒", "block %s was handed to the flow", sb.ID)
	}
}

// TestHandleRunFlow_ResolvesProjectFlows proves a flow authored on the project
// runs by id like a built-in one, and one authored elsewhere does not.
func TestHandleRunFlow_ResolvesProjectFlows(t *testing.T) {
	ms, cs, defs := newFlowToolServer(t)
	ctx := t.Context()
	projID := seedFlowProject(t, cs)
	other := seedFlowProject(t, cs)

	require.NoError(t, defs.Upsert(ctx, projID, &flow.FlowDefinition{
		ID:    "mine",
		Name:  "Mine",
		Nodes: []flow.FlowNode{{ID: "p", Type: flow.NodeTool, Name: "pseudo-translate"}},
	}))

	_, out, err := ms.handleRunFlow(ctx, nil, runFlowInput{ProjectID: projID, FlowName: "mine", TargetLocale: "fr"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.BlocksUpdated)

	_, _, err = ms.handleRunFlow(ctx, nil, runFlowInput{ProjectID: other, FlowName: "mine", TargetLocale: "fr"})
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrFlowNotFound)
}

// TestHandleRunFlow_UnknownFlow proves an unknown id is an error naming it.
func TestHandleRunFlow_UnknownFlow(t *testing.T) {
	ms, cs, _ := newFlowToolServer(t)
	projID := seedFlowProject(t, cs)

	_, _, err := ms.handleRunFlow(t.Context(), nil, runFlowInput{ProjectID: projID, FlowName: "no-such-flow"})
	require.Error(t, err)
	require.ErrorIs(t, err, service.ErrFlowNotFound)
	assert.True(t, strings.Contains(err.Error(), "no-such-flow"), err.Error())
}

// TestHandleRunFlow_RequiresInputsAndRunner covers the refusals.
func TestHandleRunFlow_RequiresInputsAndRunner(t *testing.T) {
	ms, cs, _ := newFlowToolServer(t)
	ctx := t.Context()
	projID := seedFlowProject(t, cs)

	_, _, err := ms.handleRunFlow(ctx, nil, runFlowInput{ProjectID: projID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow_name is required")

	_, _, err = ms.handleRunFlow(ctx, nil, runFlowInput{FlowName: "qa"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_id is required")

	bare := &MCPServer{}
	_, _, err = bare.handleRunFlow(ctx, nil, runFlowInput{ProjectID: projID, FlowName: "qa"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow runner not configured")
}

// TestHandleRunFlow_EmptyProjectCompletesWithMessage proves a project with no
// blocks completes and says so.
func TestHandleRunFlow_EmptyProjectCompletesWithMessage(t *testing.T) {
	ms, cs, _ := newFlowToolServer(t)
	ctx := t.Context()
	p := &store.Project{Name: "Empty", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, cs.CreateProject(ctx, p))

	_, out, err := ms.handleRunFlow(ctx, nil, runFlowInput{ProjectID: p.ID, FlowName: "pseudo-translate", TargetLocale: "fr"})
	require.NoError(t, err)
	assert.Equal(t, "completed", out.Status)
	assert.Equal(t, 0, out.BlocksUpdated)
	assert.Equal(t, "No blocks to process.", out.Message)
}

// TestHandleListFlows_IncludesProjectFlows proves list_flows adds the
// project's own flows beside the built-in catalog when a project is named.
func TestHandleListFlows_IncludesProjectFlows(t *testing.T) {
	ms, cs, defs := newFlowToolServer(t)
	ctx := t.Context()
	projID := seedFlowProject(t, cs)
	require.NoError(t, defs.Upsert(ctx, projID, &flow.FlowDefinition{
		ID:    "mine",
		Name:  "Mine",
		Nodes: []flow.FlowNode{{ID: "q", Type: flow.NodeTool, Name: "qa"}},
	}))

	_, out, err := ms.handleListFlows(ctx, nil, listFlowsInput{ProjectID: projID})
	require.NoError(t, err)
	kinds := map[string]string{}
	for _, f := range out.Flows {
		kinds[f.Name] = f.Type
	}
	assert.Equal(t, "builtin", kinds["qa"])
	assert.Equal(t, "custom", kinds["mine"])

	_, out, err = ms.handleListFlows(ctx, nil, listFlowsInput{})
	require.NoError(t, err)
	for _, f := range out.Flows {
		assert.Equal(t, "builtin", f.Type, "without a project only the catalog is listed")
	}
}
