package host_test

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Retrieval answers "where is this word used" — and a place in a governed
// project is a coordinate, not only a path. The same word may be admitted on
// one surface and discouraged on another, so a context answer resolves the
// point each use sits at through the same seam a run and a check use.

// governedSearchRecipe routes one sub-tree of a docs collection to a second
// profile with a per-item channel, and bounds that profile's validity with the
// caller's window (empty for unbounded).
func governedSearchRecipe(t *testing.T, window string) *project.KapiProject {
	t.Helper()
	var p project.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(`version: v1
name: governed-search
defaults:
  source_language: en
  target_languages: [nb]
profiles:
  promo:
    channels: [docs]
  legal:
    channels: [docs]
`+window+`collections:
  - name: docs
    channel: promo/docs
    content:
      - path: docs/legal/**/*.md
        channel: legal/docs
      - path: docs/**/*.md
`), &p))
	require.NoError(t, p.Validate())
	return &p
}

// governedSearchSources builds a project store holding one discouraged word and
// content using it in two documents, one in the collection's own tree and one
// under the per-item override, with the context graph materialized over them.
// It returns the sources a search reads, bound to the given recipe.
func governedSearchSources(t *testing.T, recipe *project.KapiProject) host.ContextSearchSources {
	t.Helper()
	p := newGraphProject(t, recipe, []terms.Concept{{
		ID: "c-widget",
		Terms: []terms.Term{
			{Text: "widget", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
		},
	}}, []usesBlock{
		{hash: "h1", id: "guide.a", file: "docs/guide.md", source: "Drag a widget onto the board."},
		{hash: "h2", id: "eula.a", file: "docs/legal/eula.md", source: "Every widget is provided as is."},
	})
	src := p.src
	src.Recipe = recipe
	src.At = time.Now()
	return src
}

// usePoints maps each reported use to the point it was resolved at.
func usePoints(res *host.ContextSearchResult) map[string]string {
	out := map[string]string{}
	for _, h := range res.Terms {
		for _, u := range h.TopUses {
			out[u.Document] = u.Point
		}
	}
	return out
}

// TestContextSearch_ReportsThePointEachUseSitsAt: a per-item channel is visible
// in retrieval, so a writer asking about a word learns which surface each use is
// governed on.
func TestContextSearch_ReportsThePointEachUseSitsAt(t *testing.T) {
	src := governedSearchSources(t, governedSearchRecipe(t, ""))

	res, err := host.SearchContext(t.Context(), src, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	points := usePoints(res)
	assert.Equal(t, "promo/docs", points["docs/guide.md"], "the collection's channel")
	assert.Equal(t, "legal/docs", points["docs/legal/eula.md"], "the item's own channel")
}

// TestContextSearch_ExpiredProfileFallsThroughAndSaysSo: the answer never
// reports a coordinate a lapsed profile no longer holds, and the swap becomes a
// note rather than a silent change of scenery.
func TestContextSearch_ExpiredProfileFallsThroughAndSaysSo(t *testing.T) {
	tests := []struct {
		name      string
		window    string
		wantPoint string
		wantNote  string
	}{
		{
			name:      "after valid_to",
			window:    "    valid_to: 2020-01-01\n",
			wantPoint: "promo/docs",
			wantNote:  `profile "legal" expired 2020-01-01; governing with profile "promo"`,
		},
		{
			name:      "before valid_from",
			window:    "    valid_from: 2099-01-01\n",
			wantPoint: "promo/docs",
			wantNote:  `profile "legal" is not in force until 2099-01-01; governing with profile "promo"`,
		},
		{
			name:      "an always-valid profile is untouched",
			window:    "",
			wantPoint: "legal/docs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := governedSearchSources(t, governedSearchRecipe(t, tt.window))
			res, err := host.SearchContext(t.Context(), src, host.ContextSearchRequest{Query: "widget"})
			require.NoError(t, err)

			assert.Equal(t, tt.wantPoint, usePoints(res)["docs/legal/eula.md"],
				"the overridden file falls back to its collection's channel")
			if tt.wantNote == "" {
				for _, n := range res.Notes {
					assert.NotContains(t, n, "governing with", "nothing lapsed, so nothing to report")
				}
				return
			}
			assert.Contains(t, res.Notes, tt.wantNote)
		})
	}
}
