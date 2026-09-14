package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// termCheck runs term-check over one block and returns whether it passed and
// the failing messages.
func termCheck(t *testing.T, cfg *tools.TermCheckConfig, source, target string) (bool, string) {
	t.Helper()
	block := model.NewBlock("tu1", source)
	block.SetTargetText(cfg.TargetLocale, target)
	result := processPart(t, tools.NewTermCheckTool(cfg), &model.Part{Type: model.PartBlock, Resource: block})
	b := result.Resource.(*model.Block)
	passed, ok := b.Properties[tools.PropTermCheckPassed]
	require.True(t, ok, "term-check did not run on %q", source)
	return passed == "true", b.Properties[tools.PropTermCheckErrors]
}

func nb(rules ...coreprofile.TermRule) *tools.TermCheckConfig {
	return &tools.TermCheckConfig{TermRules: rules, SourceLocale: "en-GB", TargetLocale: "nb"}
}

// TestTermCheck_EnglishSourceFindsInflections covers the under-demand in #2672:
// a rule on "alert" never fired on the navigation label "Alerts".
func TestTermCheck_EnglishSourceFindsInflections(t *testing.T) {
	alert := coreprofile.TermRule{Term: "alert", Replacement: "varsel"}

	passed, msg := termCheck(t, nb(alert), "Alerts", "Oversikt")
	assert.False(t, passed, "the English plural uses the term")
	assert.Contains(t, msg, `required translation "varsel" missing in target (no forms declared in nb)`)

	passed, _ = termCheck(t, nb(alert), "Two alerts arrived", "To varsler kom")
	assert.False(t, passed, "without a declared form the Norwegian plural is not the term")
}

// TestTermCheck_EnglishSourceStopsBeforeDerivation covers the over-demand the
// research measured: these source words are other terms, or compounds.
func TestTermCheck_EnglishSourceStopsBeforeDerivation(t *testing.T) {
	cases := []struct {
		rule   coreprofile.TermRule
		source string
	}{
		{coreprofile.TermRule{Term: "flow", Replacement: "flyt"}, "Define a workflow"},
		{coreprofile.TermRule{Term: "translate", Replacement: "oversett"}, "Run pseudo-translate first"},
		{coreprofile.TermRule{Term: "extract", Replacement: "trekk ut"}, "Run the extraction"},
		{coreprofile.TermRule{Term: "set", Replacement: "angi"}, "Open the settings"},
	}
	for _, tc := range cases {
		passed, msg := termCheck(t, nb(tc.rule), tc.source, "Ingen treff")
		assert.True(t, passed, "%q must not demand %q: %s", tc.source, tc.rule.Term, msg)
	}
}

// TestTermCheck_LongestDeclaredTermWins: a project that declared "berth plan"
// has said those words are not a use of "berth", so the reviewed "Kaiplan" is
// held to the longer concept alone.
func TestTermCheck_LongestDeclaredTermWins(t *testing.T) {
	berth := coreprofile.TermRule{Term: "berth", Replacement: "kaiplass"}
	berthPlan := coreprofile.TermRule{Term: "berth plan", Replacement: "kaiplan"}

	passed, msg := termCheck(t, nb(berth, berthPlan), "Berth plan", "Kaiplan")
	assert.True(t, passed, msg)

	passed, msg = termCheck(t, nb(berth, berthPlan), "Book a berth from the berth plan", "Book fra kaiplanen")
	assert.False(t, passed, "the berth outside the plan is still a use of berth")
	assert.Contains(t, msg, `"kaiplass"`)
}

// TestTermCheck_PlaceholderNamesAreNotTerms keeps #2674 fixed under the new
// matcher.
func TestTermCheck_PlaceholderNamesAreNotTerms(t *testing.T) {
	vessel := coreprofile.TermRule{Term: "vessel", Replacement: "fartøy"}
	passed, msg := termCheck(t, nb(vessel), "{vessel} is alongside until {until}.", "{vessel} ligger til kai til {until}.")
	assert.True(t, passed, msg)
}

// TestTermCheck_AcceptedRenderings: a concept's admitted term satisfies the
// rule, and a failure names the concept and every rendering it accepts.
func TestTermCheck_AcceptedRenderings(t *testing.T) {
	vessel := coreprofile.TermRule{
		Term: "vessel", Replacement: "fartøy", ConceptID: "term:vessel",
		Accepted: []coreprofile.Rendering{{Text: "skip", Forms: []string{"skipet"}}},
	}
	passed, msg := termCheck(t, nb(vessel), "Every vessel", "Hvert skip")
	assert.True(t, passed, msg)
	passed, msg = termCheck(t, nb(vessel), "The vessel", "Skipet")
	assert.True(t, passed, "a declared form of an accepted rendering satisfies the rule: %s", msg)

	passed, msg = termCheck(t, nb(vessel), "The vessel", "Båten")
	assert.False(t, passed)
	assert.Equal(t, `term "vessel" found in source but required translation "fartøy" missing in target (concept term:vessel, also accepted: "skip")`, msg)
}

// TestTermCheck_NonEnglishSourceUsesDeclaredForms: without a built-in rule for
// the language, a source term is found as a whole word or as a form it declares.
func TestTermCheck_NonEnglishSourceUsesDeclaredForms(t *testing.T) {
	cfg := &tools.TermCheckConfig{SourceLocale: "nb", TargetLocale: "en", TermRules: []coreprofile.TermRule{
		{Term: "varsel", Replacement: "alert"},
	}}
	passed, _ := termCheck(t, cfg, "Lese varsler", "Read notices")
	assert.True(t, passed, "an undeclared Norwegian plural is not demanded")

	cfg.TermRules[0].Forms = []string{"varsler"}
	passed, _ = termCheck(t, cfg, "Lese varsler", "Read notices")
	assert.False(t, passed, "a declared source form demands the rule")
}

// TestTermCheck_Canaries are the decision-2 fixtures for the terminology gate.
// The must-pass pair holds once the forms are declared; the must-fail pair
// holds whatever is declared. Containment is kept on purpose, so a different
// word that contains a rendering also passes.
func TestTermCheck_Canaries(t *testing.T) {
	alert := coreprofile.TermRule{Term: "alert", Replacement: "varsel", ReplacementForms: []string{"varsler", "varselet", "varslene"}}
	berth := coreprofile.TermRule{Term: "berth", Replacement: "kaiplass", ReplacementForms: []string{"kaiplassen", "kaiplasser", "kaiplassene"}}
	deBerth := &tools.TermCheckConfig{SourceLocale: "en-GB", TargetLocale: "de", TermRules: []coreprofile.TermRule{
		{Term: "berth", Replacement: "Liegeplatz", ReplacementForms: []string{"Liegeplatzes", "Liegeplätze", "Liegeplätzen"}},
	}}

	t.Run("must pass: Varsler for varsel", func(t *testing.T) {
		passed, msg := termCheck(t, nb(alert), "Alerts", "Varsler")
		assert.True(t, passed, msg)
	})
	t.Run("must pass: Liegeplätze for Liegeplatz", func(t *testing.T) {
		passed, msg := termCheck(t, deBerth, "Every berth at this terminal", "Alle Liegeplätze an diesem Terminal")
		assert.True(t, passed, msg)
	})
	t.Run("must fail: Kaiplan for kaiplass", func(t *testing.T) {
		passed, _ := termCheck(t, nb(berth), "Berth plan", "Kaiplan")
		assert.False(t, passed)
	})
	t.Run("must fail: the term deleted", func(t *testing.T) {
		passed, _ := termCheck(t, nb(berth), "Book a berth", "Bestill nå")
		assert.False(t, passed)
	})
	t.Run("known limitation: a different word containing the rendering passes", func(t *testing.T) {
		plugin := coreprofile.TermRule{Term: "plugin", Replacement: "tillegg"}
		passed, _ := termCheck(t, nb(plugin), "Plugin requirements", "Tilleggskrav")
		assert.True(t, passed)
	})
}

func TestTermCheckCanaries_FlagBothShapes(t *testing.T) {
	cfg := nb(coreprofile.TermRule{Term: "berth", Replacement: "kaiplass", ReplacementForms: []string{"kaiplasser"}})
	canaries, uncheckable := tools.TermCheckCanaries(cfg)
	require.Empty(t, uncheckable)
	require.Len(t, canaries, 2)
	assert.Equal(t, `term "berth" with "kaiplan" in place of "kaiplass"`, canaries[1].Name)

	tc := tools.NewTermCheckTool(cfg)
	outcome, err := check.Probe(canaries, uncheckable, func(b *model.Block) ([]check.Finding, error) {
		if err := tc.Annotate(tool.NewBlockView(b)); err != nil {
			return nil, err
		}
		if b.Properties[tools.PropTermCheckPassed] == "false" {
			return []check.Finding{{Category: "terminology", Message: b.Properties[tools.PropTermCheckErrors]}}, nil
		}
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, check.CanaryCaught, outcome.Status)
}

func TestTermCheckMatching(t *testing.T) {
	cfg := nb(
		coreprofile.TermRule{Term: "alert", Replacement: "varsel", ReplacementForms: []string{"varsler"}},
		coreprofile.TermRule{Term: "berth", Replacement: "kaiplass"},
		coreprofile.TermRule{Term: "Compass", DoNotTranslate: true},
	)
	assert.Equal(t, check.TermMatching{
		Locale: "nb", Source: check.TermSourceEnglishInflection, Target: check.TermTargetContainmentForms,
		Rules: 3, RulesWithForms: 1,
	}, tools.TermCheckMatching(cfg))

	cfg.SourceLocale = "de"
	cfg.TermRules = cfg.TermRules[1:]
	assert.Equal(t, check.TermMatching{
		Locale: "nb", Source: check.TermSourceWholeWord, Target: check.TermTargetContainment, Rules: 2,
	}, tools.TermCheckMatching(cfg))
}
