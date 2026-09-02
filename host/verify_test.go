package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verifyVoiceYAML binds a voice profile with a critical competitor term, so a
// single occurrence in the source drops the compliance score below the default
// threshold (100 - 25 = 75 < 80) and fails the voice gate.
const verifyVoiceYAML = `name: Verify Voice
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
  competitor_terms:
    - term: Globex
      replacement: our platform
      severity: critical
`

// writeVerifyProject creates a temp project that binds a voice profile and a
// project terms store, with an English source file and a French target file. The
// returned root is the project directory; the target file is returned so the
// test can rewrite it for the passing case.
func writeVerifyProject(t *testing.T) (root, targetFile string) {
	t.Helper()
	// Hermetic: neutralise an inherited KAPI_NO_PROJECT (the in-repo dogfood
	// contract encourages devs to set it) so discovery finds the temp project
	// written below. An empty value does NOT disable discovery — only a
	// non-empty KAPI_NO_PROJECT does.
	t.Setenv("KAPI_NO_PROJECT", "")
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: verify
defaults:
  source_language: en
  target_languages: [fr]
  voice:
    profile_file: voice.yaml
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(verifyVoiceYAML), 0o644))

	// Source: contains the competitor term "Globex" (voice fail) and a
	// {name} placeholder plus a ruled term "Save".
	src := `{
  "greeting": "Hello {name}, welcome to Globex!",
  "save": "Save"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"), []byte(src), 0o644))

	// Target: drops the {name} placeholder (a check failure) and mistranslates
	// "Save" (terminology fail — the rule requires "Enregistrer").
	bad := `{
  "greeting": "Bonjour, bienvenue chez Globex!",
  "save": "Sauvegarder"
}
`
	targetFile = filepath.Join(root, "locales", "fr", "app.json")
	require.NoError(t, os.WriteFile(targetFile, []byte(bad), 0o644))

	// Seed the project store's vocabulary: Save -> Enregistrer (approved).
	// Bound now means "the store holds a concept", not "a terms file exists" —
	// every project store has the tables from its first open.
	db, err := projectdb.Open(t.Context(), project.LayoutAt(root))
	require.NoError(t, err)
	require.NoError(t, db.Terms().AddConcept(t.Context(), terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, db.Close())

	return root, targetFile
}

// runVerifyJSON runs `verify --json` against the project rooted at the cwd and
// returns the parsed output plus the RunE error (so the caller can assert the
// quality-gate sentinel and exit code).
func runVerifyJSON(t *testing.T) (verifyOutput, error) {
	t.Helper()
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))

	// Capture stdout (output.Print writes to os.Stdout). The returned error is
	// RunVerify's own return value (the quality-gate sentinel on failure).
	out, runErr := captureStdout(t, func() error {
		return a.RunVerify(cmd, nil)
	})

	var parsed verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "verify must emit valid JSON: %s", out)
	return parsed, runErr
}

// gateByName returns the gate result with the given name (or a zero value with
// found=false).
func gateByName(o verifyOutput, name string) (verifyGateResult, bool) {
	for _, g := range o.Gates {
		if g.Gate == name {
			return g, true
		}
	}
	return verifyGateResult{}, false
}

// TestVerify_FailingProject asserts the failing project produces voice,
// terminology, and qa findings, an overall pass:false, and the quality-gate
// sentinel (exit 3 via ExitCode).
func TestVerify_FailingProject(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	out, runErr := runVerifyJSON(t)

	assert.False(t, out.Pass, "failing project must not pass")
	require.ErrorIs(t, runErr, ErrQualityGate, "must return the quality-gate sentinel")
	assert.Equal(t, ExitGate, ExitCode(nil, runErr), "quality-gate failure must map to exit 3")

	voiceGate, ok := gateByName(out, gateVoice)
	require.True(t, ok, "voice gate must be present")
	assert.False(t, voiceGate.Pass, "voice gate must fail (competitor term Globex)")
	require.NotEmpty(t, voiceGate.Findings, "voice gate must produce findings")

	terms, ok := gateByName(out, gateTerms)
	require.True(t, ok, "terminology gate must be present")
	assert.False(t, terms.Pass, "terminology gate must fail")
	require.NotEmpty(t, terms.Findings)
	assert.Contains(t, terms.Findings[0].Message, "Enregistrer")

	qa, ok := gateByName(out, gateQA)
	require.True(t, ok, "qa gate must be present")
	assert.False(t, qa.Pass, "qa gate must fail (dropped placeholder)")
	require.NotEmpty(t, qa.Findings)

	// Summary is internally consistent.
	assert.Equal(t, len(out.Gates), out.Summary.Gates)
	assert.Positive(t, out.Summary.Failed)
}

// TestVerify_NoFailReportsButExitsZero asserts that --no-fail keeps the verdict in
// the output (pass:false with findings) while exiting 0 — report mode for an assistant
// loop, where a not-yet-passing gate is expected feedback, not a failure.
func TestVerify_NoFailReportsButExitsZero(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.Flags().Set("no-fail", "true"))

	out, runErr := captureStdout(t, func() error { return a.RunVerify(cmd, nil) })

	require.NoError(t, runErr, "--no-fail must not return the gate sentinel (exit 0)")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr), "--no-fail maps to exit 0 even on gate failure")

	var parsed verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.False(t, parsed.Pass, "the verdict (pass:false) is still reported in the output")
	assert.Positive(t, parsed.Summary.Failed, "findings are still reported")
}

// TestVerify_PassingAfterFix asserts that fixing the voice, terminology, and
// placeholder violations makes verify pass with a zero exit code.
func TestVerify_PassingAfterFix(t *testing.T) {
	root, targetFile := writeVerifyProject(t)

	// Fix the source: remove the competitor term, keep the placeholder.
	goodSrc := `{
  "greeting": "Hello {name}, welcome!",
  "save": "Save"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"), []byte(goodSrc), 0o644))

	// Fix the target: keep the placeholder, use the approved term.
	good := `{
  "greeting": "Bonjour {name}, bienvenue!",
  "save": "Enregistrer"
}
`
	require.NoError(t, os.WriteFile(targetFile, []byte(good), 0o644))

	t.Chdir(root)
	out, runErr := runVerifyJSON(t)

	assert.True(t, out.Pass, "fixed project must pass: %+v", out)
	require.NoError(t, runErr, "passing run must return no error")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr), "pass must map to exit 0")

	for _, g := range out.Gates {
		assert.True(t, g.Pass, "gate %q must pass", g.Gate)
		assert.Empty(t, g.Findings, "gate %q must have no findings", g.Gate)
	}
}

// TestVerify_NoProject asserts that running verify outside any project returns
// an operational error (exit 1), not a quality-gate failure.
func TestVerify_NoProject(t *testing.T) {
	// Hermetic: the "no project" result must come from the empty temp dir, not
	// an inherited KAPI_NO_PROJECT.
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Chdir(t.TempDir())

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	err := a.RunVerify(cmd, nil)

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrQualityGate, "no-project is operational, not a gate failure")
	assert.Equal(t, ExitError, ExitCode(nil, err), "operational error must map to exit 1")
	assert.Contains(t, err.Error(), "no kapi project")
}

// TestVerify_GateSelection asserts that naming one gate runs only that gate.
func TestVerify_GateSelection(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.Flags().Set(gateFlagName, gateTerms))

	out, err := captureStdout(t, func() error { return a.RunVerify(cmd, nil) })
	// The failing project's terminology gate fails, so verify returns the
	// quality-gate sentinel — the point of this test is that ONLY that gate ran.
	require.ErrorIs(t, err, ErrQualityGate)

	var parsed verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))

	require.Len(t, parsed.Gates, 1, "only the terminology gate should run")
	assert.Equal(t, gateTerms, parsed.Gates[0].Gate)
}

// writeTermsSourceProject creates a project that binds a committed
// termbase_source (a .terms.json) and whose own store holds no concepts, so the
// check path must resolve the vocabulary straight from the source. The terms store carries
// two concepts: a do-not-translate brand term (KapiMart, identical in en/fr) and
// a translated term (Save -> Enregistrer). The fr target keeps the brand term
// identical (correct) and mistranslates "Save" as "Sauvegarder" (a terms fail).
func writeTermsSourceProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: verifysrc
defaults:
  source_language: en
  target_languages: [fr]
  terms_source: .kapi/terms.json
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))

	src := `{
  "brand": "KapiMart",
  "save": "Save"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"), []byte(src), 0o644))

	target := `{
  "brand": "KapiMart",
  "save": "Sauvegarder"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr", "app.json"), []byte(target), 0o644))

	file := ktb.FromConcepts([]terms.Concept{
		{
			ID: "brand",
			Terms: []terms.Term{
				{Text: "KapiMart", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "KapiMart", Locale: model.LocaleFrench, Status: model.TermPreferred},
			},
		},
		{
			ID: "save",
			Terms: []terms.Term{
				{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
			},
		},
	})
	data, err := ktb.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RelStatePath("terms.json")), data, 0o644))

	// Assert the fallback path is the one under test: no project store exists
	// yet, so nothing could have been compiled into it. This is the fresh-checkout
	// shape — the committed source is tracked, the store is not.
	_, statErr := os.Stat(project.LayoutAt(root).StorePath())
	require.True(t, os.IsNotExist(statErr), "test must exercise the terms_source fallback, not a compiled store")

	return root
}

// TestVerify_PreferredTermsFromTermsSource asserts the terminology gate resolves
// the project vocabulary directly from the committed termbase_source
// (.terms.json) when the project store holds no concepts — the common case at
// check time in CI, where the gitignored store does not exist at all. The gate must run and fail on the mistranslated "Save".
func TestVerify_PreferredTermsFromTermsSource(t *testing.T) {
	root := writeTermsSourceProject(t)
	t.Chdir(root)

	out, runErr := runVerifyJSON(t)

	require.ErrorIs(t, runErr, ErrQualityGate, "the mistranslated term must fail the gate")

	terms, ok := gateByName(out, gateTerms)
	require.True(t, ok, "terminology gate must run when termbase_source is bound")
	assert.False(t, terms.Pass, "terminology gate must fail on Save -> Sauvegarder")
	require.NotEmpty(t, terms.Findings)
	assert.Contains(t, terms.Findings[0].Message, "Enregistrer",
		"the finding must name the preferred term resolved from the .terms.json source")
}

// TestVerify_PreferredTermsFromTermsSourceWithAnEmptyStorePresent is the case the
// merged store introduced, and the one a stat-based gate gets wrong.
//
// A fresh checkout has the committed `.terms.json` and no store. The first kapi
// command run there — any command — opens the project store, which CREATES it
// with every subsystem's tables in place and none of them populated. From that
// moment a file-presence test says "there is a compiled vocabulary here", and a
// gate that trusted it would read an empty store and pass a project whose
// committed terms are being violated.
//
// So the rule is: committed source wins while the store's vocabulary tables are
// empty. Same project, same expected failure, with the store sitting there.
func TestVerify_PreferredTermsFromTermsSourceWithAnEmptyStorePresent(t *testing.T) {
	root := writeTermsSourceProject(t)

	// Exactly what a first command in a fresh checkout leaves behind.
	db := openProjectStore(t, root)
	has, err := db.HasTerms(t.Context())
	require.NoError(t, err)
	require.False(t, has, "the store must be present and empty — that is the whole point")
	require.NoError(t, db.Close())
	require.FileExists(t, project.LayoutAt(root).StorePath())

	t.Chdir(root)
	out, runErr := runVerifyJSON(t)

	require.ErrorIs(t, runErr, ErrQualityGate,
		"an empty store must not shadow the committed source and pass a violating project")

	terms, ok := gateByName(out, gateTerms)
	require.True(t, ok, "terminology gate must still run")
	assert.False(t, terms.Pass)
	require.NotEmpty(t, terms.Findings)
	assert.Contains(t, terms.Findings[0].Message, "Enregistrer")
}

// TestVerify_DoNotTranslateNotFlagged asserts that a do-not-translate
// term whose target legitimately equals the source (e.g. the brand "KapiMart")
// is not reported as untranslated by the checks gate.
func TestVerify_DoNotTranslateNotFlagged(t *testing.T) {
	root := writeTermsSourceProject(t)
	t.Chdir(root)

	out, _ := runVerifyJSON(t)

	qa, ok := gateByName(out, gateQA)
	require.True(t, ok, "qa gate must be present")
	for _, f := range qa.Findings {
		assert.NotContains(t, f.Message, "identical to source",
			"the do-not-translate brand term must not be flagged as untranslated")
	}
}

// writeTermsProjectUnbound builds the same project as writeTermsSourceProject
// but with NO defaults.terms_source binding, committing the terms at the
// conventional location rel instead.
func writeTermsProjectUnbound(t *testing.T, rel string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: verifyconv
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NotContains(t, recipe, "termbase_source",
		"this test must exercise the convention ladder, not a binding")

	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte("{\n  \"save\": \"Save\"\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr", "app.json"),
		[]byte("{\n  \"save\": \"Sauvegarder\"\n}\n"), 0o644))

	file := ktb.FromConcepts([]terms.Concept{{
		ID: "save",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}})
	data, err := ktb.Marshal(file)
	require.NoError(t, err)
	dest := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	require.NoError(t, os.WriteFile(dest, data, 0o644))

	return root
}

// TestVerify_PreferredTermsFromConventionalLocation covers the well-known-location
// ladder for terms: with no defaults.terms_source binding, a terms file
// committed at either conventional path is still found and still gates.
func TestVerify_PreferredTermsFromConventionalLocation(t *testing.T) {
	tests := []struct {
		name string
		rel  string
	}{
		{"context directory", project.RelStatePath("terms.json")},
		{"repository root", "terms.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTermsProjectUnbound(t, tt.rel)
			t.Chdir(root)

			_, runErr := runVerifyJSON(t)
			require.ErrorIs(t, runErr, ErrQualityGate,
				"an unbound project must find the conventional terms and fail on the mistranslated term")
		})
	}
}

// TestVerify_ConventionalPreferredTermsPreferContextDir pins the ordering when both
// rungs are present: `.kapi/` wins. It is committed and it is where a
// project's authored sources live, so the conventional home and the reviewed
// home are the same directory.
func TestVerify_ConventionalPreferredTermsPreferContextDir(t *testing.T) {
	root := writeTermsProjectUnbound(t, project.RelStatePath("terms.json"))

	// A root terms file that would PASS the gate. If the ladder still preferred
	// the root, the run would come back clean and this test would fail.
	stray := ktb.FromConcepts([]terms.Concept{{
		ID: "save",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Sauvegarder", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}})
	data, err := ktb.Marshal(stray)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "terms.json"), data, 0o644))

	t.Chdir(root)
	_, runErr := runVerifyJSON(t)
	require.ErrorIs(t, runErr, ErrQualityGate,
		"the context-directory terms must win over the one at the root")
}
