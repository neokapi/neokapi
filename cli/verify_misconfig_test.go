package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeUnboundProject creates a project that binds neither a brand voice nor a
// termbase (and has no convention brand.yaml / .kapi/termbase.db), with a clean
// en→fr translation so the QA gate passes. The brand and terminology gates have
// no binding to run against.
func writeUnboundProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: unbound
defaults:
  source_language: en
  target_languages: [fr]
content:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "proj.kapi"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte("{\"greeting\": \"Hello\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr", "app.json"),
		[]byte("{\"greeting\": \"Bonjour\"}\n"), 0o644))
	return root
}

// runVerifyGates runs verify --json with the given flag overrides applied.
func runVerifyGates(t *testing.T, flags map[string]string) (VerifyOutput, error) {
	t.Helper()
	a := &App{}
	cmd := a.NewVerifyCmd()
	require.NoError(t, cmd.Flags().Set("json", "true"))
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	out, runErr := captureStdout(t, func() error { return a.runVerify(cmd, nil) })
	var parsed VerifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "verify must emit valid JSON: %s", out)
	return parsed, runErr
}

// TestVerify_ExplicitBrandUnboundFails asserts that requesting --brand on a
// project that binds no brand voice fails loudly (misconfiguration) instead of
// silently passing.
func TestVerify_ExplicitBrandUnboundFails(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"brand": "true"})

	require.ErrorIs(t, runErr, ErrQualityGate, "an explicitly-requested unbound gate must fail")
	assert.Equal(t, ExitGate, ExitCode(nil, runErr))
	assert.False(t, out.Pass)

	g, ok := gateByName(out, gateBrand)
	require.True(t, ok, "the brand gate must appear as a misconfig failure, not be skipped")
	assert.False(t, g.Pass)
	require.NotEmpty(t, g.Findings)
	assert.Contains(t, g.Findings[0].Message, "defaults.brand_voice")
	assert.Equal(t, "error", g.Findings[0].Severity)
	require.Len(t, out.Gates, 1, "only the explicitly requested gate ran")
}

// TestVerify_ExplicitTermsUnboundFails asserts the same for --terms with no
// bound termbase.
func TestVerify_ExplicitTermsUnboundFails(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"terms": "true"})

	require.ErrorIs(t, runErr, ErrQualityGate)
	assert.Equal(t, ExitGate, ExitCode(nil, runErr))
	assert.False(t, out.Pass)

	g, ok := gateByName(out, gateTerms)
	require.True(t, ok, "the terminology gate must appear as a misconfig failure")
	assert.False(t, g.Pass)
	require.NotEmpty(t, g.Findings)
	assert.Contains(t, g.Findings[0].Message, "defaults.termbase")
	require.Len(t, out.Gates, 1)
}

// TestVerify_UnboundMisconfigNoFailReportsOnly asserts that --no-fail downgrades
// a requested-but-unbound gate failure to report-only (exit 0) while still
// reporting the misconfiguration in the output.
func TestVerify_UnboundMisconfigNoFailReportsOnly(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"brand": "true", "no-fail": "true"})

	require.NoError(t, runErr, "--no-fail must exit 0 even for a misconfig failure")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr))
	assert.False(t, out.Pass, "the misconfiguration is still reported in the verdict")
	g, ok := gateByName(out, gateBrand)
	require.True(t, ok)
	assert.False(t, g.Pass)
}

// TestVerify_DefaultRunSkipsUnboundGates asserts that with no gate flags, unbound
// brand and terminology gates are skipped silently (kept out of the result) and
// only the binding-free QA gate runs.
func TestVerify_DefaultRunSkipsUnboundGates(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, nil)

	require.NoError(t, runErr, "a clean default run with no bindings must pass")
	assert.True(t, out.Pass)

	_, hasBrand := gateByName(out, gateBrand)
	assert.False(t, hasBrand, "unbound brand gate must be skipped in a default run")
	_, hasTerms := gateByName(out, gateTerms)
	assert.False(t, hasTerms, "unbound terminology gate must be skipped in a default run")

	qa, hasQA := gateByName(out, gateQA)
	require.True(t, hasQA, "the QA gate always runs (no binding required)")
	assert.True(t, qa.Pass)
}

// TestVerify_ExplicitBrandBoundRunsRealCheck asserts that when a brand voice IS
// bound, --brand runs the real check (and fails on actual content) rather than
// emitting the misconfiguration failure.
func TestVerify_ExplicitBrandBoundRunsRealCheck(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	out, runErr := runVerifyGates(t, map[string]string{"brand": "true"})

	require.ErrorIs(t, runErr, ErrQualityGate)
	g, ok := gateByName(out, gateBrand)
	require.True(t, ok)
	assert.False(t, g.Pass, "the bound brand gate fails on the competitor term")
	for _, f := range g.Findings {
		assert.NotContains(t, f.Message, "binds no",
			"a bound gate must run the real check, not the misconfig failure")
	}
	require.Len(t, out.Gates, 1, "only the brand gate ran")
}

// writeCleanBrandProject writes a project that binds a violation-free brand
// voice over clean source content, so the brand gate passes.
func writeCleanBrandProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))

	recipe := `version: v1
name: clean
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
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"),
		[]byte("name: Clean Voice\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte("{\"greeting\": \"Hello there\"}\n"), 0o644))
	return root
}

// TestVerify_ExplicitBrandBoundPasses asserts that --brand passes (exit 0) when
// a clean brand voice is bound over clean content.
func TestVerify_ExplicitBrandBoundPasses(t *testing.T) {
	t.Chdir(writeCleanBrandProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"brand": "true"})

	require.NoError(t, runErr, "a bound, satisfied brand gate must pass")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr))
	assert.True(t, out.Pass)
	g, ok := gateByName(out, gateBrand)
	require.True(t, ok)
	assert.True(t, g.Pass)
	assert.Empty(t, g.Findings)
}
