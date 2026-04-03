package graph

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeJSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	node := Node{
		ID:         "n1",
		Label:      "Concept",
		Properties: map[string]string{"domain": "automotive"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	data, err := json.Marshal(node)
	require.NoError(t, err)

	var got Node
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, node.ID, got.ID)
	assert.Equal(t, node.Label, got.Label)
	assert.Equal(t, node.Properties, got.Properties)
	assert.True(t, node.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, node.UpdatedAt.Equal(got.UpdatedAt))
}

func TestEdgeJSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	edge := Edge{
		ID:         "e1",
		Source:     "n1",
		Target:     "n2",
		Label:      LabelBroader,
		Properties: map[string]string{"weight": "0.9"},
		Validity: &Validity{
			ValidFrom: &from,
			Tags:      map[string]string{"market": "us"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(edge)
	require.NoError(t, err)

	var got Edge
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, edge.ID, got.ID)
	assert.Equal(t, edge.Source, got.Source)
	assert.Equal(t, edge.Target, got.Target)
	assert.Equal(t, edge.Label, got.Label)
	assert.Equal(t, edge.Properties, got.Properties)
	require.NotNil(t, got.Validity)
	assert.True(t, edge.Validity.ValidFrom.Equal(*got.Validity.ValidFrom))
	assert.Equal(t, edge.Validity.Tags, got.Validity.Tags)
}

func TestEdgeJSONOmitsNilValidity(t *testing.T) {
	edge := Edge{ID: "e1", Source: "n1", Target: "n2", Label: LabelRelated}
	data, err := json.Marshal(edge)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "validity")
}

func TestPathLen(t *testing.T) {
	tests := []struct {
		name string
		path Path
		want int
	}{
		{"empty", Path{}, 0},
		{"one edge", Path{Edges: []Edge{{}}}, 1},
		{"three edges", Path{Edges: []Edge{{}, {}, {}}}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.path.Len())
		})
	}
}

func TestPathEmpty(t *testing.T) {
	tests := []struct {
		name string
		path Path
		want bool
	}{
		{"no nodes", Path{}, true},
		{"has nodes", Path{Nodes: []Node{{ID: "n1"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.path.Empty())
		})
	}
}

func TestInverseLabel(t *testing.T) {
	tests := []struct {
		label   string
		inverse string
	}{
		{LabelBroader, LabelNarrower},
		{LabelNarrower, LabelBroader},
		{LabelPartOf, LabelHasPart},
		{LabelHasPart, LabelPartOf},
		{LabelRelated, ""},
		{LabelExactMatch, ""},
		{LabelForbidden, ""},
		{"UNKNOWN", ""},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			assert.Equal(t, tt.inverse, InverseLabel(tt.label))
		})
	}
}

func TestLabelsNonEmpty(t *testing.T) {
	labels := []string{
		LabelBroader, LabelNarrower, LabelRelated,
		LabelPartOf, LabelHasPart,
		LabelHasTerm, LabelUseInstead, LabelReplacedBy,
		LabelExactMatch, LabelCloseMatch,
		LabelForbidden, LabelPreferred, LabelCompetitor,
	}
	for _, l := range labels {
		assert.NotEmpty(t, l)
	}
}
