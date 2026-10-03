//go:build !js

package host

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// TestCheckCanary_InertCheckerInvalidatesTheRun puts a checker that finds
// nothing in place of hygiene. The content is clean, so without the canary the
// run would pass; with it, the run must not.
func TestCheckCanary_InertCheckerInvalidatesTheRun(t *testing.T) {
	isolateCheckExecution(t)
	src := writeCheckInput(t, t.TempDir(), "app.json", `{"title":"Hello world"}`)
	app := &App{SourceLang: "en"}

	report, err := app.ComputeCheck(executionCommand(t), []string{src})
	require.NoError(t, err)
	require.Equal(t, check.VerdictPassed, report.Verdict, "the real checker catches its canary")
	assert.Equal(t, check.CanaryCaught, analyzerRun(t, report, "hygiene").Canary.Status)

	real := hygieneTool
	hygieneTool = inertHygiene
	t.Cleanup(func() { hygieneTool = real })

	report, err = app.ComputeCheck(executionCommand(t), []string{src})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	assert.False(t, report.Pass)
	hygiene := analyzerRun(t, report, "hygiene")
	assert.Equal(t, check.AnalyzerInvalid, hygiene.Status)
	assert.Equal(t, check.CanaryMissed, hygiene.Canary.Status)
	require.NotEmpty(t, report.DidNotRun)
	assert.Contains(t, report.DidNotRun[0], "hygiene reported no finding on its canary")

	for _, noFail := range []bool{false, true} {
		cmd := executionCommand(t)
		cmd.Flags().Bool("no-fail", noFail, "")
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := app.RunCheck(cmd, []string{src})
		require.ErrorIs(t, err, ErrCheckNotRun)
		assert.Equal(t, ExitNotRun, ExitCode(cmd, err))
		assert.Contains(t, out.String(), "DID NOT RUN")
		assert.Contains(t, out.String(), "missed a canary")
	}

	_, mcpReport, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Hello world"})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictDidNotRun, mcpReport.Verdict, "the agent surface reaches the same verdict")
}

func TestCheckCanary_NothingToCatch(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	src := writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)
	toneOnly := writeCheckInput(t, dir, "tone.yaml", "id: tone\nname: Tone\ntone:\n  personality: [plain]\n")
	app := &App{SourceLang: "en"}

	t.Run("a named profile with no rules", func(t *testing.T) {
		cmd := executionCommand(t)
		cmd.Flags().String("profile-file", toneOnly, "")
		report, err := app.ComputeCheck(cmd, []string{src})
		require.NoError(t, err)
		run := analyzerRun(t, report, "voice.rules")
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.True(t, run.Required)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("a required pattern every text satisfies", func(t *testing.T) {
		cmd := executionCommand(t)
		cmd.Flags().StringSlice("require", []string{".*"}, "")
		report, err := app.ComputeCheck(cmd, []string{src})
		require.NoError(t, err)
		assert.Equal(t, check.AnalyzerDidNotRun, analyzerRun(t, report, "pattern").Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("a project profile with no rules", func(t *testing.T) {
		require.NoError(t, os.WriteFile(layoutVoicePath(t, dir), []byte("id: tone\nname: Tone\ntone:\n  personality: [plain]\n"), 0o644))
		recipe := writeCheckInput(t, dir, "custom.kapi", "version: v1\ndefaults:\n  source_language: en\n  voice:\n    profile: tone\ncollections:\n  - path: app.json\n")
		readContextAt(t, recipe)
		serverCmd := NewEnvCommand(t.Context(), "mcp")
		serverCmd.Flags().String("project", recipe, "")
		projectApp := &App{}
		require.NoError(t, projectApp.ResolveMCPProject(serverCmd))
		_, report, err := projectApp.checkFileMCP(t.Context(), checkFileInput{File: src})
		require.NoError(t, err)
		run := analyzerRun(t, report, "voice.rules")
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.False(t, run.Required, "a profile the project binds may govern tone alone")
		assert.Equal(t, check.VerdictPassed, report.Verdict)
	})
}
