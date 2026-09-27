package host

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// recipeRulesRecipe declares French term rules on the steps of the project's
// default flow, as the Voltway demo does, and binds no terms anywhere else.
// "billing period" fails when missed; "reading" is advisory.
const recipeRulesRecipe = `version: v1
name: reciperules
defaults:
  source_language: en
  target_languages: [fr]
  flow: converge
collections:
  - name: app
    path: app/en.json
    target: "app/{lang}.json"
ship_gate: { translated: 0 }
flows:
  converge:
    steps:
      - tool: translate
        config:
          provider: demo
          term_rules: &rules
            - term: billing period
              replacement: période de facturation
            - term: reading
              replacement: relevé
              advisory: true
      - tool: term-check
        config:
          term_rules: *rules
`

const recipeRulesSource = `{"period": "Your billing period ends today.", "meter": "Meter reading"}`

// writeRecipeRulesProject writes the project with the given French target and
// returns its root.
func writeRecipeRulesProject(t *testing.T, recipe, target string) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	writeFixtureFile(t, root, project.RecipeFileName, recipe)
	writeFixtureFile(t, root, "app/en.json", recipeRulesSource)
	writeFixtureFile(t, root, "app/fr.json", target)
	return root
}

// recipeRulesUpCoverage is the coverage `kapi up` derives after a pass, with
// the loop checks it runs over the produced units.
func recipeRulesUpCoverage(t *testing.T, root string) []LocaleCoverage {
	t.Helper()
	recipe := filepath.Join(root, project.RecipeFileName)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	cmd := NewEnvCommand(t.Context(), "up")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cov, _, err := a.deriveCoverage(t.Context(), cmd, proj, root, true, nil)
	require.NoError(t, err)
	return cov
}

// recipeRulesFileCheck runs `kapi check app/en.json --target app/fr.json
// --target-lang fr` in the project.
func recipeRulesFileCheck(t *testing.T, root string) check.Report {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "check")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, project.RecipeFileName)))
	cmd.Flags().String("target", filepath.Join(root, "app", "fr.json"), "")
	cmd.Flags().String("target-lang", "fr", "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "app", "en.json")})
	require.NoError(t, err)
	return report
}

func termFindings(r check.Report) []check.Diagnostic {
	var out []check.Diagnostic
	for _, f := range r.Findings {
		if f.Check == "terms" && strings.Contains(f.Rule, "terminology") {
			out = append(out, f)
		}
	}
	return out
}

// A locale governed only by term rules on a flow step counts as governed, and
// status, `kapi up`, `kapi check --ship`, `--gate terms` and `kapi check` with
// a target agree about each rule: a miss of a failing rule fails, a miss of an
// advisory one reports, and a target that follows both passes.
func TestRecipeTermRules_GovernTheTermsGate(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		fails   bool
		reports int
	}{
		{
			name:   "follows both rules",
			target: `{"period": "Votre période de facturation se termine aujourd'hui.", "meter": "Relevé du compteur"}`,
		},
		{
			name:    "misses the failing rule",
			target:  `{"period": "Votre période de paiement se termine aujourd'hui.", "meter": "Relevé du compteur"}`,
			fails:   true,
			reports: 1,
		},
		{
			name:    "misses the advisory rule",
			target:  `{"period": "Votre période de facturation se termine aujourd'hui.", "meter": "Lecture du compteur"}`,
			reports: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRecipeRulesProject(t, recipeRulesRecipe, tc.target)

			// kapi check --ship
			out := shipCheck(t, root, nil)
			g, ok := gateByName(out, gateTerms)
			require.True(t, ok, "the recipe's rules schedule the terminology gate: %+v", out.Gates)
			assert.Equal(t, 1, coverageCounts(t, g)["files"])
			assert.Zero(t, coverageCounts(t, g)["not_governed"])
			require.Len(t, g.Findings, tc.reports, "%+v", g.Findings)
			if tc.fails {
				assert.Equal(t, check.VerdictFailed, g.Verdict)
				assert.True(t, g.Findings[0].Fails)
				assert.Contains(t, g.Findings[0].Message, "période de facturation")
			} else {
				assert.Equal(t, check.VerdictPassed, g.Verdict, "%+v", g)
				for _, f := range g.Findings {
					assert.False(t, f.Fails, "an advisory rule reports without failing: %+v", f)
				}
			}

			// --gate terms, named on its own
			named := shipCheck(t, root, map[string]string{gateFlagName: gateTerms})
			ng, ok := gateByName(named, gateTerms)
			require.True(t, ok)
			assert.Equal(t, g.Verdict, ng.Verdict)
			assert.Len(t, ng.Findings, tc.reports)

			// kapi status and ship.json
			status := profileTermsStatus(t, root)
			fr := scopeCoverage(t, status, "app", "fr")
			assert.Empty(t, fr.NotGoverned, "the recipe's rules govern French")
			assert.Equal(t, map[bool]int{true: 1, false: 0}[tc.fails], fr.FailingChecks)
			assert.Empty(t, profileTermsManifest(t, root)["fr"].NotGoverned)
			var text bytes.Buffer
			require.NoError(t, status.FormatText(&text))
			assert.NotContains(t, text.String(), "No terms govern")

			// kapi up
			up := recipeRulesUpCoverage(t, root)
			require.Len(t, up, 1)
			assert.Equal(t, fr.FailingChecks, up[0].FailingChecks, "up and status agree")
			assert.Equal(t, fr.NotGoverned, up[0].NotGoverned)
			assert.Equal(t, fr.Shippable, up[0].Shippable)

			// kapi check with a target
			report := recipeRulesFileCheck(t, root)
			tf := termFindings(report)
			assert.Len(t, tf, tc.reports, "%+v", report.Findings)
			assert.Equal(t, tc.fails, report.Summary.Failing > 0, "%+v", report.Summary)
		})
	}
}

// The review point a reviewer reads lists the rules the gate holds the target
// to, the recipe's among them.
func TestRecipeTermRules_ReachTheReviewPoint(t *testing.T) {
	root := writeRecipeRulesProject(t, recipeRulesRecipe, `{"period": "x", "meter": "y"}`)
	cmd := NewEnvCommand(t.Context(), "status")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, project.RecipeFileName)))
	point := (&App{SourceLang: "en"}).NewReviewPointResolver(cmd, root).At(t.Context(), "app", filepath.Join(root, "app", "en.json"), "fr")
	var terms []string
	for _, r := range point.TermRules {
		terms = append(terms, r.Term+"→"+r.Replacement)
	}
	assert.ElementsMatch(t, []string{"billing period→période de facturation", "reading→relevé"}, terms)
}

// A rule declared for one language governs that language alone. French is
// then ungoverned, the explicit gate says where terms come from, and status
// says neither source has a rule for French.
func TestRecipeTermRules_LocaleRulesGovernTheirLanguage(t *testing.T) {
	recipe := `version: v1
name: reciperules
defaults:
  source_language: en
  target_languages: [fr]
  locales:
    de:
      tools:
        term-check:
          term_rules:
            - term: billing period
              replacement: Abrechnungszeitraum
collections:
  - name: app
    path: app/en.json
    target: "app/{lang}.json"
ship_gate: { translated: 0 }
`
	root := writeRecipeRulesProject(t, recipe, `{"period": "Votre période de paiement.", "meter": "Lecture"}`)

	out := shipCheck(t, root, nil)
	_, ok := gateByName(out, gateTerms)
	assert.False(t, ok, "rules for German leave French with no terminology gate: %+v", out.Gates)

	named := shipCheck(t, root, map[string]string{gateFlagName: gateTerms})
	g, ok := gateByName(named, gateTerms)
	require.True(t, ok)
	assert.Equal(t, check.VerdictDidNotRun, g.Verdict)
	require.NotEmpty(t, g.Findings)
	assert.Contains(t, g.Findings[0].Message, "the recipe's term_rules for de")

	status := profileTermsStatus(t, root)
	assert.Equal(t, []string{"terms"}, scopeCoverage(t, status, "app", "fr").NotGoverned)
	var text bytes.Buffer
	require.NoError(t, status.FormatText(&text))
	assert.Contains(t, text.String(), "No terms govern fr: neither the terms bound where its content sits nor the recipe's "+
		"term_rules have a rule for it, so terminology is not a bar to shipping there.")
}

// declaredTermRulesWhere names the recipe's rules for the gate's message.
func TestDeclaredTermRulesWhere(t *testing.T) {
	assert.Empty(t, declaredTermRulesWhere(coreprofile.RecipeTermRules{}))
	assert.Equal(t, "the recipe's term_rules", declaredTermRulesWhere(coreprofile.RecipeTermRules{
		All: []coreprofile.TermRule{{Term: "a", Replacement: "b"}},
	}))
	assert.Equal(t, "the recipe's term_rules for de, nb", declaredTermRulesWhere(coreprofile.RecipeTermRules{
		ByLocale: map[string][]coreprofile.TermRule{
			"nb": {{Term: "a", Replacement: "b"}},
			"de": {{Term: "a", Replacement: "c"}},
		},
	}))
}
