package host_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The by-location half of the retrieval surface (AD-037): *what applies here?*
// asked of a place rather than of a store.

// pointRecipe is a two-product recipe: one profile governs the docs tree, a
// second (bounded in time) governs the landing pages.
func pointRecipe(t *testing.T, window string) *project.KapiProject {
	t.Helper()
	var p project.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(`version: v1
name: governed-point
defaults:
  source_language: en
  target_languages: [nb]
profiles:
  acme:
    channels: [docs]
  relaunch:
    channels: [landing]
`+window+`collections:
  - name: acme-docs
    channel: acme/docs
    content:
      - path: docs/**/*.md
  - name: acme-landing
    channel: relaunch/landing
    content:
      - path: site/**/*.mdx
`), &p))
	require.NoError(t, p.Validate())
	return &p
}

// pointConcepts is a small vocabulary with one preferred word, one retired word
// with a replacement, and one whose retirement has already lapsed.
func pointConcepts() []terms.Concept {
	lapsed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	return []terms.Concept{
		{
			ID:         "c-memory",
			Domain:     "product",
			Definition: "The store of source/target pairs the loop reuses.",
			Terms: []terms.Term{
				{Text: "content memory", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "translation memory", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
			},
		},
		{
			ID: "c-widget",
			Terms: []terms.Term{
				{Text: "widget", Locale: model.LocaleEnglish, Status: model.TermForbidden,
					Validity: &graph.Validity{ValidTo: &lapsed}},
				{Text: "dings", Locale: model.LocaleID("nb"), Status: model.TermPreferred},
			},
		},
	}
}

func pointVoice() *coreprofile.VoiceProfile {
	return &coreprofile.VoiceProfile{
		Name:        "Acme",
		Description: "Plain, exact writing.",
		Tone:        coreprofile.ToneProfile{Personality: []string{"clear"}, Formality: "neutral"},
	}
}

// renderAnswer is the markdown document the CLI prints and the `context://`
// resource serves.
func renderAnswer(t *testing.T, res *host.ContextAnswer) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	return buf.String()
}

// resolveAt is the by-location answer for one point, assembled the way
// ContextSourcesAt assembles it: one governance resolution, shared.
func resolveAt(t *testing.T, proj *project.KapiProject, req host.ContextPointRequest, src host.ContextPointSources) *host.ContextAnswer {
	t.Helper()
	if src.At.IsZero() {
		src.At = time.Now()
	}
	src.Recipe = proj
	src.Path = req.Path
	rc, err := host.ResolveContextGovernance(proj, req, req.Path, src.At)
	require.NoError(t, err)
	src.Governance = rc
	if proj != nil && req.Path != "" {
		src.Collection = proj.CollectionForPath(req.Path)
	}
	res, err := host.ResolveContextAt(t.Context(), src, req)
	require.NoError(t, err)
	return res
}

// TestResolveContextAt_ReportsTheCoordinate: the answer says where it is about,
// because a caller holding two answers must be able to tell them apart — and
// because the point it names is the point its voice and terms were read at.
func TestResolveContextAt_ReportsTheCoordinate(t *testing.T) {
	proj := pointRecipe(t, "")

	tests := []struct {
		name           string
		req            host.ContextPointRequest
		wantRef        string
		wantProfile    string
		wantChannel    string
		wantCollection string
		wantDefault    bool
	}{
		{
			name:           "a governed path",
			req:            host.ContextPointRequest{Path: "docs/guide.md"},
			wantRef:        "acme/docs",
			wantProfile:    "acme",
			wantChannel:    "docs",
			wantCollection: "acme-docs",
		},
		{
			name:           "a second product's path",
			req:            host.ContextPointRequest{Path: "site/index.mdx"},
			wantRef:        "relaunch/landing",
			wantProfile:    "relaunch",
			wantChannel:    "landing",
			wantCollection: "acme-landing",
		},
		{
			name:        "a path no collection claims",
			req:         host.ContextPointRequest{Path: "README.md"},
			wantDefault: true,
		},
		{
			name:        "a profile addressed by name carries no channel",
			req:         host.ContextPointRequest{Profile: "acme"},
			wantRef:     "acme",
			wantProfile: "acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := resolveAt(t, proj, tt.req, host.ContextPointSources{})
			assert.Equal(t, tt.wantRef, res.Point.Ref)
			assert.Equal(t, tt.wantProfile, res.Point.Profile)
			assert.Equal(t, tt.wantChannel, res.Point.Channel)
			assert.Equal(t, tt.wantCollection, res.Point.Collection)
			assert.Equal(t, tt.wantDefault, res.Point.Default)
		})
	}
}

// TestResolveContextAt_ExpiredProfileFallsThroughAndSaysSo: a profile that
// stopped governing on a date must not silently change the scenery. The path
// answer resolves at the instant, so the lapse applies and is reported.
func TestResolveContextAt_ExpiredProfileFallsThroughAndSaysSo(t *testing.T) {
	proj := pointRecipe(t, "    valid_to: 2020-01-01\n")

	res := resolveAt(t, proj, host.ContextPointRequest{Path: "site/index.mdx"}, host.ContextPointSources{})

	assert.True(t, res.Point.Default, "the lapsed profile no longer governs")
	assert.Contains(t, res.Notes,
		`profile "relaunch" expired 2020-01-01; governing with the project default`)
}

// TestResolveContextAt_NamedProfileIsAsDeclared: asking what a profile holds has
// an answer before it comes into force and after it lapses — the window is
// reported, not applied, so an "as if" question is answerable at all.
func TestResolveContextAt_NamedProfileIsAsDeclared(t *testing.T) {
	proj := pointRecipe(t, "    valid_to: 2020-01-01\n")

	res := resolveAt(t, proj, host.ContextPointRequest{Profile: "relaunch"}, host.ContextPointSources{
		Profiles: []host.ContextProfileHit{{Name: "relaunch", ValidTo: "2020-01-01", State: "expired"}},
	})

	assert.Equal(t, "relaunch", res.Point.Profile, "the profile asked about is the one answered")
	require.Len(t, res.Profiles, 1)
	assert.Equal(t, "expired", res.Profiles[0].State, "and its window is on the answer")
}

// TestResolveContextAt_TermsAreRankedAndCapped: a capped briefing keeps the
// rules a writer must not break, because truncating alphabetically would drop
// them about as often as it kept them.
func TestResolveContextAt_TermsAreRankedAndCapped(t *testing.T) {
	proj := pointRecipe(t, "")

	res := resolveAt(t, proj, host.ContextPointRequest{Path: "docs/guide.md", Limit: 2},
		host.ContextPointSources{Concepts: pointConcepts()})

	require.Len(t, res.Terms, 2)
	assert.Equal(t, "translation memory", res.Terms[0].Term, "the discouraged word leads")
	assert.True(t, res.Terms[0].Discouraged)
	assert.Equal(t, "content memory", res.Terms[0].Replacement, "with what to say instead")
	assert.Equal(t, "content memory", res.Terms[1].Term)
	assert.Equal(t, 3, res.TermsTotal, "the lapsed ban is out; three terms remain in force")
	for _, n := range res.Notes {
		assert.NotContains(t, n, "terms bound here", "the cap is a count on the answer, not a note")
	}

	var sb strings.Builder
	res.FormatText(&sb)
	assert.Contains(t, sb.String(), "Showing 2 of 3 terms bound here. `kapi context search <word>` finds one by name.",
		"the text rendering says where the rest are")
}

// TestResolveContextAt_LapsedTermBanIsNotReported: "recognise the old name until
// March" is a different instruction from "never use it", and a retrieval answer
// must not go on enforcing a rule whose window has closed.
func TestResolveContextAt_LapsedTermBanIsNotReported(t *testing.T) {
	res := resolveAt(t, pointRecipe(t, ""), host.ContextPointRequest{Path: "docs/guide.md"},
		host.ContextPointSources{Concepts: pointConcepts()})

	for _, term := range res.Terms {
		assert.NotEqual(t, "widget", term.Term, "the ban lapsed in 2020")
	}
}

// TestResolveContextAt_LocaleNarrowsTheTerms: one spelling can be retired in one
// language and admitted in another, so a caller writing in one is not handed the
// other's vocabulary.
func TestResolveContextAt_LocaleNarrowsTheTerms(t *testing.T) {
	res := resolveAt(t, pointRecipe(t, ""),
		host.ContextPointRequest{Path: "docs/guide.md", Locale: model.LocaleID("nb")},
		host.ContextPointSources{Concepts: pointConcepts()})

	require.Len(t, res.Terms, 1)
	assert.Equal(t, "dings", res.Terms[0].Term)
}

// TestResolveContextAt_StatesItsScope is AD-037's reach-not-capability rule: a
// caller must be able to tell "this project holds no answer" from "this scope
// cannot hold one", which it can only do if the answer says what it read.
func TestResolveContextAt_StatesItsScope(t *testing.T) {
	tests := []struct {
		name      string
		proj      *project.KapiProject
		req       host.ContextPointRequest
		src       host.ContextPointSources
		wantScope host.ContextScope
		wantLine  string
		wantNote  string
	}{
		{
			name:      "a project answer names the project",
			proj:      pointRecipe(t, ""),
			req:       host.ContextPointRequest{Path: "docs/guide.md"},
			src:       host.ContextPointSources{Concepts: pointConcepts()},
			wantScope: host.ScopeProject,
			wantLine:  "Answered from this project alone",
			wantNote:  "project scope: concept relations, revisions and market scoping live in a connected workspace",
		},
		{
			name:      "a profile answered with no project behind it says so",
			req:       host.ContextPointRequest{Profile: "technical-docs"},
			src:       host.ContextPointSources{Scope: host.ScopeProfile, Voice: pointVoice()},
			wantScope: host.ScopeProfile,
			wantLine:  "Answered from one voice profile alone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := resolveAt(t, tt.proj, tt.req, tt.src)
			assert.Equal(t, tt.wantScope, res.Scope)
			assert.Contains(t, renderAnswer(t, res), tt.wantLine)
			if tt.wantNote != "" {
				assert.Contains(t, res.Notes, tt.wantNote)
			}
		})
	}
}

// TestResolveContextAt_FreshnessLeadsTheNotes: the staleness note the search
// path carries is the same note here, and it comes first — it is the only note
// that says the rest of the answer may already describe a graph that has moved.
func TestResolveContextAt_FreshnessLeadsTheNotes(t *testing.T) {
	const moved = "terms moved since this was last read here — re-read the project's context before continuing"

	res := resolveAt(t, pointRecipe(t, ""), host.ContextPointRequest{Path: "docs/guide.md"},
		host.ContextPointSources{Freshness: []string{moved}, Concepts: pointConcepts()})

	require.NotEmpty(t, res.Notes)
	assert.Equal(t, moved, res.Notes[0])
}

// TestResolveContextAt_RendersTheVoiceGuide: the guidance a writer reads here is
// the rendering `kapi voice guide` prints and the translation prompt carries —
// not a second, narrower summary of the same profile.
func TestResolveContextAt_RendersTheVoiceGuide(t *testing.T) {
	voice := pointVoice()
	res := resolveAt(t, pointRecipe(t, ""), host.ContextPointRequest{Path: "docs/guide.md"},
		host.ContextPointSources{Voice: voice, VoiceSource: ".kapi/voice.yaml"})

	require.NotNil(t, res.Voice)
	assert.Equal(t, "Acme", res.Voice.Name)
	assert.Equal(t, coreprofile.RenderVoiceGuide(voice), res.Voice.Guide)

	// Nested one level under the answer's own title, so the result is one
	// document rather than two competing ones.
	text := renderAnswer(t, res)
	assert.Contains(t, text, "# Context at docs/guide.md")
	assert.Contains(t, text, "\n## Voice Guide: Acme\n")
	assert.NotContains(t, text, "\n# Voice Guide: Acme\n")
}

// TestResolveContextAt_NeedsAnAddress: a request that names neither a location
// nor a profile is not a thin answer, it is not a question.
func TestResolveContextAt_NeedsAnAddress(t *testing.T) {
	_, err := host.ResolveContextAt(t.Context(), host.ContextPointSources{}, host.ContextPointRequest{})
	require.Error(t, err)
}
