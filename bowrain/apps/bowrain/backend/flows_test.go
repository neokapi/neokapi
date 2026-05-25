package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowDefToInfoRoundTrip(t *testing.T) {
	app := NewApp()
	defs := app.ListFlowDefinitions()
	require.NotEmpty(t, defs)

	for _, d := range defs {
		assert.NotEmpty(t, d.ID)
		assert.NotEmpty(t, d.Name)
		if d.Source == registry.SourceBuiltIn {
			assert.NotEmpty(t, d.Nodes)
			assert.NotEmpty(t, d.Edges)
		}
	}
}

func TestListFlowDefinitionsIncludesBuiltIn(t *testing.T) {
	app := NewApp()
	defs := app.ListFlowDefinitions()

	ids := make(map[string]bool)
	for _, d := range defs {
		ids[d.ID] = true
	}
	assert.True(t, ids["ai-translate"])
	assert.True(t, ids["ai-translate-qa"])
	assert.True(t, ids["pseudo-translate"])
}

func TestGetFlowDefinitionBuiltIn(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("ai-translate")
	require.NoError(t, err)
	assert.Equal(t, "AI Translate", info.Name)
	assert.Equal(t, registry.SourceBuiltIn, info.Source)
}

func TestGetFlowDefinitionNotFound(t *testing.T) {
	app := NewApp()
	_, err := app.GetFlowDefinition("nonexistent")
	assert.Error(t, err)
}

func TestFlowNodeStageRoundTrip(t *testing.T) {
	// Build a FlowDefinition with a source-transform node.
	def := flow.FlowDefinition{
		ID:   "test-stage-rt",
		Name: "Test Stage Round-Trip",
		Nodes: []flow.FlowNode{
			{ID: "reader", Type: flow.NodeReader, Name: "auto", Position: flow.NodePosition{X: 0, Y: 0}},
			{
				ID:    "redact",
				Type:  flow.NodeTool,
				Name:  "redact",
				Stage: flow.StageSourceTransform,
				Position: flow.NodePosition{X: 200, Y: 0},
			},
			{ID: "writer", Type: flow.NodeWriter, Name: "auto", Position: flow.NodePosition{X: 400, Y: 0}},
		},
		Edges: []flow.FlowEdge{
			{ID: "e1", Source: "reader", Target: "redact"},
			{ID: "e2", Source: "redact", Target: "writer"},
		},
	}

	// flowDefToInfo should carry Stage into FlowNodeInfo.Stage.
	info := flowDefToInfo(def)
	var redactInfo *FlowNodeInfo
	for i := range info.Nodes {
		if info.Nodes[i].ID == "redact" {
			redactInfo = &info.Nodes[i]
		}
	}
	require.NotNil(t, redactInfo, "redact node not found in info")
	assert.Equal(t, string(flow.StageSourceTransform), redactInfo.Stage, "stage should be serialized into FlowNodeInfo")

	// infoToFlowDef should carry it back.
	def2 := infoToFlowDef(info)
	var redactNode *flow.FlowNode
	for i := range def2.Nodes {
		if def2.Nodes[i].ID == "redact" {
			redactNode = &def2.Nodes[i]
		}
	}
	require.NotNil(t, redactNode, "redact node not found in def2")
	assert.Equal(t, flow.StageSourceTransform, redactNode.Stage, "stage should survive round-trip through info")
}

func TestFlowNodeMainStageIsEmpty(t *testing.T) {
	def := flow.FlowDefinition{
		ID:   "test-main",
		Name: "Test Main",
		Nodes: []flow.FlowNode{
			{ID: "t1", Type: flow.NodeTool, Name: "ai-translate", Stage: flow.StageMain, Position: flow.NodePosition{}},
		},
		Edges: []flow.FlowEdge{},
	}
	info := flowDefToInfo(def)
	assert.Equal(t, "", info.Nodes[0].Stage, "main stage serializes as empty string")

	def2 := infoToFlowDef(info)
	assert.Equal(t, flow.StageMain, def2.Nodes[0].Stage, "empty string deserializes back to StageMain")
}

func TestSecureTranslateBuiltInHasSourceTransformNode(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("secure-translate")
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

func TestDeleteBuiltInFlowFails(t *testing.T) {
	app := NewApp()
	err := app.DeleteFlowDefinition("ai-translate")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
}

func TestSaveBuiltInFlowFails(t *testing.T) {
	app := NewApp()
	_, err := app.SaveFlowDefinition(FlowDefinitionInfo{
		ID:     "test",
		Name:   "test",
		Source: registry.SourceBuiltIn,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
}
