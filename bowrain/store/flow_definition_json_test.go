package store

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StoredFlow embeds flow.FlowDefinition, whose MarshalJSON Go promotes to it;
// these pin that the revision, the optimistic-concurrency token, is written
// beside the definition's fields and read back.
func TestStoredFlow_JSON(t *testing.T) {
	node := flow.FlowNode{ID: "a", Type: flow.NodeTool, Name: "qa"}
	twoSteps := flow.FlowDefinition{
		ID: "f1", Name: "Two", Source: "project",
		Nodes: []flow.FlowNode{node, {ID: "b", Type: flow.NodeTool, Name: "recycle"}},
		Edges: []flow.FlowEdge{{ID: "e1", Source: "a", Target: "b"}},
	}
	oneStep := flow.FlowDefinition{ID: "f2", Name: "One", Source: "project", Nodes: []flow.FlowNode{node}}
	const nodeJSON = `{"id":"a","type":"tool","name":"qa","position":{"x":0,"y":0}}`

	for _, tc := range []struct {
		name string
		in   StoredFlow
		want string
	}{
		{
			name: "edges and a revision",
			in:   StoredFlow{FlowDefinition: twoSteps, Revision: "r1"},
			want: `{"id":"f1","name":"Two","nodes":[` + nodeJSON +
				`,{"id":"b","type":"tool","name":"recycle","position":{"x":0,"y":0}}],` +
				`"edges":[{"id":"e1","source":"a","target":"b"}],"source":"project","revision":"r1"}`,
		},
		{
			name: "edges, no revision",
			in:   StoredFlow{FlowDefinition: twoSteps},
			want: `{"id":"f1","name":"Two","nodes":[` + nodeJSON +
				`,{"id":"b","type":"tool","name":"recycle","position":{"x":0,"y":0}}],` +
				`"edges":[{"id":"e1","source":"a","target":"b"}],"source":"project"}`,
		},
		{
			name: "no edges, a revision",
			in:   StoredFlow{FlowDefinition: oneStep, Revision: "r2"},
			want: `{"id":"f2","name":"One","nodes":[` + nodeJSON + `],"edges":[],"source":"project","revision":"r2"}`,
		},
		{
			name: "no edges, no revision",
			in:   StoredFlow{FlowDefinition: oneStep},
			want: `{"id":"f2","name":"One","nodes":[` + nodeJSON + `],"edges":[],"source":"project"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(data))
			// A pointer marshals the same.
			ptr, err := json.Marshal(&tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(ptr))

			var back StoredFlow
			require.NoError(t, json.Unmarshal(data, &back))
			want := tc.in
			if want.Edges == nil {
				want.Edges = []flow.FlowEdge{}
			}
			assert.Equal(t, want, back)
		})
	}
}
