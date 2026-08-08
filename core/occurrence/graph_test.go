package occurrence

import (
	"context"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deltaEdgeReader indexes a built GraphDelta's edges and answers EdgesOf, so the
// traversal can be exercised over the same edges BuildGraph produced without a
// database backend.
type deltaEdgeReader struct{ edges []graph.Edge }

func newDeltaEdgeReader(d *GraphDelta) *deltaEdgeReader { return &deltaEdgeReader{edges: d.Edges} }

func (r *deltaEdgeReader) EdgesOf(_ context.Context, nodeID string, dir graph.Direction, labels ...string) ([]*graph.Edge, error) {
	want := map[string]bool{}
	for _, l := range labels {
		want[l] = true
	}
	var out []*graph.Edge
	for i := range r.edges {
		e := r.edges[i]
		if len(want) > 0 && !want[e.Label] {
			continue
		}
		match := false
		switch dir {
		case graph.Outgoing:
			match = e.Source == nodeID
		case graph.Incoming:
			match = e.Target == nodeID
		case graph.Both:
			match = e.Source == nodeID || e.Target == nodeID
		}
		if match {
			ec := e
			out = append(out, &ec)
		}
	}
	return out, nil
}

func TestBuildGraphShape(t *testing.T) {
	for name, src := range backends(t) {
		t.Run(name, func(t *testing.T) {
			delta, err := BuildGraph(t.Context(), src)
			require.NoError(t, err)
			require.NotEmpty(t, delta.Edges, "the graph has a first writer's worth of edges")

			byLabel := map[string]int{}
			for _, e := range delta.Edges {
				byLabel[e.Label]++
			}
			assert.Positive(t, byLabel[EdgeLabelUsesTerm], "uses_term edges are written")

			nodeLabels := map[string]int{}
			for _, n := range delta.Nodes {
				nodeLabels[n.Label]++
			}
			assert.Positive(t, nodeLabels[NodeLabelBlock])
			assert.Positive(t, nodeLabels[NodeLabelConcept])

			// The block's collection is a store capability: the merged SQLite
			// store carries it, the browser build's in-memory fallback does not.
			// Where it is present, collection nodes and in_collection edges must
			// both be, and the production store must have them.
			assert.Equal(t, nodeLabels[NodeLabelCollection] > 0, byLabel[EdgeLabelInCollection] > 0,
				"collection nodes and in_collection edges appear together")
			if name == "sqlite" {
				assert.Positive(t, byLabel[EdgeLabelInCollection], "the merged store materializes collections")
				assert.Positive(t, nodeLabels[NodeLabelCollection])
			}

			// h2 uses "content memory" twice (source), folded onto one counted edge.
			var h2 *graph.Edge
			for i := range delta.Edges {
				if delta.Edges[i].ID == UsesTermEdgeID("h2", "c-memory", "") {
					h2 = &delta.Edges[i]
				}
			}
			require.NotNil(t, h2, "the twice-using block has exactly one uses_term edge")
			assert.Equal(t, "2", h2.Properties["count"], "repeat uses are counted, not multiplied into edges")

			// An unused concept contributes a node but no uses_term edge.
			for _, e := range delta.Edges {
				assert.NotEqual(t, ConceptNodeID("c-unused"), e.Target, "an unused concept has no uses")
			}
		})
	}
}

func TestUsesInCollectionsMatchesTheJoin(t *testing.T) {
	for name, src := range backends(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			delta, err := BuildGraph(ctx, src)
			require.NoError(t, err)
			reader := newDeltaEdgeReader(delta)

			for _, conceptID := range []string{"c-memory", "c-term"} {
				// The graph traversal: term → blocks → collection.
				twoHop, err := UsesInCollections(ctx, reader, conceptID)
				require.NoError(t, err)
				assert.NotEmpty(t, twoHop, "a used concept reaches at least one collection")

				// The direct blocks×terms join: Find, grouped to (block, collection).
				res, err := Find(ctx, src, Query{Subject: conceptID})
				require.NoError(t, err)
				joinSet := map[[2]string]bool{}
				for _, o := range res.Occurrences {
					joinSet[[2]string{o.BlockHash, o.Collection}] = true
				}

				graphSet := map[[2]string]bool{}
				for _, u := range twoHop {
					graphSet[[2]string{u.BlockHash, u.Collection}] = true
				}

				assert.Equal(t, joinKeys(joinSet), joinKeys(graphSet),
					"two-hop graph query returns the same (block, collection) set as the direct join")
			}
		})
	}
}

func joinKeys(m map[[2]string]bool) [][2]string {
	out := make([][2]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}
