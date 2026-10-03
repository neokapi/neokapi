//go:build !js

package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckExecutionCLIAndMCPAgree(t *testing.T) {
	isolateCheckExecution(t)
	for _, bilingual := range []bool{false, true} {
		t.Run(map[bool]string{false: "source", true: "bilingual"}[bilingual], func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "app.json")
			require.NoError(t, os.WriteFile(src, []byte(`{"title":"We we ship Acme {name}"}`), 0o644))
			cmd := executionCommand(t)
			cmd.Flags().Int("max-chars", 10, "")
			in := checkFileInput{File: src, MaxChars: 10}
			if bilingual {
				target := filepath.Join(dir, "fr.json")
				require.NoError(t, os.WriteFile(target, []byte(`{"title":"Bonjour"}`), 0o644))
				cmd.Flags().String("target", target, "")
				cmd.Flags().String("target-lang", "fr", "")
				cmd.Flags().StringSlice("dnt", []string{"Acme"}, "")
				in.Target, in.TargetLang, in.DNT = target, "fr", []string{"Acme"}
			}
			cliReport, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
			require.NoError(t, err)
			_, mcpReport, err := (&App{SourceLang: "en"}).checkFileMCP(t.Context(), in)
			require.NoError(t, err)
			assert.Equal(t, cliReport.Findings, mcpReport.Findings)
			assert.Equal(t, cliReport.Summary, mcpReport.Summary)
			assert.Equal(t, cliReport.Summary.Failing, mcpReport.Summary.Failing)
			require.NotNil(t, cliReport.Execution)
			require.NotNil(t, mcpReport.Execution)
			for _, report := range []*check.Report{&cliReport, &mcpReport} {
				assert.Positive(t, report.Execution.Timings.TotalMS)
				assert.Positive(t, report.Execution.Timings.AnalyzersMS)
				for i := range report.Execution.Analyzers {
					report.Execution.Analyzers[i].DurationMS = nil
				}
			}
			assert.Equal(t, cliReport.Execution.Analyzers, mcpReport.Execution.Analyzers)
		})
	}
}

func TestCheckExecutionReportsConfiguredCoverage(t *testing.T) {
	isolateCheckExecution(t)
	_, report, err := (&App{SourceLang: "en"}).checkTextMCP(t.Context(), checkTextInput{Text: "Ready to ship."})
	require.NoError(t, err)
	require.NotNil(t, report.Execution)
	runs := map[string]check.AnalyzerExecution{}
	for _, run := range report.Execution.Analyzers {
		runs[run.ID] = run
	}
	assert.Equal(t, check.AnalyzerPassed, runs["hygiene"].Status)
	assert.True(t, runs["hygiene"].Required)
	require.NotNil(t, runs["hygiene"].DurationMS)
	for _, id := range []string{"length", "pattern", "voice.rules", "voice.similarity", "voice.llm"} {
		run := runs[id]
		assert.Equal(t, check.AnalyzerNotRequested, run.Status, id)
		assert.NotEmpty(t, run.Reason, id)
		assert.False(t, run.Required, id)
		assert.Nil(t, run.DurationMS, id)
	}
	assert.Equal(t, 100, report.Summary.Score)
	var out bytes.Buffer
	require.NoError(t, (checkReport{report}).FormatText(&out))
	assert.Contains(t, out.String(), "configured checks")
	assert.Contains(t, out.String(), "Coverage:")
}

func TestCheckExecutionRejectsUnavailableProfile(t *testing.T) {
	isolateCheckExecution(t)
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	_, report, err := (&App{SourceLang: "en"}).checkTextMCP(t.Context(), checkTextInput{Text: "Hello", ProfileFile: missing})
	require.Error(t, err)
	assert.Empty(t, report.Schema)
	assert.False(t, report.Pass)
}

func TestCheckExecutionBilingualValidationCannotSilentlySkip(t *testing.T) {
	isolateCheckExecution(t)
	src := filepath.Join(t.TempDir(), "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title":"Hello"}`), 0o644))
	cmd := executionCommand(t)
	cmd.Flags().String("target", src, "")
	cmd.Flags().String("validate", "strict", "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
	require.ErrorContains(t, err, "reader validation is unavailable")
	assert.Empty(t, report.Schema)
	_, report, err = (&App{SourceLang: "en"}).checkFileMCP(t.Context(), checkFileInput{File: src, Target: src, Validate: "strict"})
	require.ErrorContains(t, err, "reader validation is unavailable")
	assert.Empty(t, report.Schema)
}

func TestCheckExecutionMCPProjectResolutionFailure(t *testing.T) {
	isolateCheckExecution(t)
	t.Setenv("KAPI_NO_PROJECT", "")
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(projectPath, []byte("defaults: ["), 0o644))
	t.Setenv("KAPI_PROJECT", projectPath)
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title":"Hello"}`), 0o644))
	_, report, err := (&App{SourceLang: "en"}).checkFileMCP(t.Context(), checkFileInput{File: src})
	require.Error(t, err)
	assert.Empty(t, report.Schema)
	assert.False(t, report.Pass)
}

func TestCheckExecutionMCPUsesExplicitServerProject(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "content.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"body":"A risk-free appointment."}`), 0o644))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, dir), []byte(`name: Scoped
terms:
  - term: risk-free
`), 0o644))
	recipe := filepath.Join(dir, "custom.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
defaults:
  source_language: nb
collections:
  - path: content.json
`), 0o644))
	readContextAt(t, recipe)
	app := &App{}
	serverCmd := NewEnvCommand(t.Context(), "mcp")
	serverCmd.Flags().String("project", recipe, "")
	require.NoError(t, app.ResolveMCPProject(serverCmd))
	assert.Equal(t, "nb", app.SourceLocale())
	resolved, err := app.mcpProjectPath("")
	require.NoError(t, err)
	assert.Equal(t, recipe, resolved)
	_, report, err := app.checkFileMCP(t.Context(), checkFileInput{File: source})
	require.NoError(t, err)
	assert.False(t, report.Pass)
	assert.Equal(t, 1, report.Summary.Failing)
	termsRan := false
	for _, run := range report.Execution.Analyzers {
		if run.ID == "terms" {
			termsRan = run.Status == check.AnalyzerFindings
		}
	}
	assert.True(t, termsRan, "the explicit server recipe must survive disabled discovery")
	require.NoError(t, os.Remove(recipe))
	_, report, err = app.checkFileMCP(t.Context(), checkFileInput{File: source})
	require.Error(t, err, "an unavailable bound recipe must not become ungoverned")
	assert.Empty(t, report.Schema)
}
