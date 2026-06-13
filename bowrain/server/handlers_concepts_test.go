package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
)

// These tests drive the concept list handler against the in-memory workspace
// termbase (no database). The gtConcept / harness helpers are shared from
// handlers_concepts_graph_test.go.

// TestListConceptsTotalKeepsDBWideCount proves the page-facet post-filter
// (status/domain/market/source) does not collapse total_count to the surviving
// page size: a workspace of many concepts keeps its DB-wide count even when a
// facet narrows the returned page to a handful. Without the fix, total_count
// would report len(filtered) — at most the page size — so the UI would show a
// single-digit concept count for a large workspace.
func TestListConceptsTotalKeepsDBWideCount(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	tb := h.tb(t)

	// 8 approved concepts and 2 forbidden ones — 10 in the workspace.
	for i := 0; i < 8; i++ {
		require.NoError(t, tb.AddConcept(ctx, gtConcept(fmt.Sprintf("ok%02d", i), "auth", model.TermApproved)))
	}
	require.NoError(t, tb.AddConcept(ctx, gtConcept("bad1", "auth", model.TermForbidden)))
	require.NoError(t, tb.AddConcept(ctx, gtConcept("bad2", "auth", model.TermForbidden)))

	c, rec := h.req(http.MethodGet, "/?status=forbidden", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleListConcepts(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp TermSearchResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// The page is post-filtered to the two forbidden concepts...
	assert.Len(t, resp.Concepts, 2, "facet narrows the returned page")
	// ...but total_count stays the DB-wide concept count, not the 2 that survived.
	assert.Equal(t, 10, resp.TotalCount, "total reflects the workspace, not the filtered page")
}

// TestListConceptsTotalUnfilteredFullCount proves the unfiltered list reports the
// full workspace count as total_count (the baseline the facet case must not
// regress below).
func TestListConceptsTotalUnfilteredFullCount(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	tb := h.tb(t)
	for i := 0; i < 6; i++ {
		require.NoError(t, tb.AddConcept(ctx, gtConcept(fmt.Sprintf("c%02d", i), "auth", model.TermApproved)))
	}

	c, rec := h.req(http.MethodGet, "/", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleListConcepts(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp TermSearchResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Concepts, 6)
	assert.Equal(t, 6, resp.TotalCount)
}
