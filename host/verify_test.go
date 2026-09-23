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
      advisory: true
  competitor_terms:
    - term: Globex
      replacement: our platform
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
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root), []byte(verifyVoiceYAML), 0o644))

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
	seedProjectStore(t, root, func(db *projectdb.DB) {
		require.NoError(t, db.Terms().AddConcept(t.Context(), terms.Concept{
			ID: "c1",
			Terms: []terms.Term{
				{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
			},
		}))
	})

	// The voice gate reads the profile the store holds, so the bound file
	// reaches it through the explicit import.
	readProjectContext(t, root)

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
// terminology, and check findings, an overall pass:false, and the quality-gate
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

	checks, ok := gateByName(out, gateChecks)
	require.True(t, ok, "the checks gate must be present")
	assert.False(t, checks.Pass, "the checks gate must fail (dropped placeholder)")
	require.NotEmpty(t, checks.Findings)

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

// writeTermsSourceProject creates a project that binds a committed terms source
// (a `.kapi/terms.json`) and reads it into the project store, which is where
// the check path resolves the vocabulary from. The bundle carries two concepts:
// a do-not-translate brand term (KapiMart, identical in en/fr) and a translated
// term (Save -> Enregistrer). The fr target keeps the brand term identical
// (correct) and mistranslates "Save" as "Sauvegarder" (a terms fail).
func writeTermsSourceProject(t *testing.T) string {
	t.Helper()
	root := writeUnreadTermsSourceProject(t)
	readProjectContext(t, root)
	return root
}

// writeUnreadTermsSourceProject is writeTermsSourceProject with the bundle left
// where it was written: committed, bound by the recipe, and read by nobody.
func writeUnreadTermsSourceProject(t *testing.T) string {
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

	return root
}

// TestVerify_PreferredTermsFromTermsSource asserts the terminology gate resolves
// the project vocabulary a read of the committed bundle put into the store. The
// gate runs and fails on the mistranslated "Save".
func TestVerify_PreferredTermsFromTermsSource(t *testing.T) {
	root := writeTermsSourceProject(t)
	t.Chdir(root)

	out, runErr := runVerifyJSON(t)

	require.ErrorIs(t, runErr, ErrQualityGate, "the mistranslated term must fail the gate")

	terms, ok := gateByName(out, gateTerms)
	require.True(t, ok, "terminology gate must run when a terms source is bound")
	assert.False(t, terms.Pass, "terminology gate must fail on Save -> Sauvegarder")
	require.NotEmpty(t, terms.Findings)
	assert.Contains(t, terms.Findings[0].Message, "Enregistrer",
		"the finding must name the preferred term the store holds")
}

// TestVerify_PreferredTermsFromAnUnreadTermsSource pins the gate's half of the
// store-only contract, at the shape a fresh checkout arrives in: the committed
// bundle is there, the recipe binds it, and the store is present and empty
// because the first kapi command run in the checkout opened it.
//
// The gate enforces what the store holds, so the mistranslated "Save" passes
// until someone runs `kapi context import`. A gate that answered from the file
// would hold every checkout to whatever its own branch carries.
func TestVerify_PreferredTermsFromAnUnreadTermsSource(t *testing.T) {
	root := writeUnreadTermsSourceProject(t)

	// Exactly what a first command in a fresh checkout leaves behind.
	db := openProjectStore(t, root)
	has, err := db.HasTerms(t.Context())
	require.NoError(t, err)
	require.False(t, has, "the store is present and empty")
	require.NoError(t, db.Close())
	require.FileExists(t, project.LayoutAt(root).StorePath())

	t.Chdir(root)
	out, _ := runVerifyJSON(t)

	_, gated := gateByName(out, gateTerms)
	assert.False(t, gated, "an unread bundle leaves the project with no vocabulary to gate")

	// Reading it in is what puts the vocabulary in force.
	readProjectContext(t, root)
	out, runErr := runVerifyJSON(t)
	require.ErrorIs(t, runErr, ErrQualityGate)
	terms, ok := gateByName(out, gateTerms)
	require.True(t, ok)
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

	checks, ok := gateByName(out, gateChecks)
	require.True(t, ok, "the checks gate must be present")
	for _, f := range checks.Findings {
		assert.NotContains(t, f.Message, "identical to source",
			"the do-not-translate brand term must not be flagged as untranslated")
	}
}

// writeTermsProjectUnbound builds the same project as writeTermsSourceProject
// but with NO defaults.terms_source binding, writing the terms bundle at rel
// instead.
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
	require.NotContains(t, recipe, "terms_source",
		"the bundle must reach the store as part of the layout, not through a binding")

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

// TestVerify_UnboundTermsReachTheGateFromTheLayout covers the bundle an import
// picks up without a binding. `.kapi/terms.json` is part of the layout, so a
// read finds it and the gate then enforces it. A bundle anywhere else is a file
// the import was never pointed at, and the gate enforces nothing from it.
func TestVerify_UnboundTermsReachTheGateFromTheLayout(t *testing.T) {
	tests := []struct {
		name     string
		rel      string
		wantGate bool
	}{
		{"the layout's own terms bundle", project.RelStatePath("terms.json"), true},
		{"a bundle at the repository root", "terms.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTermsProjectUnbound(t, tt.rel)
			readProjectContext(t, root)
			t.Chdir(root)

			_, runErr := runVerifyJSON(t)
			if tt.wantGate {
				require.ErrorIs(t, runErr, ErrQualityGate,
					"the read vocabulary fails the mistranslated term")
				return
			}
			require.NoError(t, runErr, "a bundle outside the layout enforces nothing")
		})
	}
}

// TestVerify_TermsAtTheRootLeaveTheLayoutsBundleInForce pins which bundle
// answers when a project carries two. The layout's is read; the one at the root
// is a file like any other, so the vocabulary in force stays the layout's even
// though the stray copy would pass the same project.
func TestVerify_TermsAtTheRootLeaveTheLayoutsBundleInForce(t *testing.T) {
	root := writeTermsProjectUnbound(t, project.RelStatePath("terms.json"))

	// A root terms file that would PASS the gate, so a run that read it would
	// come back clean and this test would fail.
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

	readProjectContext(t, root)
	t.Chdir(root)
	_, runErr := runVerifyJSON(t)
	require.ErrorIs(t, runErr, ErrQualityGate,
		"the layout's terms stay in force beside the copy at the root")
}
