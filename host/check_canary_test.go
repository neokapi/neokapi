package host

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/tool"
)

// inertHygiene reports nothing for any input: the checker the canary is there
// to expose.
func inertHygiene() BlockProcessor {
	return &tool.BaseTool{ToolName: "inert", Annotate: func(tool.BlockView) error { return nil }}
}

func writeCheckInput(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func analyzerRun(t *testing.T, report check.Report, id string) check.AnalyzerExecution {
	t.Helper()
	require.NotNil(t, report.Execution)
	for _, run := range report.Execution.Analyzers {
		if run.ID == id {
			return run
		}
	}
	t.Fatalf("no %s analyzer in the report", id)
	return check.AnalyzerExecution{}
}

// TestCheckCanary_ZeroBlocksDoesNotPass checks a file with no content. It is
// not a pass under any flag.
func TestCheckCanary_ZeroBlocksDoesNotPass(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	empty := writeCheckInput(t, dir, "empty.json", `{}`)
	full := writeCheckInput(t, dir, "full.json", `{"title":"Hello world"}`)
	app := &App{SourceLang: "en"}

	report, err := app.ComputeCheck(executionCommand(t), []string{empty})
	require.NoError(t, err)
	assert.Equal(t, 0, report.Target.Blocks)
	assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	assert.False(t, report.Pass)
	assert.Equal(t, []string{"no content blocks were checked"}, report.DidNotRun)

	report, err = app.ComputeCheck(executionCommand(t), []string{empty, full})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictPassed, report.Verdict, "coverage counts across the run")

	for _, flag := range []string{"", "no-fail"} {
		cmd := executionCommand(t)
		if flag != "" {
			cmd.Flags().Bool(flag, true, "")
		}
		cmd.SetOut(io.Discard)
		err := app.RunCheck(cmd, []string{empty})
		assert.Equal(t, ExitNotRun, ExitCode(cmd, err), "with --%s", flag)
	}
}

func TestCheckCanary_GateFailureKeepsItsExitCodes(t *testing.T) {
	isolateCheckExecution(t)
	src := writeCheckInput(t, t.TempDir(), "app.json", `{"title":"TODO write this"}`)
	app := &App{SourceLang: "en"}

	cmd := executionCommand(t)
	cmd.Flags().StringSlice("forbid", []string{"TODO"}, "")
	report, err := app.ComputeCheck(cmd, []string{src})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictFailed, report.Verdict)
	assert.Equal(t, check.CanaryCaught, analyzerRun(t, report, "pattern").Canary.Status)

	cmd.SetOut(io.Discard)
	err = app.RunCheck(cmd, []string{src})
	assert.Equal(t, ExitGate, ExitCode(cmd, err))

	cmd.Flags().Bool("no-fail", true, "")
	assert.NoError(t, app.RunCheck(cmd, []string{src}))
}

func TestCheckCanary_EveryCompletedAnalyzerCarriesOne(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	src := writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)
	profilePath := writeCheckInput(t, dir, "voice.yaml", "id: v\nname: V\nterms:\n  - term: risk-free\nstyle:\n  required_patterns:\n    - regex: Hello\n")
	cmd := executionCommand(t)
	cmd.Flags().Int("max-chars", 40, "")
	cmd.Flags().StringSlice("forbid", []string{`(?i)\btodo\b`}, "")
	cmd.Flags().String("profile-file", profilePath, "")
	cmd.Flags().String("validate", "report", "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)

	completed := 0
	for _, run := range report.Execution.Analyzers {
		if run.Status != check.AnalyzerPassed && run.Status != check.AnalyzerFindings {
			continue
		}
		completed++
		require.NotNil(t, run.Canary, run.ID)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status, run.ID)
		assert.Positive(t, run.Canary.Probes, run.ID)
	}
	assert.Equal(t, 6, completed, "hygiene, length, pattern, terms, voice.rules and reader.validation")
}
