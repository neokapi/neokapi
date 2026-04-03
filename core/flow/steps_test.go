package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepsToGraph_Linear(t *testing.T) {
	spec := &StepsSpec{
		Input:  "auto",
		Output: "auto",
		Steps: []FlowStep{
			{Tool: "ai-translate", Config: map[string]any{"provider": "anthropic"}},
			{Tool: "qa-check"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	// reader + 2 tools + writer = 4 nodes
	assert.Len(t, nodes, 4)
	assert.Equal(t, "reader", nodes[0].Type)
	assert.Equal(t, "auto", nodes[0].Name)
	assert.Equal(t, "tool", nodes[1].Type)
	assert.Equal(t, "ai-translate", nodes[1].Name)
	assert.Equal(t, map[string]any{"provider": "anthropic"}, nodes[1].Config)
	assert.Equal(t, "tool", nodes[2].Type)
	assert.Equal(t, "qa-check", nodes[2].Name)
	assert.Equal(t, "writer", nodes[3].Type)

	// reader->translate, translate->qa, qa->writer = 3 edges
	assert.Len(t, edges, 3)
	assert.Equal(t, "reader", edges[0].Source)
	assert.Equal(t, nodes[1].ID, edges[0].Target)
	assert.Equal(t, nodes[1].ID, edges[1].Source)
	assert.Equal(t, nodes[2].ID, edges[1].Target)
	assert.Equal(t, nodes[2].ID, edges[2].Source)
	assert.Equal(t, "writer", edges[2].Target)
}

func TestStepsToGraph_Parallel(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{
				Parallel: []FlowStep{
					{Tool: "ai-translate"},
					{Tool: "word-count"},
				},
			},
			{Tool: "qa-check"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	// reader + 2 parallel tools + qa + writer = 5 nodes
	assert.Len(t, nodes, 5)

	// Find parallel tool nodes
	var parallelIDs []string
	for _, n := range nodes {
		if n.Name == "ai-translate" || n.Name == "word-count" {
			parallelIDs = append(parallelIDs, n.ID)
		}
	}
	assert.Len(t, parallelIDs, 2)

	// reader fans out to both parallel branches
	readerEdges := filterEdges(edges, "reader", "")
	assert.Len(t, readerEdges, 2)

	// Both branches connect to qa-check
	var qaNode FlowNode
	for _, n := range nodes {
		if n.Name == "qa-check" {
			qaNode = n
			break
		}
	}
	qaEdges := filterEdges(edges, "", qaNode.ID)
	assert.Len(t, qaEdges, 2)
}

func TestStepsToGraph_SingleStep(t *testing.T) {
	spec := &StepsSpec{
		Input: "json",
		Steps: []FlowStep{
			{Tool: "pseudo-translate"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	assert.Len(t, nodes, 3) // reader + tool + writer
	assert.Len(t, edges, 2)
	assert.Equal(t, "json", nodes[0].Name) // reader uses specified format
}

func TestStepsToGraph_Empty(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{}}
	_, _, err := StepsToGraph(spec)
	assert.Error(t, err)
}

func TestStepsToGraph_CustomLabel(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{Tool: "script", Label: "Filter long segments"},
		},
	}

	nodes, _, err := StepsToGraph(spec)
	require.NoError(t, err)
	assert.Equal(t, "Filter long segments", nodes[1].Label)
}

func TestStepsToGraph_DefaultFormats(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{{Tool: "pseudo-translate"}},
	}

	nodes, _, err := StepsToGraph(spec)
	require.NoError(t, err)
	assert.Equal(t, "auto", nodes[0].Name)            // reader defaults to auto
	assert.Equal(t, "auto", nodes[len(nodes)-1].Name) // writer defaults to auto
}

func TestStepsToGraph_ValidTopology(t *testing.T) {
	spec := &StepsSpec{
		Steps: []FlowStep{
			{Tool: "tm-leverage"},
			{
				Parallel: []FlowStep{
					{Tool: "ai-translate"},
					{Tool: "word-count"},
				},
			},
			{Tool: "qa-check"},
		},
	}

	nodes, edges, err := StepsToGraph(spec)
	require.NoError(t, err)

	// Validate it produces a valid FlowDefinition
	def := &FlowDefinition{
		ID:    "test",
		Name:  "test",
		Nodes: nodes,
		Edges: edges,
	}
	assert.NoError(t, def.Validate())

	// Verify topological sort works (no cycles)
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
  - tool: qa-check
`
	def, err := parseFlowYAML([]byte(yaml))
	require.NoError(t, err)
	assert.Len(t, def.Nodes, 4) // reader + 2 tools + writer
	assert.Len(t, def.Edges, 3)

	// Verify tool configs preserved
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
