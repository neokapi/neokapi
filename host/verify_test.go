package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verifyBrandYAML binds a brand voice with a critical competitor term, so a
// single occurrence in the source drops the compliance score below the default
// threshold (100 - 25 = 75 < 80) and fails the brand gate.
const verifyBrandYAML = `name: Verify Brand
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

// writeVerifyProject creates a temp project that binds a brand profile and a
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
  brand_voice:
    profile_file: brand.yaml
content:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(verifyBrandYAML), 0o644))

	// Source: contains the competitor term "Globex" (brand fail) and a
	// {name} placeholder plus a glossary term "Save".
	src := `{
  "greeting": "Hello {name}, welcome to Globex!",
  "save": "Save"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"), []byte(src), 0o644))

	// Target: drops the {name} placeholder (QA fail) and mistranslates "Save"
	// (terminology fail — glossary requires "Enregistrer").
	bad := `{
  "greeting": "Bonjour, bienvenue chez Globex!",
  "save": "Sauvegarder"
}
`
	targetFile = filepath.Join(root, "locales", "fr", "app.json")
	require.NoError(t, os.WriteFile(targetFile, []byte(bad), 0o644))

	// Seed the project terms store: Save -> Enregistrer (approved).
	tbPath := filepath.Join(root, ".kapi", "terms.db")
	tb, err := terms.NewSQLiteStore(tbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.Close())

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

// TestVerify_FailingProject asserts the failing project produces brand,
// terminology, and qa findings, an overall pass:false, and the quality-gate
// sentinel (exit 3 via ExitCode).
func TestVerify_FailingProject(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	out, runErr := runVerifyJSON(t)

	assert.False(t, out.Pass, "failing project must not pass")
	require.ErrorIs(t, runErr, ErrQualityGate, "must return the quality-gate sentinel")
	assert.Equal(t, ExitGate, ExitCode(nil, runErr), "quality-gate failure must map to exit 3")

	brand, ok := gateByName(out, gateBrand)
	require.True(t, ok, "brand gate must be present")
	assert.False(t, profile.Pass, "brand gate must fail (competitor term Globex)")
	require.NotEmpty(t, profile.Findings, "brand gate must produce findings")

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

// TestVerify_PassingAfterFix asserts that fixing the brand, terminology, and
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

// TestVerify_GateSelection asserts that --terms runs only the terminology gate.
func TestVerify_GateSelection(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.Flags().Set("terms", "true"))

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
// termbase_source (a .terms.json) but NO compiled .kapi/terms.db, so the check
// path must resolve the glossary straight from the source. The terms store carries
// two concepts: a do-not-translate brand term (KapiMart, identical in en/fr) and
// a translated term (Save -> Enregistrer). The fr target keeps the brand term
// identical (correct) and mistranslates "Save" as "Sauvegarder" (a terms fail).
func writeTermsSourceProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "l10n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: verifysrc
defaults:
  source_language: en
  target_languages: [fr]
  terms_source: context/terms.json
content:
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
	require.NoError(t, os.WriteFile(filepath.Join(root, "l10n", "terms.json"), data, 0o644))

	// Assert the fallback path is the one under test: no compiled db exists.
	_, statErr := os.Stat(filepath.Join(root, ".kapi", "terms.db"))
	require.True(t, os.IsNotExist(statErr), "test must exercise the termbase_source fallback, not a compiled db")

	return root
}

// TestVerify_GlossaryFromTermsSource asserts the terminology gate resolves
// the project glossary directly from the committed termbase_source (.terms.json) when
// the compiled .kapi/terms.db is absent — the common case at check time in
// CI. The gate must run and fail on the mistranslated "Save".
func TestVerify_GlossaryFromTermsSource(t *testing.T) {
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

// TestVerify_DoNotTranslateNotFlagged asserts that a do-not-translate glossary
// term whose target legitimately equals the source (e.g. the brand "KapiMart")
// is not reported as untranslated by the QA gate.
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
// but with NO defaults.termbase_source binding, committing the glossary at the
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
content:
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

// TestVerify_GlossaryFromConventionalLocation covers the well-known-location
// ladder for terms: with no defaults.termbase_source binding, a glossary
// committed at the conventional path is still found and still gates.
//
// The repository root is searched before .kapi/ deliberately. The bundle is
// committed source, and .kapi/ is ignored by both the repository .gitignore and
// core/ignore's defaults — so a glossary kept there would never be committed or
// reviewed. .kapi/ stays as the second rung only so a project that treats its
// glossary as local state still resolves.
func TestVerify_GlossaryFromConventionalLocation(t *testing.T) {
	tests := []struct {
		name string
		rel  string
	}{
		{"repository root", "terms.json"},
		{"state directory", filepath.Join(".kapi", "terms.json")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTermsProjectUnbound(t, tt.rel)
			t.Chdir(root)

			_, runErr := runVerifyJSON(t)
			require.ErrorIs(t, runErr, ErrQualityGate,
				"an unbound project must find the conventional glossary and fail on the mistranslated term")
		})
	}
}

// TestVerify_ConventionalGlossaryPrefersRoot pins the ordering when both rungs
// are present: the committed root copy wins over the gitignored state copy.
func TestVerify_ConventionalGlossaryPrefersRoot(t *testing.T) {
	root := writeTermsProjectUnbound(t, "terms.json")

	// A state-directory glossary that would PASS the gate. If the ladder
	// preferred .kapi/, the run would come back clean and this test would fail.
	stray := ktb.FromConcepts([]terms.Concept{{
		ID: "save",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Sauvegarder", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}})
	data, err := ktb.Marshal(stray)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", "terms.json"), data, 0o644))

	t.Chdir(root)
	_, runErr := runVerifyJSON(t)
	require.ErrorIs(t, runErr, ErrQualityGate,
		"the committed root glossary must win over the gitignored state copy")
}
