package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowStore_YAMLEnveloped(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	yamlContent := `apiVersion: v1
kind: FlowDefinition
metadata:
  name: pseudo-translate
  description: "Generate pseudo-translations"
spec:
  id: pseudo
  name: Pseudo Translate
  nodes:
    - id: reader
      type: reader
      name: auto
    - id: pseudo
      type: tool
      name: pseudo-translate
    - id: writer
      type: writer
      name: auto
  edges:
    - id: e1
      source: reader
      target: pseudo
    - id: e2
      source: pseudo
      target: writer
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pseudo.yaml"), []byte(yamlContent), 0644))

	// List should find the YAML file
	defs, err := store.List()
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "pseudo", defs[0].ID)
	assert.Equal(t, "Pseudo Translate", defs[0].Name)
	assert.Equal(t, "user", defs[0].Source)
	assert.Len(t, defs[0].Nodes, 3)
	assert.Len(t, defs[0].Edges, 2)

	// Get by ID should find it
	def, err := store.Get("pseudo")
	require.NoError(t, err)
	assert.Equal(t, "pseudo", def.ID)
	assert.Equal(t, "Pseudo Translate", def.Name)
}

func TestFlowStore_BareYAML(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	yamlContent := `id: my-flow
name: My Flow
nodes:
  - id: r
    type: reader
    name: auto
  - id: w
    type: writer
    name: auto
edges:
  - id: e1
    source: r
    target: w
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my-flow.yml"), []byte(yamlContent), 0644))

	def, err := store.Get("my-flow")
	require.NoError(t, err)
	assert.Equal(t, "my-flow", def.ID)
	assert.Equal(t, "My Flow", def.Name)
}

func TestFlowStore_EnvelopedMetadataFallback(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	// Flow with name/description in metadata but not in spec
	yamlContent := `apiVersion: v1
kind: FlowDefinition
metadata:
  name: Meta Flow
  description: "From metadata"
spec:
  id: meta-flow
  nodes:
    - id: r
      type: reader
      name: auto
  edges: []
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "meta-flow.yaml"), []byte(yamlContent), 0644))

	def, err := store.Get("meta-flow")
	require.NoError(t, err)
	assert.Equal(t, "meta-flow", def.ID)
	assert.Equal(t, "Meta Flow", def.Name)
	assert.Equal(t, "From metadata", def.Description)
}

func TestFlowStore_WrongKindRejected(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	yamlContent := `apiVersion: v1
kind: ProjectConfig
metadata:
  name: wrong
spec: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wrong.yaml"), []byte(yamlContent), 0644))

	// List should skip the file (parse error)
	defs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, defs)
}

func TestFlowStore_MixedFormats(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	// JSON flow
	jsonDef := FlowDefinition{
		ID:   "json-flow",
		Name: "JSON Flow",
		Nodes: []FlowNode{
			{ID: "r", Type: NodeReader, Name: "auto"},
			{ID: "w", Type: NodeWriter, Name: "auto"},
		},
		Edges: []FlowEdge{
			{ID: "e1", Source: "r", Target: "w"},
		},
	}
	jsonData, _ := json.MarshalIndent(jsonDef, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "json-flow.json"), jsonData, 0644))

	// YAML flow
	yamlContent := `id: yaml-flow
name: YAML Flow
nodes:
  - id: r
    type: reader
    name: auto
  - id: w
    type: writer
    name: auto
edges:
  - id: e1
    source: r
    target: w
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "yaml-flow.yaml"), []byte(yamlContent), 0644))

	defs, err := store.List()
	require.NoError(t, err)
	assert.Len(t, defs, 2)

	ids := map[string]bool{}
	for _, d := range defs {
		ids[d.ID] = true
	}
	assert.True(t, ids["json-flow"])
	assert.True(t, ids["yaml-flow"])
}

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

func TestBuiltInFlows(t *testing.T) {
	flows := BuiltInFlows()
	require.Len(t, flows, 10)

	ids := make(map[string]bool)
	for _, f := range flows {
		assert.NotEmpty(t, f.ID)
		assert.NotEmpty(t, f.Name)
		assert.Equal(t, registry.SourceBuiltIn, f.Source)
		require.NoError(t, f.Validate())
		ids[f.ID] = true
	}
	assert.True(t, ids["translate"])
	assert.True(t, ids["translate-qa"])
	assert.True(t, ids["pseudo-translate"])
	assert.True(t, ids["qa"])
	assert.True(t, ids["tm-leverage"])
	assert.True(t, ids["secure-translate"])
	assert.True(t, ids["redact-pii"])
	assert.True(t, ids["audio-to-subtitles"])
	assert.True(t, ids["video-to-subtitles"])
	assert.True(t, ids["image-ocr-translate"])
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
			{ID: "r", Type: NodeReader, Name: "html"},
			{ID: "w", Type: NodeWriter, Name: "html"},
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
	require.Error(t, err)

	// Delete.
	require.NoError(t, store.Delete("my-flow"))
	defs, err = store.List()
	require.NoError(t, err)
	assert.Empty(t, defs)

	// Delete non-existent.
	require.Error(t, store.Delete("nope"))
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
			{ID: "r", Type: NodeReader, Name: "html"},
		},
	}
	require.NoError(t, store.Save(def))

	// Verify file exists.
	_, err = os.Stat(filepath.Join(store.dir, "test.json"))
	require.NoError(t, err)
}

func TestFlowStoreSaveValidation(t *testing.T) {
	store := NewFlowStore(t.TempDir())
	def := &FlowDefinition{Name: "no id"}
	require.Error(t, store.Save(def))
}
