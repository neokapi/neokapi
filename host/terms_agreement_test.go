package host

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

var agreementCollections = []string{"translated", "kept", "placeholder"}

// writeTermsAgreementProject writes one French project with three collections
// held to the same committed terms: the do-not-translate concept kapi and the
// concept save, rendered Enregistrer. "translated" renders kapi, "kept" keeps
// it, and "placeholder" uses it only as a placeholder name.
func writeTermsAgreementProject(t *testing.T) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	var recipe strings.Builder
	recipe.WriteString("version: v1\nname: agreement\ndefaults:\n  source_language: en\n  target_languages: [fr]\ncollections:\n")
	for _, name := range agreementCollections {
		recipe.WriteString("  - name: " + name + "\n    path: en/" + name + ".json\n    target: \"{lang}/" + name + ".json\"\n")
	}
	recipe.WriteString("ship_gate: { translated: 0 }\n")
	writeFixtureFile(t, root, "kapi.yaml", recipe.String())

	writeFixtureFile(t, root, "en/translated.json", `{"open": "Open kapi and save."}`)
	writeFixtureFile(t, root, "fr/translated.json", `{"open": "Ouvrez capi et Enregistrer."}`)
	writeFixtureFile(t, root, "en/kept.json", `{"open": "Open kapi and save."}`)
	writeFixtureFile(t, root, "fr/kept.json", `{"open": "Ouvrez kapi et Enregistrer."}`)
	writeFixtureFile(t, root, "en/placeholder.json", `{"ready": "{kapi} is ready to save."}`)
	writeFixtureFile(t, root, "fr/placeholder.json", `{"ready": "{kapi} est prêt à Enregistrer."}`)

	writeConceptsBundle(t, filepath.Join(root, project.RelStatePath(ktb.ConventionalName)), []terms.Concept{
		kapiDoNotTranslate,
		{ID: "c-save", Terms: []terms.Term{
			{Text: "save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		}},
	})
	readProjectContext(t, root)
	return root
}

// bilingualTermsCheck runs `kapi check <source> --target <target>` in the project
// for one collection.
func bilingualTermsCheck(t *testing.T, root, name string) check.Report {
	t.Helper()
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	cmd.Flags().String("target", filepath.Join(root, "fr", name+".json"), "")
	cmd.Flags().String("target-lang", "fr", "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "en", name+".json")})
	require.NoError(t, err)
	return report
}

func collectionName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// TestTermsSurfacesAgreeOnDoNotTranslate runs every surface that holds content
// to the project's terms over one project, and asserts they fail the same
// content: the ship terminology gate, kapi check with the project's terms, the
// loop checks, kapi status and ship.json.
func TestTermsSurfacesAgreeOnDoNotTranslate(t *testing.T) {
	root := writeTermsAgreementProject(t)
	want := []string{"translated"}

	var gate []string
	for _, f := range termsGateOf(t, shipCheck(t, root, nil)).Findings {
		gate = append(gate, collectionName(f.File))
	}

	var checked []string
	for _, name := range agreementCollections {
		report := bilingualTermsCheck(t, root, name)
		require.NotNil(t, report.Execution)
		ran := false
		for _, run := range report.Execution.Analyzers {
			if run.ID == "terms" {
				ran = run.Canary != nil && run.Canary.Status == check.CanaryCaught
			}
		}
		assert.True(t, ran, "%s: kapi check ran the terms analyzer with a caught canary", name)
		for _, d := range report.Findings {
			if d.Check == "terms" {
				checked = append(checked, name)
				assert.True(t, d.Fails, "%s: %s", name, d.Message)
			}
		}
		assert.Equal(t, name == "translated", report.Verdict == check.VerdictFailed, "%s: kapi check verdict %s", name, report.Verdict)
	}

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	proj, err := project.LoadWithOptions(filepath.Join(root, "kapi.yaml"), project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.Len(t, units, 3)
	excl, err := a.computeLoopCheckExclusions(t.Context(), sourceShipCommand(t, root), proj, root, units)
	require.NoError(t, err)
	var looped []string
	for key, failing := range excl.Failing {
		if failing {
			looped = append(looped, collectionName(strings.SplitN(key, "\x00", 2)[0]))
		}
	}

	status := profileTermsStatus(t, root)
	var statused []string
	for _, lc := range status.Locales {
		if lc.FailingChecks > 0 {
			statused = append(statused, lc.Collection)
		}
		assert.Equal(t, lc.Collection != "translated", lc.Shippable, "status: %s", lc.Collection)
	}

	assert.Equal(t, want, sortedUnique(gate), "the ship terminology gate")
	assert.Equal(t, want, sortedUnique(checked), "kapi check with the project's terms")
	assert.Equal(t, want, sortedUnique(looped), "the loop checks")
	assert.Equal(t, want, sortedUnique(statused), "kapi status")
	assert.False(t, profileTermsManifest(t, root)["fr"].Shippable, "ship.json withholds fr on the translated term")
}
