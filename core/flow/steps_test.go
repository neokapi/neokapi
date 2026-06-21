package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepsToGraph_Linear(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{Tool: "translate", Config: map[string]any{"provider": "anthropic"}},
			{Tool: "qa"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	// Tool-only graph: 2 tool nodes, no reader/writer (AD-026).
	require.Len(t, nodes, 2)
	assert.Equal(t, NodeTool, nodes[0].Type)
	assert.Equal(t, "translate", nodes[0].Name)
	assert.Equal(t, map[string]any{"provider": "anthropic"}, nodes[0].Config)
	assert.Equal(t, NodeTool, nodes[1].Type)
	assert.Equal(t, "qa", nodes[1].Name)

	// One edge: translate -> qa. The first tool has no incoming edge.
	require.Len(t, edges, 1)
	assert.Equal(t, nodes[0].ID, edges[0].Source)
	assert.Equal(t, nodes[1].ID, edges[0].Target)
}

func TestStepsToGraph_Parallel(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{
				Parallel: []FlowStep{
					{Tool: "translate"},
					{Tool: "word-count"},
				},
			},
			{Tool: "qa"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	// 2 parallel tools + qa = 3 nodes (no reader/writer).
	require.Len(t, nodes, 3)

	var parallelIDs []string
	for _, n := range nodes {
		if n.Name == "translate" || n.Name == "word-count" {
			parallelIDs = append(parallelIDs, n.ID)
		}
	}
	assert.Len(t, parallelIDs, 2)

	// The parallel tools are graph roots (no incoming edges).
	for _, id := range parallelIDs {
		assert.Empty(t, filterEdges(edges, "", id), "parallel root %s should have no incoming edge", id)
	}

	// Both branches fan in to qa.
	var qaNode FlowNode
	for _, n := range nodes {
		if n.Name == "qa" {
			qaNode = n
			break
		}
	}
	assert.Len(t, filterEdges(edges, "", qaNode.ID), 2)
}

func TestStepsToGraph_SingleStep(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{Tool: "pseudo-translate"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	require.Len(t, nodes, 1) // the single tool only
	assert.Equal(t, NodeTool, nodes[0].Type)
	assert.Equal(t, "pseudo-translate", nodes[0].Name)
	assert.Empty(t, edges)
}

func TestStepsToGraph_SourceTransformsRejected(t *testing.T) {
	// The structural source-transform stage is removed (AD-006): a flow that
	// still uses it gets an actionable error instead of a silently dropped
	// stage — transformers are ordinary ordered steps.
	spec := &StepsSpec{
		SourceTransforms: []FlowStep{{Tool: "redact"}},
		Steps:            []FlowStep{{Tool: "translate"}},
	}

	_, _, err := StepsToGraph(spec)
	require.ErrorContains(t, err, "source_transforms")
	require.ErrorContains(t, err, "ordered steps")
}

func TestStepsToGraph_TransformerAsOrderedStep(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{{Tool: "redact"}, {Tool: "translate"}},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	require.Len(t, nodes, 2)
	assert.Equal(t, "redact", nodes[0].Name)
	assert.Equal(t, "translate", nodes[1].Name)
	// redact -> translate; redact is the root.
	require.Len(t, edges, 1)
	assert.Equal(t, nodes[0].ID, edges[0].Source)
	assert.Equal(t, nodes[1].ID, edges[0].Target)
}

func TestStepsToGraph_Empty(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{}}
	_, _, err := StepsToGraph(spec)
	require.Error(t, err)
}

func TestStepsToGraph_CustomLabel(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{Tool: "script", Label: "Filter long segments"},
		},
	}

	nodes, _, err := StepsToGraph(spec)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "Filter long segments", nodes[0].Label)
}

func TestStepsToGraph_ValidTopology(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{Tool: "recycle"},
			{
				Parallel: []FlowStep{
					{Tool: "translate"},
					{Tool: "word-count"},
				},
			},
			{Tool: "qa"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	def := &FlowDefinition{
		ID:    "test",
		Name:  "test",
		Nodes: nodes,
		Edges: edges,
	}
	require.NoError(t, def.Validate())

	order, err := def.TopologicalOrder()
	require.NoError(t, err)
	assert.Len(t, order, len(nodes))
}

func TestParseFlowYAML_StepsFormat(t *testing.T) {
	yaml := `
steps:
  - tool: pseudo-translate
    config:
      expansion: 30
  - tool: qa
`
	def, err := parseFlowYAML([]byte(yaml))
	require.NoError(t, err)
	assert.Len(t, def.Nodes, 2) // 2 tool nodes (no reader/writer)
	assert.Len(t, def.Edges, 1)

	var pseudoNode *FlowNode
	for i := range def.Nodes {
		if def.Nodes[i].Name == "pseudo-translate" {
			pseudoNode = &def.Nodes[i]
			break
		}
	}
	require.NotNil(t, pseudoNode)
	assert.Equal(t, 30, pseudoNode.Config["expansion"])
}

// TestParseFlowYAML_GraphFormatStillWorks verifies legacy graphs that still
// carry reader/writer nodes load (Validate tolerates them; execution ignores
// non-tool nodes).
func TestParseFlowYAML_GraphFormatStillWorks(t *testing.T) {
	yaml := `
id: test-flow
name: Test Flow
nodes:
  - id: reader
    type: reader
    name: auto
    position: {x: 0, y: 100}
  - id: writer
    type: writer
    name: auto
    position: {x: 250, y: 100}
edges:
  - id: e1
    source: reader
    target: writer
`
	def, err := parseFlowYAML([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "test-flow", def.ID)
	assert.Len(t, def.Nodes, 2)
	assert.Len(t, def.Edges, 1)
}

func filterEdges(edges []FlowEdge, source, target string) []FlowEdge {
	var result []FlowEdge
	for _, e := range edges {
		if (source == "" || e.Source == source) && (target == "" || e.Target == target) {
			result = append(result, e)
		}
	}
	return result
}
