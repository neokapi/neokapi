package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// These exercise the aggregates the Brand dashboard used to assemble
// client-side: six limit=1 list calls for six status totals, and a
// 100-concept sample presented as workspace-wide locale coverage.

// seedAggregateConcepts stores the fixture the coverage math is pinned to:
// en-US in three of four concepts, de-DE and fr-FR in one each, one concept
// with no terms at all.
func seedAggregateConcepts(t *testing.T, h *kgHarness) {
	t.Helper()
	tb := h.tb(t)
	add := func(id string, locales ...model.LocaleID) {
		cp := terms.Concept{ID: id, Domain: "commerce", UpdatedAt: time.Now().UTC()}
		for _, locale := range locales {
			cp.Terms = append(cp.Terms, terms.Term{
				Text: id + "-" + string(locale), Locale: locale, Status: model.TermPreferred,
			})
		}
		require.NoError(t, tb.AddConcept(t.Context(), cp))
	}
	add("a", "en-US", "de-DE")
	add("b", "en-US")
	add("c", "en-US", "fr-FR")
	add("d")
}

func TestConceptStatusCountsBucketsInOneCall(t *testing.T) {
	h := newKGHarness(t)
	tb := h.tb(t)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{ID: "a", Terms: []terms.Term{
		{Text: "sign in", Locale: "en", Status: model.TermPreferred},
		{Text: "log in", Locale: "en", Status: model.TermForbidden},
	}}))
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{ID: "b", Terms: []terms.Term{
		{Text: "cart", Locale: "en", Status: model.TermPreferred},
	}}))

	c, rec := h.req(http.MethodGet, "/concepts/status-counts", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleGetConceptStatusCounts(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got ConceptStatusCountsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, 2, got.ByStatus["preferred"])
	assert.Equal(t, 1, got.ByStatus["forbidden"])
	// Every known status is present, so a caller renders a stable set of bars
	// rather than reading a missing key as an error.
	assert.Len(t, got.ByStatus, 6)
	assert.Contains(t, got.ByStatus, "deprecated")
	assert.Zero(t, got.ByStatus["deprecated"])
}

func TestConceptLocaleCoverageSpansTheWorkspace(t *testing.T) {
	h := newKGHarness(t)
	seedAggregateConcepts(t, h)

	c, rec := h.req(http.MethodGet, "/concepts/locale-coverage", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleGetConceptLocaleCoverage(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got LocaleCoverageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, 4, got.Total, "a concept with no terms is still a concept")
	require.Len(t, got.Locales, 3)
	assert.Equal(t, model.LocaleID("en-US"), got.Locales[0].Locale)
	assert.Equal(t, 3, got.Locales[0].Present)
	assert.Equal(t, 4, got.Locales[0].Total)
	assert.Equal(t, 75, got.Locales[0].Pct)
	assert.Equal(t, []model.LocaleID{"en-US", "de-DE", "fr-FR"},
		[]model.LocaleID{got.Locales[0].Locale, got.Locales[1].Locale, got.Locales[2].Locale})
}

func TestConceptLocaleCoverageEmptyWorkspace(t *testing.T) {
	h := newKGHarness(t)

	c, rec := h.req(http.MethodGet, "/concepts/locale-coverage", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleGetConceptLocaleCoverage(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got LocaleCoverageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Zero(t, got.Total)
	assert.Empty(t, got.Locales, "an empty workspace answers with an empty list, not null")
	assert.Contains(t, rec.Body.String(), `"locales":[]`)
}

func TestConceptAggregatesRequireViewContent(t *testing.T) {
	h := newKGHarness(t)
	for path, handler := range map[string]func(echo.Context) error{
		"/concepts/status-counts":   h.srv.HandleGetConceptStatusCounts,
		"/concepts/locale-coverage": h.srv.HandleGetConceptLocaleCoverage,
	} {
		c, _ := h.req(http.MethodGet, path, "", 0)
		require.Error(t, handler(c), "%s must refuse a caller without view_content", path)
	}
}

func TestListConceptsSortsByLastUpdate(t *testing.T) {
	h := newKGHarness(t)
	tb := h.tb(t)
	now := time.Now().UTC()
	for i, id := range []string{"oldest", "middle", "newest"} {
		require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
			ID:        id,
			Terms:     []terms.Term{{Text: id, Locale: "en", Status: model.TermApproved}},
			CreatedAt: now,
			UpdatedAt: now.Add(time.Duration(i) * time.Hour),
		}))
	}

	c, rec := h.req(http.MethodGet, "/concepts?sort=updated_at", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleListConcepts(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got TermSearchResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Concepts, 3)
	assert.Equal(t, []string{"newest", "middle", "oldest"},
		[]string{got.Concepts[0].ID, got.Concepts[1].ID, got.Concepts[2].ID})
}

// TestListConceptsRejectsUnknownSort: a typo must not silently fall back to an
// arbitrary order that the caller then presents as "recently changed".
func TestListConceptsRejectsUnknownSort(t *testing.T) {
	h := newKGHarness(t)
	c, rec := h.req(http.MethodGet, "/concepts?sort=updated", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleListConcepts(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
