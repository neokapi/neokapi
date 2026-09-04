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
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// These tests drive the concept list handler against the in-memory workspace
// terms (no database), using the newKGHarness helpers.

// gtConcept builds an in-memory concept with one term per status (each term's
// text derived from the id + status), for the handler tests above.
func gtConcept(id, domain string, statuses ...model.TermStatus) terms.Concept {
	ts := make([]terms.Term, len(statuses))
	for i, st := range statuses {
		ts[i] = terms.Term{Text: id + "-" + string(st), Locale: "en", Status: st}
	}
	return terms.Concept{ID: id, Domain: domain, Terms: ts, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

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
	for i := range 8 {
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
	for i := range 6 {
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

// TestCreateConceptAcceptsHarnessSeedPayload holds the concept the harness
// seeds (harness/scripts/seed-collaboration.mjs and seed-bowrain.ts) against the
// route it posts to: the payload shape, the project scope, and the term
// statuses the direct route accepts. The seeds create their terms `approved`
// because `preferred` is a governed status, refused here with a change-set
// hint. The lookup at the end is the one the review context runs over a
// block's source, so a seeded term reaches the reviewer's Terms card.
func TestCreateConceptAcceptsHarnessSeedPayload(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()

	seeded := `{
		"domain": "security",
		"definition": "Protecting data so only authorised parties can read it, end to end.",
		"terms": [
			{"text": "encryption", "locale": "en", "status": "approved"},
			{"text": "chiffrement", "locale": "fr", "status": "approved"},
			{"text": "cryptage", "locale": "fr", "status": "deprecated"},
			{"text": "Verschlüsselung", "locale": "de", "status": "approved"}
		],
		"project_id": "proj-seed"
	}`
	c, rec := h.req(http.MethodPost, "/concepts", seeded, platauth.PermManageTerms)
	require.NoError(t, h.srv.HandleCreateConcept(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created ConceptInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "proj-seed", created.ProjectID)
	assert.Equal(t, "security", created.Domain)
	require.Len(t, created.Terms, 4)
	assert.Equal(t, "deprecated", created.Terms[2].Status)

	stored, ok, err := h.tb(t).GetConcept(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, ok, "the concept is in the workspace terms")
	assert.Equal(t, "proj-seed", stored.ProjectID)

	matches, err := h.tb(t).LookupAll(ctx, "SOC 2 Type II certified with end-to-end encryption.",
		terms.LookupOptions{SourceLocale: "en", TargetLocale: "fr", ProjectID: "proj-seed"})
	require.NoError(t, err)
	require.Len(t, matches, 1, "the seeded term is found in the file's source text")
	assert.Equal(t, "encryption", matches[0].Term.Text)

	governed := `{
		"domain": "cloud",
		"definition": "Managed, multi-tenant compute and storage delivered over the network.",
		"terms": [{"text": "cloud infrastructure", "locale": "en", "status": "preferred"}],
		"project_id": "proj-seed"
	}`
	c, rec = h.req(http.MethodPost, "/concepts", governed, platauth.PermManageTerms)
	require.NoError(t, h.srv.HandleCreateConcept(c))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var refusal map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
	assert.Equal(t, "governed change requires a change-set", refusal["error"])
}
