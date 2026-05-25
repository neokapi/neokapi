package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
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
      severity: major
`

// writeVerifyProject creates a temp project that binds a brand profile and a
// project termbase, with an English source file and a French target file. The
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
	require.NoError(t, os.WriteFile(filepath.Join(root, "proj.kapi"), []byte(recipe), 0o644))
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

	// Seed the project termbase: Save -> Enregistrer (approved).
	tbPath := filepath.Join(root, ".kapi", "termbase.db")
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID: "c1",
		Terms: []termbase.Term{
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
func runVerifyJSON(t *testing.T) (VerifyOutput, error) {
	t.Helper()
	a := &App{}
	cmd := a.NewVerifyCmd()
	require.NoError(t, cmd.Flags().Set("json", "true"))

	// Capture stdout (output.Print writes to os.Stdout). The returned error is
	// runVerify's own return value (the quality-gate sentinel on failure).
	out, runErr := captureStdout(t, func() error {
		return a.runVerify(cmd, nil)
	})

	var parsed VerifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "verify must emit valid JSON: %s", out)
	return parsed, runErr
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written along with fn's return value.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String(), runErr
}

// gateByName returns the gate result with the given name (or a zero value with
// found=false).
func gateByName(o VerifyOutput, name string) (VerifyGateResult, bool) {
	for _, g := range o.Gates {
		if g.Gate == name {
			return g, true
		}
	}
	return VerifyGateResult{}, false
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
	assert.False(t, brand.Pass, "brand gate must fail (competitor term Globex)")
	require.NotEmpty(t, brand.Findings, "brand gate must produce findings")

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
	cmd := a.NewVerifyCmd()
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.Flags().Set("no-fail", "true"))

	out, runErr := captureStdout(t, func() error { return a.runVerify(cmd, nil) })

	require.NoError(t, runErr, "--no-fail must not return the gate sentinel (exit 0)")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr), "--no-fail maps to exit 0 even on gate failure")

	var parsed VerifyOutput
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
	cmd := a.NewVerifyCmd()
	err := a.runVerify(cmd, nil)

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrQualityGate, "no-project is operational, not a gate failure")
	assert.Equal(t, ExitError, ExitCode(nil, err), "operational error must map to exit 1")
	assert.Contains(t, err.Error(), "no .kapi project")
}

// TestVerify_GateSelection asserts that --terms runs only the terminology gate.
func TestVerify_GateSelection(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := a.NewVerifyCmd()
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.Flags().Set("terms", "true"))

	out, err := captureStdout(t, func() error { return a.runVerify(cmd, nil) })
	// The failing project's terminology gate fails, so verify returns the
	// quality-gate sentinel — the point of this test is that ONLY that gate ran.
	require.ErrorIs(t, err, ErrQualityGate)

	var parsed VerifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))

	require.Len(t, parsed.Gates, 1, "only the terminology gate should run")
	assert.Equal(t, gateTerms, parsed.Gates[0].Gate)
}
