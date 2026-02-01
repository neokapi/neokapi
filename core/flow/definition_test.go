package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
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
					{ID: "a", Type: "tool", Name: "t1"},
					{ID: "a", Type: "tool", Name: "t2"},
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
					{ID: "a", Type: "tool", Name: "t1"},
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
					{ID: "a", Type: "tool", Name: "t1"},
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
					{ID: "r", Type: "reader", Name: "html"},
					{ID: "t", Type: "tool", Name: "translate"},
					{ID: "w", Type: "writer", Name: "html"},
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
				assert.NoError(t, err)
			}
		})
	}
}

func TestTopologicalOrder(t *testing.T) {
	def := FlowDefinition{
		ID:   "test",
		Name: "test",
		Nodes: []FlowNode{
			{ID: "reader", Type: "reader", Name: "html"},
			{ID: "tool1", Type: "tool", Name: "translate"},
			{ID: "tool2", Type: "tool", Name: "qa"},
			{ID: "writer", Type: "writer", Name: "html"},
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
			{ID: "a", Type: "tool", Name: "t1"},
			{ID: "b", Type: "tool", Name: "t2"},
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
			{ID: "reader", Type: "reader", Name: "html"},
			{ID: "tool1", Type: "tool", Name: "ai-translate"},
			{ID: "tool2", Type: "tool", Name: "ai-qa"},
			{ID: "writer", Type: "writer", Name: "html"},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "reader", Target: "tool1"},
			{ID: "e2", Source: "tool1", Target: "tool2"},
			{ID: "e3", Source: "tool2", Target: "writer"},
		},
	}

	names, err := def.ToolNodeNames()
	require.NoError(t, err)
	assert.Equal(t, []string{"ai-translate", "ai-qa"}, names)
}

func TestBuiltInFlows(t *testing.T) {
	flows := BuiltInFlows()
	require.Len(t, flows, 5)

	ids := make(map[string]bool)
	for _, f := range flows {
		assert.NotEmpty(t, f.ID)
		assert.NotEmpty(t, f.Name)
		assert.Equal(t, "built-in", f.Source)
		assert.NoError(t, f.Validate())
		ids[f.ID] = true
	}
	assert.True(t, ids["ai-translate"])
	assert.True(t, ids["ai-translate-qa"])
	assert.True(t, ids["pseudo-translate"])
	assert.True(t, ids["qa-check"])
	assert.True(t, ids["tm-leverage"])
}

func TestFlowDefinitionJSON(t *testing.T) {
	def := FlowDefinition{
		ID:          "test-flow",
		Name:        "Test Flow",
		Description: "A test flow",
		Source:      "user",
		Nodes: []FlowNode{
			{ID: "r", Type: "reader", Name: "html", Position: NodePosition{X: 0, Y: 100}},
			{ID: "t", Type: "tool", Name: "translate", Label: "Translate", Position: NodePosition{X: 250, Y: 100}},
			{ID: "w", Type: "writer", Name: "html", Position: NodePosition{X: 500, Y: 100}},
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

func TestFlowStore(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	// List empty store.
	defs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, defs)

	// Save a flow.
	def := &FlowDefinition{
		ID:   "my-flow",
		Name: "My Flow",
		Nodes: []FlowNode{
			{ID: "r", Type: "reader", Name: "html"},
			{ID: "w", Type: "writer", Name: "html"},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "r", Target: "w"},
		},
	}
	require.NoError(t, store.Save(def))
	assert.NotEmpty(t, def.CreatedAt)
	assert.NotEmpty(t, def.ModifiedAt)

	// List should have one entry.
	defs, err = store.List()
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "my-flow", defs[0].ID)
	assert.Equal(t, "user", defs[0].Source)

	// Get by ID.
	got, err := store.Get("my-flow")
	require.NoError(t, err)
	assert.Equal(t, "My Flow", got.Name)

	// Get non-existent.
	_, err = store.Get("nope")
	assert.Error(t, err)

	// Delete.
	require.NoError(t, store.Delete("my-flow"))
	defs, err = store.List()
	require.NoError(t, err)
	assert.Empty(t, defs)

	// Delete non-existent.
	assert.Error(t, store.Delete("nope"))
}

func TestFlowStoreNonExistentDir(t *testing.T) {
	store := NewFlowStore(filepath.Join(t.TempDir(), "nested", "flows"))

	// List on non-existent dir returns empty, not error.
	defs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, defs)

	// Save creates the directory.
	def := &FlowDefinition{
		ID:   "test",
		Name: "Test",
		Nodes: []FlowNode{
			{ID: "r", Type: "reader", Name: "html"},
		},
	}
	require.NoError(t, store.Save(def))

	// Verify file exists.
	_, err = os.Stat(filepath.Join(store.dir, "test.json"))
	assert.NoError(t, err)
}

func TestFlowStoreSaveValidation(t *testing.T) {
	store := NewFlowStore(t.TempDir())
	def := &FlowDefinition{Name: "no id"}
	assert.Error(t, store.Save(def))
}
