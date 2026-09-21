package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

var kapiDoNotTranslate = terms.Concept{
	ID:             "c-kapi",
	DoNotTranslate: true,
	Terms:          []terms.Term{{Text: "kapi", Locale: model.LocaleEnglish, Status: model.TermPreferred}},
}

// writeDoNotTranslateProject writes a project whose committed terms hold the
// do-not-translate concept kapi, with one English source and its French target.
func writeDoNotTranslateProject(t *testing.T, source, target string) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	writeFixtureFile(t, root, "kapi.yaml", "version: v1\nname: dnt\ndefaults:\n  source_language: en\n  target_languages: [fr]\n"+
		"  terms_source: .kapi/terms.json\ncollections:\n  - name: app\n    path: en/app.json\n    target: \"{lang}/app.json\"\n")
	writeFixtureFile(t, root, "en/app.json", source)
	writeFixtureFile(t, root, "fr/app.json", target)
	writeConceptsBundle(t, filepath.Join(root, project.RelStatePath(ktb.ConventionalName)), []terms.Concept{kapiDoNotTranslate})
	readProjectContext(t, root)
	return root
}

func termsGateOf(t *testing.T, out verifyOutput) verifyGateResult {
	t.Helper()
	g, ok := gateByName(out, gateTerms)
	require.True(t, ok, "a do-not-translate concept schedules the terminology gate: %+v", out.Gates)
	return g
}

// TestShip_TerminologyGateEnforcesDoNotTranslate runs the ship terminology gate
// over a do-not-translate term translated, kept, and used as a placeholder name.
func TestShip_TerminologyGateEnforcesDoNotTranslate(t *testing.T) {
	t.Run("a translated term fails", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"open": "Open kapi to begin."}`, `{"open": "Ouvrez capi pour commencer."}`)
		g := termsGateOf(t, shipCheck(t, root, nil))
		assert.Equal(t, check.VerdictFailed, g.Verdict, "%+v", g)
		require.NotEmpty(t, g.Findings)
		assert.Contains(t, g.Findings[0].Message, `do-not-translate term "kapi"`)
		assert.Equal(t, "fr/app.json", g.Findings[0].File)
	})

	t.Run("a verbatim term passes", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"open": "Open kapi to begin."}`, `{"open": "Ouvrez kapi pour commencer."}`)
		g := termsGateOf(t, shipCheck(t, root, nil))
		assert.Equal(t, check.VerdictPassed, g.Verdict, "%+v", g)
		assert.Empty(t, g.Findings)
		assert.Positive(t, coverageCounts(t, g)["blocks"], "a pass checked content")
	})

	t.Run("a term inside a placeholder is not a use", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"ready": "{kapi} is ready."}`, `{"ready": "{kapi} est prêt."}`)
		g := termsGateOf(t, shipCheck(t, root, nil))
		assert.Equal(t, check.VerdictPassed, g.Verdict, "%+v", g)
		assert.Positive(t, coverageCounts(t, g)["blocks"])
	})

	t.Run("a placeholder name does not keep a term the text uses", func(t *testing.T) {
		root := writeDoNotTranslateProject(t, `{"ready": "{kapi} opens kapi."}`, `{"ready": "{kapi} ouvre capi."}`)
		g := termsGateOf(t, shipCheck(t, root, nil))
		assert.Equal(t, check.VerdictFailed, g.Verdict, "%+v", g)
	})
}
