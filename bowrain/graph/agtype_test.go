package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVertex(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantID  string
		wantLbl string
		wantErr bool
	}{
		{
			name:    "basic vertex",
			raw:     `{"id": 1125899906842625, "label": "Concept", "properties": {"id": "abc-123", "name": "test"}}::vertex`,
			wantID:  "abc-123",
			wantLbl: "Concept",
		},
		{
			name:    "vertex with no id property falls back to AGE id",
			raw:     `{"id": 42, "label": "Term", "properties": {"name": "hello"}}::vertex`,
			wantID:  "42",
			wantLbl: "Term",
		},
		{
			name:    "vertex with empty properties",
			raw:     `{"id": 1, "label": "Node", "properties": {}}::vertex`,
			wantID:  "1",
			wantLbl: "Node",
		},
		{
			name:    "missing suffix",
			raw:     `{"id": 1, "label": "Node", "properties": {}}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			raw:     `{broken}::vertex`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := ParseVertex(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, node.ID)
			assert.Equal(t, tt.wantLbl, node.Label)
		})
	}
}

func TestParseEdge(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantID     string
		wantLabel  string
		wantSource string
		wantTarget string
		wantErr    bool
	}{
		{
			name:       "basic edge",
			raw:        `{"id": 99, "label": "BROADER", "end_id": 456, "start_id": 789, "properties": {"id": "edge-1", "source": "node-a", "target": "node-b"}}::edge`,
			wantID:     "edge-1",
			wantLabel:  "BROADER",
			wantSource: "node-a",
			wantTarget: "node-b",
		},
		{
			name:       "edge with AGE id fallback",
			raw:        `{"id": 55, "label": "RELATED", "end_id": 20, "start_id": 10, "properties": {}}::edge`,
			wantID:     "55",
			wantLabel:  "RELATED",
			wantSource: "10",
			wantTarget: "20",
		},
		{
			name:    "missing suffix",
			raw:     `{"id": 1, "label": "X", "end_id": 2, "start_id": 3, "properties": {}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge, err := ParseEdge(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, edge.ID)
			assert.Equal(t, tt.wantLabel, edge.Label)
			assert.Equal(t, tt.wantSource, edge.Source)
			assert.Equal(t, tt.wantTarget, edge.Target)
		})
	}
}

func TestParseEdgeWithValidity(t *testing.T) {
	raw := `{"id": 1, "label": "REL", "end_id": 2, "start_id": 3, "properties": {"id": "e1", "source": "n1", "target": "n2", "valid_from": "2024-01-01T00:00:00Z", "valid_to": "2025-01-01T00:00:00Z", "tags": "{\"env\":\"prod\"}"}}::edge`

	edge, err := ParseEdge(raw)
	require.NoError(t, err)
	require.NotNil(t, edge.Validity)
	require.NotNil(t, edge.Validity.ValidFrom)
	require.NotNil(t, edge.Validity.ValidTo)
	assert.Equal(t, 2024, edge.Validity.ValidFrom.Year())
	assert.Equal(t, 2025, edge.Validity.ValidTo.Year())
	assert.Equal(t, "prod", edge.Validity.Tags["env"])
}

func TestParsePath(t *testing.T) {
	raw := `[{"id": 1, "label": "A", "properties": {"id": "n1"}}::vertex, {"id": 10, "label": "REL", "end_id": 2, "start_id": 1, "properties": {"id": "e1", "source": "n1", "target": "n2"}}::edge, {"id": 2, "label": "B", "properties": {"id": "n2"}}::vertex]::path`

	path, err := ParsePath(raw)
	require.NoError(t, err)
	assert.Len(t, path.Nodes, 2)
	assert.Len(t, path.Edges, 1)
	assert.Equal(t, "n1", path.Nodes[0].ID)
	assert.Equal(t, "n2", path.Nodes[1].ID)
	assert.Equal(t, "e1", path.Edges[0].ID)
}

func TestParsePathEmpty(t *testing.T) {
	_, err := ParsePath("not-a-path")
	require.Error(t, err)
}

func TestToStringMap(t *testing.T) {
	m := map[string]any{
		"str":   "hello",
		"num":   float64(42),
		"float": float64(3.14),
		"bool":  true,
		"nil":   nil,
	}

	result := toStringMap(m)
	assert.Equal(t, "hello", result["str"])
	assert.Equal(t, "42", result["num"])
	assert.Equal(t, "3.14", result["float"])
	assert.Equal(t, "true", result["bool"])
	_, hasNil := result["nil"]
	assert.False(t, hasNil)
}

func TestStripSuffix(t *testing.T) {
	body, err := stripSuffix(`{"id": 1}::vertex`, "::vertex")
	require.NoError(t, err)
	assert.Equal(t, `{"id": 1}`, body)

	_, err = stripSuffix(`{"id": 1}::edge`, "::vertex")
	require.Error(t, err)
}

func TestSplitPathElements(t *testing.T) {
	elements, err := splitPathElements(`[{a: 1}::vertex, {b: 2}::edge, {c: 3}::vertex]`)
	require.NoError(t, err)
	assert.Len(t, elements, 3)

	_, err = splitPathElements("not-brackets")
	require.Error(t, err)
}
