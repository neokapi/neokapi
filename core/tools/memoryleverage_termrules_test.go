package tools_test

import (
	"testing"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dashboardRule = profile.TermRule{Term: "dashboard", Replacement: "tableau de bord"}

// termRuleCases are the matches recycle is offered, and whether each may fill
// the target under the rules that govern the block. The decision is term-check's,
// so declared forms, accepted renderings, do-not-translate terms, severities and
// placeholder names count the way the ship gate counts them.
var termRuleCases = []struct {
	name        string
	rules       []profile.TermRule
	source      string
	translation string
	wantFill    bool
}{
	{"a match that breaks a rule goes to drafting", []profile.TermRule{dashboardRule}, "Open the dashboard", "Ouvrir le panneau", false},
	{"a match that keeps the rule is recycled", []profile.TermRule{dashboardRule}, "Open the dashboard", "Ouvrir le tableau de bord", true},
	{"a block with no governing terms is recycled", nil, "Open the dashboard", "Ouvrir le panneau", true},
	{"a source that uses no ruled term is recycled", []profile.TermRule{dashboardRule}, "Hello world", "Bonjour le monde", true},
	{"an accepted rendering is recycled", []profile.TermRule{{Term: "dashboard", Replacement: "tableau de bord", Accepted: []profile.Rendering{{Text: "panneau"}}}}, "Open the dashboard", "Ouvrir le panneau", true},
	{"a declared form of the replacement is recycled", []profile.TermRule{{Term: "dashboard", Replacement: "tableau de bord", ReplacementForms: []string{"tableaux de bord"}}}, "Open the dashboards", "Ouvrir les tableaux de bord", true},
	{"a do-not-translate term kept verbatim is recycled", []profile.TermRule{{Term: "kapi", DoNotTranslate: true}}, "Run kapi", "Lancer kapi", true},
	{"a do-not-translate term that is changed goes to drafting", []profile.TermRule{{Term: "kapi", DoNotTranslate: true}}, "Run kapi", "Lancer capi", false},
	{"a rule that only warns is recycled", []profile.TermRule{{Term: "dashboard", Replacement: "tableau de bord", Advisory: true}}, "Open the dashboard", "Ouvrir le panneau", true},
	{"a placeholder name is not a use of the term", []profile.TermRule{dashboardRule}, "Open {dashboard}", "Ouvrir {dashboard}", true},
}

// TestMemoryLeverageHonoursTermRules_BlockPath: the structure-aware lookup fills
// a target only when the match passes the rules.
func TestMemoryLeverageHonoursTermRules_BlockPath(t *testing.T) {
	t.Parallel()
	for _, tc := range termRuleCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &mockMemoryProvider{exact: map[string]string{tc.source: tc.translation}}
			rb := leverageWithRules(t, provider, tc.rules, model.NewBlock("tu1", tc.source))
			assertFill(t, rb, tc.translation, tc.wantFill)
		})
	}
}

// TestMemoryLeverageHonoursTermRules_TextPath: the flattened fallback, which
// fills on the score alone, is held to the same rules.
func TestMemoryLeverageHonoursTermRules_TextPath(t *testing.T) {
	t.Parallel()
	for _, tc := range termRuleCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &mockBlockMemoryProvider{exact: map[string]string{tc.source: tc.translation}}
			rb := leverageWithRules(t, provider, tc.rules, plainBlock("tu1", tc.source))
			require.Positive(t, provider.calls, "the block path was asked first")
			assertFill(t, rb, tc.translation, tc.wantFill)
		})
	}
}

// dashboardSegSrc is the first segment's source text. The trailing space is
// intentional: the segments concatenate back to the block's source.
const dashboardSegSrc = "Open the dashboard. "

// TestMemoryLeverageHonoursTermRules_SegmentedPath: a target assembled from
// segment matches is held to the rules as a whole block.
func TestMemoryLeverageHonoursTermRules_SegmentedPath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		segment  string
		wantFill bool
	}{
		{"an assembled target that breaks a rule goes to drafting", "Ouvrir le panneau. ", false},
		{"an assembled target that keeps the rule is recycled", "Ouvrir le tableau de bord. ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &mockMemoryProvider{exact: map[string]string{
				dashboardSegSrc: tc.segment,
				"Goodbye.":      "Au revoir.",
			}}
			rb := leverageWithRules(t, provider, []profile.TermRule{dashboardRule}, segBlock("tu1", dashboardSegSrc, "Goodbye."))
			assertFill(t, rb, tc.segment+"Au revoir.", tc.wantFill)
		})
	}
}

func leverageWithRules(t *testing.T, provider corememory.Provider, rules []profile.TermRule, block *model.Block) *model.Block {
	t.Helper()
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
		TermRules:      rules,
	}
	result := processPart(t, tools.NewMemoryLeverageTool(cfg), &model.Part{Type: model.PartBlock, Resource: block})
	return result.Resource.(*model.Block)
}

func assertFill(t *testing.T, rb *model.Block, translation string, wantFill bool) {
	t.Helper()
	_, recorded := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
	assert.True(t, recorded, "the match is recorded whether or not it fills")
	if wantFill {
		assert.Equal(t, translation, rb.TargetText(model.LocaleFrench), "recycled into the target")
		return
	}
	assert.Empty(t, rb.TargetText(model.LocaleFrench), "left for the drafter")
}
