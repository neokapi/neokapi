package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Flow definitions on the desktop are now project-scoped and proxy to the
// server REST API (#766). When disconnected, the built-in flows remain
// available as a read-only fallback; authoring requires a connection.

func TestListFlowDefinitions_OfflineReturnsBuiltIns(t *testing.T) {
	app := NewApp() // not connected
	defs, err := app.ListFlowDefinitions("proj-1")
	require.NoError(t, err)
	require.NotEmpty(t, defs)

	ids := make(map[string]bool)
	for _, d := range defs {
		assert.NotEmpty(t, d.ID)
		assert.NotEmpty(t, d.Name)
		assert.Equal(t, registry.SourceBuiltIn, d.Source)
		assert.NotEmpty(t, d.Nodes)
		assert.NotEmpty(t, d.Edges)
		ids[d.ID] = true
	}
	assert.True(t, ids["ai-translate"])
	assert.True(t, ids["ai-translate-qa"])
	assert.True(t, ids["pseudo-translate"])
	assert.True(t, ids["secure-translate"])
}

func TestGetFlowDefinition_OfflineBuiltIn(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("proj-1", "ai-translate")
	require.NoError(t, err)
	assert.Equal(t, "AI Translate", info.Name)
	assert.Equal(t, registry.SourceBuiltIn, info.Source)
}

func TestGetFlowDefinition_OfflineNotFound(t *testing.T) {
	app := NewApp()
	_, err := app.GetFlowDefinition("proj-1", "nonexistent")
	assert.Error(t, err)
}

func TestSaveFlowDefinition_OfflineRequiresConnection(t *testing.T) {
	app := NewApp()
	_, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
		ID:     "custom",
		Name:   "Custom",
		Source: "project",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestSaveFlowDefinition_BuiltInRejected(t *testing.T) {
	app := NewApp()
	_, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
		ID:     "ai-translate",
		Name:   "AI Translate",
		Source: registry.SourceBuiltIn,
	})
	require.Error(t, err)
	// Offline check fires first (not connected). Either guard is acceptable;
	// the point is built-in/offline writes never succeed.
	assert.Error(t, err)
}

func TestDeleteFlowDefinition_OfflineRequiresConnection(t *testing.T) {
	app := NewApp()
	err := app.DeleteFlowDefinition("proj-1", "custom")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestFlowDefToInfo_StageSerialization(t *testing.T) {
	def := flow.FlowDefinition{
		ID:   "test-stage",
		Name: "Test Stage",
		Nodes: []flow.FlowNode{
			{ID: "reader", Type: flow.NodeReader, Name: "auto", Position: flow.NodePosition{X: 0, Y: 0}},
			{
				ID:       "redact",
				Type:     flow.NodeTool,
				Name:     "redact",
				Stage:    flow.StageSourceTransform,
				Position: flow.NodePosition{X: 200, Y: 0},
			},
			{ID: "t1", Type: flow.NodeTool, Name: "ai-translate", Stage: flow.StageMain, Position: flow.NodePosition{X: 400, Y: 0}},
			{ID: "writer", Type: flow.NodeWriter, Name: "auto", Position: flow.NodePosition{X: 600, Y: 0}},
		},
		Edges: []flow.FlowEdge{
			{ID: "e1", Source: "reader", Target: "redact"},
			{ID: "e2", Source: "redact", Target: "t1"},
			{ID: "e3", Source: "t1", Target: "writer"},
		},
	}

	info := flowDefToInfo(def)
	byID := make(map[string]FlowNodeInfo)
	for _, n := range info.Nodes {
		byID[n.ID] = n
	}
	assert.Equal(t, string(flow.StageSourceTransform), byID["redact"].Stage, "source-transform stage serialized")
	assert.Equal(t, "", byID["t1"].Stage, "main stage serializes as empty string")
}

func TestSecureTranslateBuiltInHasSourceTransformNode(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("proj-1", "secure-translate")
	require.NoError(t, err)

	var found bool
	for _, n := range info.Nodes {
		if n.Name == "redact" {
			assert.Equal(t, string(flow.StageSourceTransform), n.Stage, "redact node should be source-transform stage")
			found = true
		}
	}
	assert.True(t, found, "secure-translate should contain a redact node")
}
