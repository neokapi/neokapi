package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// writeUnboundProject creates a project that binds neither a voice profile nor a
// terms (and has no convention voice.yaml, no committed terms source, and no
// concept in its store), with a clean en→fr translation so the checks gate
// passes. The voice and terminology gates have no binding to run against.
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
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte("{\"greeting\": \"Hello\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr", "app.json"),
		[]byte("{\"greeting\": \"Bonjour\"}\n"), 0o644))
	readProjectContext(t, root)
	return root
}

// runVerifyGates runs verify --json with the given flag overrides applied.
func runVerifyGates(t *testing.T, flags map[string]string) (verifyOutput, error) {
	t.Helper()
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	out, runErr := captureStdout(t, func() error { return a.RunVerify(cmd, nil) })
	var parsed verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "verify must emit valid JSON: %s", out)
	return parsed, runErr
}

// TestVerify_ExplicitVoiceUnboundDidNotRun asserts that naming the voice gate on
// a project that binds no voice profile reports the gate as not run
// (misconfiguration) instead of silently passing.
func TestVerify_ExplicitVoiceUnboundDidNotRun(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"gate": gateVoice})

	require.ErrorIs(t, runErr, ErrCheckNotRun, "an explicitly-requested unbound gate did not run")
	assert.Equal(t, ExitNotRun, ExitCode(nil, runErr))
	assert.False(t, out.Pass)
	assert.Equal(t, check.VerdictDidNotRun, out.Verdict)
	assert.Equal(t, check.CauseContentNotChecked, out.DidNotRunCause)

	g, ok := gateByName(out, gateVoice)
	require.True(t, ok, "the voice gate must appear as a misconfig failure, not be skipped")
	assert.False(t, g.Pass)
	require.NotEmpty(t, g.Findings)
	assert.Contains(t, g.Findings[0].Message, "defaults.voice")
	assert.Equal(t, "error", g.Findings[0].Severity)
	require.Len(t, out.Gates, 1, "only the explicitly requested gate ran")
}

// TestVerify_ExplicitTermsUnboundDidNotRun asserts the same for the terminology
// gate with no bound terms.
func TestVerify_ExplicitTermsUnboundDidNotRun(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"gate": gateTerms})

	require.ErrorIs(t, runErr, ErrCheckNotRun)
	assert.Equal(t, ExitNotRun, ExitCode(nil, runErr))
	assert.False(t, out.Pass)

	g, ok := gateByName(out, gateTerms)
	require.True(t, ok, "the terminology gate must appear as a misconfig failure")
	assert.False(t, g.Pass)
	require.NotEmpty(t, g.Findings)
	assert.Contains(t, g.Findings[0].Message, "defaults.terms_source",
		"the suggestion must name a recipe key that exists")
	require.Len(t, out.Gates, 1)
}

// TestVerify_UnboundMisconfigNoFailStillDidNotRun asserts that --no-fail does
// not turn a requested-but-unbound gate into exit 0: the gate checked nothing,
// so there are no findings for report mode to read.
func TestVerify_UnboundMisconfigNoFailStillDidNotRun(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, map[string]string{"gate": gateVoice, "no-fail": "true"})

	require.ErrorIs(t, runErr, ErrCheckNotRun, "--no-fail must not report a gate that did not run as clean")
	assert.Equal(t, ExitNotRun, ExitCode(nil, runErr))
	assert.False(t, out.Pass, "the misconfiguration is still reported in the verdict")
	g, ok := gateByName(out, gateVoice)
	require.True(t, ok)
	assert.False(t, g.Pass)
}

// TestVerify_DefaultRunSkipsUnboundGates asserts that with no gate flags, unbound
// voice and terminology gates are skipped silently (kept out of the result) and
// only the binding-free checks gate runs.
func TestVerify_DefaultRunSkipsUnboundGates(t *testing.T) {
	t.Chdir(writeUnboundProject(t))

	out, runErr := runVerifyGates(t, nil)

	require.NoError(t, runErr, "a clean default run with no bindings must pass")
	assert.True(t, out.Pass)

	_, hasVoice := gateByName(out, gateVoice)
	assert.False(t, hasVoice, "unbound voice gate must be skipped in a default run")
	_, hasTerms := gateByName(out, gateTerms)
	assert.False(t, hasTerms, "unbound terminology gate must be skipped in a default run")

	checks, hasChecks := gateByName(out, gateChecks)
	require.True(t, hasChecks, "the checks gate always runs (no binding required)")
	assert.True(t, checks.Pass)
}

// TestVerify_ExplicitVoiceBoundRunsRealCheck asserts that when a voice profile IS
// bound, naming the voice gate runs the real check (and fails on actual
// content) rather than emitting the misconfiguration failure.
func TestVerify_ExplicitVoiceBoundRunsRealCheck(t *testing.T) {
	root, _ := writeVerifyProject(t)
	t.Chdir(root)

	out, runErr := runVerifyGates(t, map[string]string{"gate": gateVoice})

	require.ErrorIs(t, runErr, ErrQualityGate)
	g, ok := gateByName(out, gateVoice)
	require.True(t, ok)
	assert.False(t, g.Pass, "the bound voice gate fails on the competitor term")
	for _, f := range g.Findings {
		assert.NotContains(t, f.Message, "binds no",
			"a bound gate must run the real check, not the misconfig failure")
	}
	require.Len(t, out.Gates, 1, "only the voice gate ran")
}

// writeCleanVoiceProject writes a project that binds a voice profile over clean
// source content. profile is the profile's YAML.
func writeCleanVoiceProject(t *testing.T, profile string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))

	recipe := `version: v1
name: clean
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
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(profile), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte("{\"greeting\": \"Hello there\"}\n"), 0o644))
	readProjectContext(t, root)
	return root
}

// TestVerify_ExplicitVoiceBoundPasses asserts that the voice gate passes (exit
// 0) when a voice profile with a rule is bound over content that keeps it.
func TestVerify_ExplicitVoiceBoundPasses(t *testing.T) {
	t.Chdir(writeCleanVoiceProject(t, "name: Clean Voice\nvocabulary:\n  forbidden_terms:\n    - term: risk-free\n"))

	out, runErr := runVerifyGates(t, map[string]string{"gate": gateVoice})

	require.NoError(t, runErr, "a bound, satisfied voice gate must pass")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr))
	assert.True(t, out.Pass)
	g, ok := gateByName(out, gateVoice)
	require.True(t, ok)
	assert.True(t, g.Pass)
	assert.Empty(t, g.Findings)
	require.NotNil(t, g.Execution)
	require.NotEmpty(t, g.Execution.Analyzers)
	assert.Equal(t, check.CanaryCaught, g.Execution.Analyzers[0].Canary.Status)
}

// TestVerify_ExplicitVoiceBoundWithNothingToCatchDidNotRun binds a profile with
// no deterministic rule. The gate has nothing it could flag, so it did not run
// rather than passing over clean content it never checked.
func TestVerify_ExplicitVoiceBoundWithNothingToCatchDidNotRun(t *testing.T) {
	t.Chdir(writeCleanVoiceProject(t, "name: Clean Voice\n"))

	out, runErr := runVerifyGates(t, map[string]string{"gate": gateVoice})

	require.ErrorIs(t, runErr, ErrCheckNotRun)
	assert.Equal(t, ExitNotRun, ExitCode(nil, runErr))
	g, ok := gateByName(out, gateVoice)
	require.True(t, ok)
	assert.Equal(t, check.VerdictDidNotRun, g.Verdict)
	assert.Equal(t, check.CauseContentNotChecked, g.DidNotRunCause)
}
