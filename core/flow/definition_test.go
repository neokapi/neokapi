package flow

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowDefinitionValidate(t *testing.T) {
	tests := []struct {
		name    string
		def     FlowDefinition
		wantErr string
	}{
		{
			name:    "missing id",
			def:     FlowDefinition{Name: "test"},
			wantErr: "id is required",
		},
		{
			name:    "missing name",
			def:     FlowDefinition{ID: "test"},
			wantErr: "name is required",
		},
		{
			name: "duplicate node id",
			def: FlowDefinition{
				ID:   "test",
				Name: "test",
				Nodes: []FlowNode{
					{ID: "a", Type: NodeTool, Name: "t1"},
					{ID: "a", Type: NodeTool, Name: "t2"},
				},
			},
			wantErr: "duplicate node id",
		},
		{
			name: "invalid node type",
			def: FlowDefinition{
				ID:   "test",
				Name: "test",
				Nodes: []FlowNode{
					{ID: "a", Type: "unknown", Name: "t1"},
				},
			},
			wantErr: "invalid node type",
		},
		{
			name: "edge source not found",
			def: FlowDefinition{
				ID:   "test",
				Name: "test",
				Nodes: []FlowNode{
					{ID: "a", Type: NodeTool, Name: "t1"},
				},
				Edges: []FlowEdge{
					{ID: "e1", Source: "missing", Target: "a"},
				},
			},
			wantErr: "edge source",
		},
		{
			name: "edge target not found",
			def: FlowDefinition{
				ID:   "test",
				Name: "test",
				Nodes: []FlowNode{
					{ID: "a", Type: NodeTool, Name: "t1"},
				},
				Edges: []FlowEdge{
					{ID: "e1", Source: "a", Target: "missing"},
				},
			},
			wantErr: "edge target",
		},
		{
			name: "valid flow",
			def: FlowDefinition{
				ID:   "test",
				Name: "test",
				Nodes: []FlowNode{
					{ID: "r", Type: NodeReader, Name: "html"},
					{ID: "t", Type: NodeTool, Name: "translate"},
					{ID: "w", Type: NodeWriter, Name: "html"},
				},
				Edges: []FlowEdge{
					{ID: "e1", Source: "r", Target: "t"},
					{ID: "e2", Source: "t", Target: "w"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTopologicalOrder(t *testing.T) {
	def := FlowDefinition{
		ID:   "test",
		Name: "test",
		Nodes: []FlowNode{
			{ID: "reader", Type: NodeReader, Name: "html"},
			{ID: "tool1", Type: NodeTool, Name: "translate"},
			{ID: "tool2", Type: NodeTool, Name: "qa"},
			{ID: "writer", Type: NodeWriter, Name: "html"},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "reader", Target: "tool1"},
			{ID: "e2", Source: "tool1", Target: "tool2"},
			{ID: "e3", Source: "tool2", Target: "writer"},
		},
	}

	order, err := def.TopologicalOrder()
	require.NoError(t, err)
	assert.Equal(t, []string{"reader", "tool1", "tool2", "writer"}, order)
}

func TestTopologicalOrderCycle(t *testing.T) {
	def := FlowDefinition{
		ID:   "test",
		Name: "test",
		Nodes: []FlowNode{
			{ID: "a", Type: NodeTool, Name: "t1"},
			{ID: "b", Type: NodeTool, Name: "t2"},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "a", Target: "b"},
			{ID: "e2", Source: "b", Target: "a"},
		},
	}

	_, err := def.TopologicalOrder()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestToolNodeNames(t *testing.T) {
	def := FlowDefinition{
		ID:   "test",
		Name: "test",
		Nodes: []FlowNode{
			{ID: "reader", Type: NodeReader, Name: "html"},
			{ID: "tool1", Type: NodeTool, Name: "translate"},
			{ID: "tool2", Type: NodeTool, Name: "qa"},
			{ID: "writer", Type: NodeWriter, Name: "html"},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "reader", Target: "tool1"},
			{ID: "e2", Source: "tool1", Target: "tool2"},
			{ID: "e3", Source: "tool2", Target: "writer"},
		},
	}

	names, err := def.ToolNodeNames()
	require.NoError(t, err)
	assert.Equal(t, []string{"translate", "qa"}, names)
}

func TestFlowDefinitionJSON(t *testing.T) {
	def := FlowDefinition{
		ID:          "test-flow",
		Name:        "Test Flow",
		Description: "A test flow",
		Source:      "user",
		Nodes: []FlowNode{
			{ID: "r", Type: NodeReader, Name: "html", Position: NodePosition{X: 0, Y: 100}},
			{ID: "t", Type: NodeTool, Name: "translate", Label: "Translate", Position: NodePosition{X: 250, Y: 100}},
			{ID: "w", Type: NodeWriter, Name: "html", Position: NodePosition{X: 500, Y: 100}},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "r", Target: "t"},
			{ID: "e2", Source: "t", Target: "w"},
		},
	}

	data, err := json.Marshal(def)
	require.NoError(t, err)

	var parsed FlowDefinition
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, def.ID, parsed.ID)
	assert.Equal(t, def.Name, parsed.Name)
	assert.Len(t, parsed.Nodes, 3)
	assert.Len(t, parsed.Edges, 2)
	assert.Equal(t, 250.0, parsed.Nodes[1].Position.X)
}
