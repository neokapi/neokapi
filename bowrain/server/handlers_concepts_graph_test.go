package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// These tests cover the pure graph-assembly logic behind HandleGetGraphViz (the
// node cap, the governed worth-seeing subset, the focus BFS, the truncated flag,
// and edge consistency) plus a handler-level check that the cap and the new
// total/truncated fields reach the JSON. They need no database — the pure helpers
// run on slices and the handler path drives the in-memory workspace termbase.

func gtConcept(id, domain string, statuses ...model.TermStatus) termbase.Concept {
	terms := make([]termbase.Term, len(statuses))
	for i, st := range statuses {
		terms[i] = termbase.Term{Text: id + "-" + string(st), Locale: "en", Status: st}
	}
	return termbase.Concept{ID: id, Domain: domain, Terms: terms, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func gtRelation(id, src, tgt string) termbase.ConceptRelation {
	return termbase.ConceptRelation{ID: id, SourceID: src, TargetID: tgt, RelationType: graph.LabelRelated, CreatedAt: time.Now()}
}

func nodeIDs(resp GraphVizResponse) []string {
	ids := make([]string, len(resp.Nodes))
	for i, n := range resp.Nodes {
		ids[i] = n.ID
	}
	return ids
}

// ---------------------------------------------------------------------------
// graphNodeLimit
// ---------------------------------------------------------------------------

func TestGraphNodeLimit(t *testing.T) {
	assert.Equal(t, graphDefaultNodeLimit, graphNodeLimit(""), "missing → default")
	assert.Equal(t, graphDefaultNodeLimit, graphNodeLimit("0"), "zero → default")
	assert.Equal(t, graphDefaultNodeLimit, graphNodeLimit("-5"), "negative → default")
	assert.Equal(t, graphDefaultNodeLimit, graphNodeLimit("abc"), "garbage → default")
	assert.Equal(t, 25, graphNodeLimit("25"), "in-range value passes through")
	assert.Equal(t, graphMaxNodeLimit, graphNodeLimit("100000"), "above ceiling → clamped")
}

// ---------------------------------------------------------------------------
// conceptIsSteered
// ---------------------------------------------------------------------------

func TestConceptIsSteered(t *testing.T) {
	assert.True(t, conceptIsSteered(gtConcept("a", "", model.TermPreferred)))
	assert.True(t, conceptIsSteered(gtConcept("a", "", model.TermForbidden)))
	assert.True(t, conceptIsSteered(gtConcept("a", "", model.TermDeprecated)))
	assert.False(t, conceptIsSteered(gtConcept("a", "", model.TermProposed)))
	assert.False(t, conceptIsSteered(gtConcept("a", "", model.TermApproved, model.TermAdmitted)))
	assert.False(t, conceptIsSteered(gtConcept("a", "")), "no terms → not steered")
}

// ---------------------------------------------------------------------------
// governedSubsetOrder
// ---------------------------------------------------------------------------

// TestGovernedSubsetOrderTiers proves the default ranking lists relation-connected
// concepts first, then steered ones, then the rest — preserving input order
// within each tier so the result is deterministic.
func TestGovernedSubsetOrderTiers(t *testing.T) {
	candidates := []termbase.Concept{
		gtConcept("plain1", "", model.TermProposed),     // tier 3 (unconnected, unsteered)
		gtConcept("steered1", "", model.TermPreferred),  // tier 2
		gtConcept("conn1", "", model.TermProposed),      // tier 1 (connected below)
		gtConcept("plain2", "", model.TermApproved),     // tier 3
		gtConcept("conn2", "", model.TermForbidden),     // tier 1 (connected); steered but connection wins
		gtConcept("steered2", "", model.TermDeprecated), // tier 2
	}
	relations := []termbase.ConceptRelation{gtRelation("r1", "conn1", "conn2")}

	order := governedSubsetOrder(candidates, relations)
	assert.Equal(t, []string{"conn1", "conn2", "steered1", "steered2", "plain1", "plain2"}, order)
}

// ---------------------------------------------------------------------------
// orderedNeighborhood
// ---------------------------------------------------------------------------

// TestOrderedNeighborhoodDepth proves BFS expands hop by hop and that depth
// bounds the reach, with focus always first.
func TestOrderedNeighborhoodDepth(t *testing.T) {
	// a — b — c — d (a chain)
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "a", "b"),
		gtRelation("r2", "b", "c"),
		gtRelation("r3", "c", "d"),
	}
	assert.Equal(t, []string{"a"}, orderedNeighborhood("a", rels, 0), "depth 0 → just the focus")
	assert.Equal(t, []string{"a", "b"}, orderedNeighborhood("a", rels, 1))
	assert.Equal(t, []string{"a", "b", "c"}, orderedNeighborhood("a", rels, 2))
	assert.Equal(t, []string{"a", "b", "c", "d"}, orderedNeighborhood("a", rels, 3))
	assert.Equal(t, []string{"a", "b", "c", "d"}, orderedNeighborhood("a", rels, 9), "depth past the graph diameter is harmless")
}

// TestOrderedNeighborhoodDeterministic proves the discovery order follows
// relation ID order regardless of the input relation order, so a later breadth
// truncation is stable.
func TestOrderedNeighborhoodDeterministic(t *testing.T) {
	// focus connects to three nodes via relations whose IDs sort z < y < x's text? Use
	// IDs that sort r1 < r2 < r3 mapping to targets c, b, d respectively.
	rels := []termbase.ConceptRelation{
		gtRelation("r3", "focus", "d"),
		gtRelation("r1", "focus", "c"),
		gtRelation("r2", "focus", "b"),
	}
	// Sorted by ID: r1(→c), r2(→b), r3(→d) ⇒ c, b, d after focus.
	assert.Equal(t, []string{"focus", "c", "b", "d"}, orderedNeighborhood("focus", rels, 1))
}

func TestOrderedNeighborhoodIsolatedFocus(t *testing.T) {
	assert.Equal(t, []string{"lonely"}, orderedNeighborhood("lonely", nil, 3))
}

// TestOrderedNeighborhoodCyclic proves a cycle does not loop: the `seen` set
// visits each concept once, so even a generous depth over a ring returns each
// node a single time with focus first.
func TestOrderedNeighborhoodCyclic(t *testing.T) {
	// a — b — c — a (a triangle) plus a self-loop on a.
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "a", "b"),
		gtRelation("r2", "b", "c"),
		gtRelation("r3", "c", "a"),
		gtRelation("r4", "a", "a"),
	}
	got := orderedNeighborhood("a", rels, 9)
	assert.Equal(t, "a", got[0], "focus first")
	assert.ElementsMatch(t, []string{"a", "b", "c"}, got, "every node once, no repeats")
	assert.Len(t, got, 3, "the cycle and self-loop do not re-add nodes")
}

// ---------------------------------------------------------------------------
// assembleGraphViz — default (governed subset) mode
// ---------------------------------------------------------------------------

// TestAssembleDefaultUnderCap proves a graph that fits under the cap renders in
// full, with total equal to the candidate count and truncated false.
func TestAssembleDefaultUnderCap(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("a", "x", model.TermPreferred),
		gtConcept("b", "x", model.TermApproved),
		gtConcept("c", "x", model.TermProposed),
	}
	rels := []termbase.ConceptRelation{gtRelation("r1", "a", "b")}

	resp := assembleGraphViz(concepts, rels, graphVizParams{})
	assert.Len(t, resp.Nodes, 3)
	assert.Equal(t, 3, resp.Total)
	assert.False(t, resp.Truncated)
	require.Len(t, resp.Edges, 1, "edge between two included nodes survives")
}

// TestAssembleDefaultCapKeepsWorthSeeing proves that when the candidate count
// exceeds the cap, the surviving nodes are the worth-seeing ones (connected, then
// steered), the cap is enforced, and total/truncated reflect the full set.
func TestAssembleDefaultCapKeepsWorthSeeing(t *testing.T) {
	var concepts []termbase.Concept
	// 5 plain (unconnected, unsteered) concepts.
	for i := 0; i < 5; i++ {
		concepts = append(concepts, gtConcept(fmt.Sprintf("plain%d", i), "x", model.TermProposed))
	}
	// 2 connected + 1 steered — the 3 worth-seeing ones.
	concepts = append(concepts,
		gtConcept("conn1", "x", model.TermProposed),
		gtConcept("conn2", "x", model.TermProposed),
		gtConcept("steered", "x", model.TermForbidden),
	)
	rels := []termbase.ConceptRelation{gtRelation("r1", "conn1", "conn2")}

	resp := assembleGraphViz(concepts, rels, graphVizParams{limit: 3})
	require.Len(t, resp.Nodes, 3)
	assert.Equal(t, []string{"conn1", "conn2", "steered"}, nodeIDs(resp), "connected first, then steered")
	assert.Equal(t, 8, resp.Total, "total counts every candidate")
	assert.True(t, resp.Truncated)
	require.Len(t, resp.Edges, 1)
}

// TestAssembleDefaultClampsToMax proves an over-ceiling limit is clamped.
func TestAssembleDefaultClampsToMax(t *testing.T) {
	concepts := []termbase.Concept{gtConcept("a", "x", model.TermProposed)}
	resp := assembleGraphViz(concepts, nil, graphVizParams{limit: 999999})
	assert.Len(t, resp.Nodes, 1)
	assert.False(t, resp.Truncated)
}

// ---------------------------------------------------------------------------
// assembleGraphViz — filters
// ---------------------------------------------------------------------------

// TestAssembleDomainFilter proves the domain filter narrows the candidate set and
// that edges to filtered-out concepts are dropped.
func TestAssembleDomainFilter(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("a", "commerce", model.TermPreferred),
		gtConcept("b", "commerce", model.TermApproved),
		gtConcept("c", "marketing", model.TermProposed),
	}
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "a", "b"), // both commerce → kept
		gtRelation("r2", "b", "c"), // crosses into marketing → dropped
	}

	resp := assembleGraphViz(concepts, rels, graphVizParams{domain: "commerce"})
	assert.ElementsMatch(t, []string{"a", "b"}, nodeIDs(resp))
	assert.Equal(t, 2, resp.Total)
	require.Len(t, resp.Edges, 1)
	assert.Equal(t, "r1", resp.Edges[0].ID)
}

// TestAssembleStatusFilter proves the status filter keeps only concepts that
// carry that term status.
func TestAssembleStatusFilter(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("a", "x", model.TermForbidden),
		gtConcept("b", "x", model.TermApproved),
		gtConcept("c", "x", model.TermForbidden, model.TermProposed),
	}
	resp := assembleGraphViz(concepts, nil, graphVizParams{status: model.TermForbidden})
	assert.ElementsMatch(t, []string{"a", "c"}, nodeIDs(resp))
}

// ---------------------------------------------------------------------------
// assembleGraphViz — focus mode
// ---------------------------------------------------------------------------

// TestAssembleFocusNeighbourhood proves focus mode returns the BFS neighbourhood
// at the given depth with all edges among the returned nodes.
func TestAssembleFocusNeighbourhood(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("a", "x", model.TermPreferred),
		gtConcept("b", "x", model.TermApproved),
		gtConcept("c", "x", model.TermProposed),
		gtConcept("d", "x", model.TermProposed),
		gtConcept("far", "x", model.TermProposed),
	}
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "a", "b"),
		gtRelation("r2", "a", "c"),
		gtRelation("r3", "b", "d"),
		gtRelation("r4", "far", "far"), // disconnected self-loop elsewhere
	}

	resp := assembleGraphViz(concepts, rels, graphVizParams{focus: "a", depth: 1})
	assert.ElementsMatch(t, []string{"a", "b", "c"}, nodeIDs(resp))
	assert.Equal(t, 3, resp.Total)
	assert.False(t, resp.Truncated)
	// Edges among {a,b,c}: r1 (a-b) and r2 (a-c). r3 (b-d) excluded (d not in set).
	assert.Len(t, resp.Edges, 2)
}

// TestAssembleFocusDepthDefault proves a missing depth defaults to 1 hop.
func TestAssembleFocusDepthDefault(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("a", "x", model.TermProposed),
		gtConcept("b", "x", model.TermProposed),
		gtConcept("c", "x", model.TermProposed),
	}
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "a", "b"),
		gtRelation("r2", "b", "c"),
	}
	resp := assembleGraphViz(concepts, rels, graphVizParams{focus: "a"}) // depth unset
	assert.ElementsMatch(t, []string{"a", "b"}, nodeIDs(resp), "depth defaults to 1")
}

// TestAssembleFocusCapTruncates proves focus mode honours the node cap and
// reports the full neighbourhood size as total.
func TestAssembleFocusCapTruncates(t *testing.T) {
	concepts := []termbase.Concept{gtConcept("hub", "x", model.TermProposed)}
	var rels []termbase.ConceptRelation
	for i := 0; i < 10; i++ {
		spoke := fmt.Sprintf("s%02d", i)
		concepts = append(concepts, gtConcept(spoke, "x", model.TermProposed))
		rels = append(rels, gtRelation(fmt.Sprintf("r%02d", i), "hub", spoke))
	}

	resp := assembleGraphViz(concepts, rels, graphVizParams{focus: "hub", depth: 1, limit: 4})
	require.Len(t, resp.Nodes, 4)
	assert.Equal(t, "hub", resp.Nodes[0].ID, "focus is kept first")
	assert.Equal(t, 11, resp.Total, "hub + 10 spokes")
	assert.True(t, resp.Truncated)
	// Deterministic breadth truncation: r00..r02 (sorted by ID) ⇒ s00..s02.
	assert.Equal(t, []string{"hub", "s00", "s01", "s02"}, nodeIDs(resp))
}

// TestAssembleFocusComposesWithFilter proves a filter narrows the candidate set
// before the focus BFS, so neighbours outside the filter are not reachable.
func TestAssembleFocusComposesWithFilter(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("a", "commerce", model.TermProposed),
		gtConcept("b", "commerce", model.TermProposed),
		gtConcept("c", "marketing", model.TermProposed),
	}
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "a", "b"),
		gtRelation("r2", "a", "c"),
	}
	resp := assembleGraphViz(concepts, rels, graphVizParams{focus: "a", depth: 2, domain: "commerce"})
	assert.ElementsMatch(t, []string{"a", "b"}, nodeIDs(resp), "marketing neighbour is unreachable")
}

// TestAssembleFocusFilteredOut proves that focusing a concept the filter excludes
// yields an empty selection rather than an error.
func TestAssembleFocusFilteredOut(t *testing.T) {
	concepts := []termbase.Concept{gtConcept("a", "commerce", model.TermProposed)}
	resp := assembleGraphViz(concepts, nil, graphVizParams{focus: "a", domain: "marketing"})
	assert.Empty(t, resp.Nodes)
	assert.Equal(t, 0, resp.Total)
	assert.False(t, resp.Truncated)
}

// ---------------------------------------------------------------------------
// assembleGraphViz — edge consistency
// ---------------------------------------------------------------------------

// TestAssembleEdgeConsistency proves no edge survives unless both endpoints are
// in the returned node set, even when the cap drops one endpoint.
func TestAssembleEdgeConsistency(t *testing.T) {
	concepts := []termbase.Concept{
		gtConcept("conn1", "x", model.TermProposed),
		gtConcept("conn2", "x", model.TermProposed),
		gtConcept("conn3", "x", model.TermProposed),
	}
	rels := []termbase.ConceptRelation{
		gtRelation("r1", "conn1", "conn2"),
		gtRelation("r2", "conn2", "conn3"),
	}
	// Cap at 2: only conn1, conn2 survive (input order within the connected tier).
	resp := assembleGraphViz(concepts, rels, graphVizParams{limit: 2})
	require.Len(t, resp.Nodes, 2)
	assert.Equal(t, []string{"conn1", "conn2"}, nodeIDs(resp))
	require.Len(t, resp.Edges, 1, "only the edge with both endpoints kept survives")
	assert.Equal(t, "r1", resp.Edges[0].ID)
}

// ---------------------------------------------------------------------------
// Handler-level: cap + total/truncated reach the JSON
// ---------------------------------------------------------------------------

// TestGraphVizHandlerCapAndTotals drives the echo handler against the in-memory
// termbase and proves the limit query param caps the nodes and the response
// carries total + truncated.
func TestGraphVizHandlerCapAndTotals(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	tb := h.tb(t)

	// 5 relation-connected concepts in a chain so all are worth-seeing.
	ids := []string{"c1", "c2", "c3", "c4", "c5"}
	for _, id := range ids {
		require.NoError(t, tb.AddConcept(ctx, gtConcept(id, "auth", model.TermApproved)))
	}
	for i := 0; i+1 < len(ids); i++ {
		require.NoError(t, tb.AddRelation(ctx, gtRelation(fmt.Sprintf("r%d", i), ids[i], ids[i+1])))
	}

	c, rec := h.req(http.MethodGet, "/?limit=3", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleGetGraphViz(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp GraphVizResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Nodes, 3, "capped at limit=3")
	assert.Equal(t, 5, resp.Total)
	assert.True(t, resp.Truncated)
	// Every edge endpoint must be among the returned nodes.
	in := map[string]bool{}
	for _, n := range resp.Nodes {
		in[n.ID] = true
	}
	for _, e := range resp.Edges {
		assert.True(t, in[e.Source] && in[e.Target], "edge %s spans a dropped node", e.ID)
	}
}

// TestGraphVizHandlerDefaultNotTruncated proves a small workspace renders in full
// (truncated false) under the default cap.
func TestGraphVizHandlerDefaultNotTruncated(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	tb := h.tb(t)
	require.NoError(t, tb.AddConcept(ctx, gtConcept("c1", "auth", model.TermPreferred)))
	require.NoError(t, tb.AddConcept(ctx, gtConcept("c2", "auth", model.TermForbidden)))

	c, rec := h.req(http.MethodGet, "/", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleGetGraphViz(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp GraphVizResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Nodes, 2)
	assert.Equal(t, 2, resp.Total)
	assert.False(t, resp.Truncated)
}
