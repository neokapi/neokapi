package flow_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepsToGraph_SourceTransformStage(t *testing.T) {
	spec := &flow.StepsSpec{
		SourceTransforms: []flow.FlowStep{{Tool: "redact"}, {Tool: "simplify"}},
		Steps:            []flow.FlowStep{{Tool: "ai-translate"}, {Tool: "qa-check"}},
	}
	nodes, edges, err := flow.StepsToGraph(spec)
	require.NoError(t, err)

	def := &flow.FlowDefinition{ID: "f", Name: "f", Nodes: nodes, Edges: edges}
	require.NoError(t, def.Validate())

	// Source transforms are marked and come first; main tools follow.
	st, main, err := def.StagedToolNodes()
	require.NoError(t, err)
	assert.Equal(t, []string{"redact", "simplify"}, st)
	assert.Equal(t, []string{"ai-translate", "qa-check"}, main)

	// The source-transform nodes carry the stage; main nodes don't.
	stage := map[string]flow.FlowStage{}
	for _, n := range nodes {
		if n.Type == flow.NodeTool {
			stage[n.Name] = n.Stage
		}
	}
	assert.Equal(t, flow.StageSourceTransform, stage["redact"])
	assert.Equal(t, flow.StageSourceTransform, stage["simplify"])
	assert.Equal(t, flow.StageMain, stage["ai-translate"])
}

func TestStepsToGraph_SourceTransformOnly(t *testing.T) {
	// A flow with only a source-transform stage is valid (degenerate but legal).
	nodes, _, err := flow.StepsToGraph(&flow.StepsSpec{
		SourceTransforms: []flow.FlowStep{{Tool: "redact"}},
	})
	require.NoError(t, err)
	var names []string
	for _, n := range nodes {
		if n.Type == flow.NodeTool {
			names = append(names, n.Name)
		}
	}
	assert.Equal(t, []string{"redact"}, names)
}

func TestFlowDefinition_StageValidation(t *testing.T) {
	// A non-tool node may not carry the source-transform stage.
	bad := &flow.FlowDefinition{
		ID: "f", Name: "f",
		Nodes: []flow.FlowNode{{ID: "r", Type: flow.NodeReader, Stage: flow.StageSourceTransform}},
	}
	require.ErrorContains(t, bad.Validate(), "source-transform")

	// An unknown stage value is rejected.
	bogus := &flow.FlowDefinition{
		ID: "f", Name: "f",
		Nodes: []flow.FlowNode{{ID: "t", Type: flow.NodeTool, Stage: "bogus"}},
	}
	require.ErrorContains(t, bogus.Validate(), "stage")
}

func TestBuiltInSecureTranslate_RedactIsSourceTransform(t *testing.T) {
	for _, def := range flow.BuiltInFlows() {
		if def.ID != "secure-translate" {
			continue
		}
		require.NoError(t, def.Validate())
		st, main, err := def.StagedToolNodes()
		require.NoError(t, err)
		// redact settles the source up front; ai-translate and the late
		// restore (unredact) are main-stage even though unredact is also
		// Transform-capable (capability != phase).
		assert.Equal(t, []string{"redact"}, st)
		assert.Equal(t, []string{"ai-translate", "unredact"}, main)
		return
	}
	t.Fatal("secure-translate built-in flow not found")
}
