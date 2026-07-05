package flowdef

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
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
	jsonDef := flow.FlowDefinition{
		ID:   "json-flow",
		Name: "JSON Flow",
		Nodes: []flow.FlowNode{
			{ID: "r", Type: flow.NodeReader, Name: "auto"},
			{ID: "w", Type: flow.NodeWriter, Name: "auto"},
		},
		Edges: []flow.FlowEdge{
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

func TestFlowStore(t *testing.T) {
	dir := t.TempDir()
	store := NewFlowStore(dir)

	// List empty store.
	defs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, defs)

	// Save a flow.
	def := &flow.FlowDefinition{
		ID:   "my-flow",
		Name: "My Flow",
		Nodes: []flow.FlowNode{
			{ID: "r", Type: flow.NodeReader, Name: "html"},
			{ID: "w", Type: flow.NodeWriter, Name: "html"},
		},
		Edges: []flow.FlowEdge{
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
	def := &flow.FlowDefinition{
		ID:   "test",
		Name: "Test",
		Nodes: []flow.FlowNode{
			{ID: "r", Type: flow.NodeReader, Name: "html"},
		},
	}
	require.NoError(t, store.Save(def))

	// Verify file exists.
	_, err = os.Stat(filepath.Join(store.dir, "test.json"))
	require.NoError(t, err)
}

func TestFlowStoreSaveValidation(t *testing.T) {
	store := NewFlowStore(t.TempDir())
	def := &flow.FlowDefinition{Name: "no id"}
	require.Error(t, store.Save(def))
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

	var pseudoNode *flow.FlowNode
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
