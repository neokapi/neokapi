package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/version"
)

// pinEvaluationBuild fixes the clock and the build stamps the evaluation record
// reads, so a golden pins the document rather than the machine it ran on.
func pinEvaluationBuild(t *testing.T) {
	t.Helper()
	at := time.Date(2026, 3, 17, 9, 45, 0, 0, time.UTC)
	realClock := evaluationClock
	evaluationClock = func() time.Time { return at }
	realVersion, realCommit := version.Version, version.Commit
	version.Version, version.Commit = "1.3.0", "b1a3033dc"
	t.Cleanup(func() {
		evaluationClock = realClock
		version.Version, version.Commit = realVersion, realCommit
	})
}

// compareEvaluationGolden pins one evaluation document. Regenerate the files
// after an intentional change with:
//
//	KAPI_UPDATE_GOLDEN=1 go test ./host -run TestCheckEvaluationRecord
func compareEvaluationGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "evaluation", name+".golden.json")
	if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing; run with KAPI_UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got),
		"evaluation record drift in %s. If intentional, regenerate with KAPI_UPDATE_GOLDEN=1 and update the docs that name its fields", path)
}

// TestCheckEvaluationRecord pins the document `kapi check --json` carries under
// `evaluation`, across the states a reader must be able to tell apart: a
// project whose context has moved, one whose context is empty, a check of files
// outside any project, a run an analyzer did not run in, and a run whose
// projection has drifted from the files on disk.
func TestCheckEvaluationRecord(t *testing.T) {
	pinEvaluationBuild(t)

	ranEverything := []check.AnalyzerExecution{
		{ID: "hygiene", File: "docs/guide.md", Status: check.AnalyzerPassed, Required: true},
		{ID: "hygiene", File: "docs/intro.md", Status: check.AnalyzerFindings, Required: true, Findings: 2},
		{ID: "voice.rules", File: "docs/guide.md", Status: check.AnalyzerPassed, Required: true},
		{ID: "voice.rules", File: "docs/intro.md", Status: check.AnalyzerPassed, Required: true},
	}

	cases := []struct {
		name      string
		context   *check.ContextProvenance
		plugins   []check.EvaluationPlugin
		analyzers []check.AnalyzerExecution
	}{
		{
			// A project whose workspace has recorded work: the revision says
			// which state of the terms, voice rules and decisions governed.
			name: "project_with_context",
			context: &check.ContextProvenance{
				Project: "prj_aaaabbbbccccddddeeee", Name: "Acme Documentation", Revision: 412,
			},
			plugins: []check.EvaluationPlugin{
				{Name: "pdfium", Version: "0.4.1", Serves: []string{"format:pdf"}},
				{Name: "sourcecode", Version: "1.2.0", Serves: []string{"comments:python", "comments:typescript"}},
			},
			analyzers: ranEverything,
		},
		{
			// A project nobody has recorded anything about: the log is at zero,
			// and the record says so rather than leaving the field out.
			name:      "project_empty_context",
			context:   &check.ContextProvenance{Project: "prj_aaaabbbbccccddddeeee", Name: "Acme Documentation"},
			analyzers: ranEverything,
		},
		{
			// Files named outside any project. There is no context to read, so
			// the record carries none and invents no identity.
			name:      "file_mode_no_project",
			analyzers: ranEverything,
		},
		{
			name:    "analyzer_did_not_run",
			context: &check.ContextProvenance{Project: "prj_aaaabbbbccccddddeeee", Revision: 412},
			analyzers: []check.AnalyzerExecution{
				{ID: "hygiene", File: "docs/guide.md", Status: check.AnalyzerPassed, Required: true},
				{ID: "length", File: "docs/guide.md", Status: check.AnalyzerNotRequested, Reason: "No length limit was configured."},
				{ID: "voice.rules", File: "docs/guide.md", Status: check.AnalyzerNotApplicable, Reason: "The voice profile declares no term or pattern, and the comment analyzers check its comment limits."},
				{ID: "voice.guidance", File: "docs/guide.md", Status: check.AnalyzerUnsupported, Reason: "Applicable guidance requires semantic analysis; deterministic rules do not assess it."},
				{ID: "voice.similarity", File: "docs/guide.md", Status: check.AnalyzerInvalid, Required: true, Reason: `Reported no finding on its canary, doubled word ("the the").`},
				{ID: "voice.similarity", File: "docs/intro.md", Status: check.AnalyzerInvalid, Required: true, Reason: `Reported no finding on its canary, doubled word ("the the").`},
			},
		},
		{
			// The blocks kapi holds were read before the files changed, so a
			// count over content describes the files as they were.
			name: "stale_context",
			context: &check.ContextProvenance{
				Project: "prj_aaaabbbbccccddddeeee", Name: "Acme Documentation", Revision: 412, Stale: true,
				StaleReason: "3 file(s) changed since kapi read them. `kapi up` reads them again",
			},
			analyzers: ranEverything,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.MarshalIndent(buildEvaluation(tc.context, tc.plugins, tc.analyzers), "", "  ")
			require.NoError(t, err)
			compareEvaluationGolden(t, tc.name, append(raw, '\n'))
		})
	}
}

// TestCheckEvaluationCoverageProjectsTheRunsRecorded asserts the coverage is a
// projection of the analyzer executions the run already recorded, counted the
// way the human summary counts them: passed and findings are runs, and every
// other status is grouped under the reason it carries.
func TestCheckEvaluationCoverageProjectsTheRunsRecorded(t *testing.T) {
	coverage := check.AnalyzerCoverageOf([]check.AnalyzerExecution{
		{ID: "voice.rules", Status: check.AnalyzerNotRequested, Reason: "No voice profile or project terms were bound."},
		{ID: "hygiene", Status: check.AnalyzerFindings, Findings: 1},
		{ID: "hygiene", Status: check.AnalyzerPassed},
		{ID: "voice.rules", Status: check.AnalyzerNotRequested, Reason: "No voice profile or project terms were bound."},
		{ID: "voice.rules", Status: check.AnalyzerPassed},
	})
	require.Len(t, coverage, 2)
	assert.Equal(t, "hygiene", coverage[0].ID)
	assert.Equal(t, 2, coverage[0].Ran)
	assert.Empty(t, coverage[0].NotRun)
	assert.Equal(t, "voice.rules", coverage[1].ID)
	assert.Equal(t, 1, coverage[1].Ran)
	require.Len(t, coverage[1].NotRun, 1, "one reason covers both inputs it was not requested for")
	assert.Equal(t, check.AnalyzerNotRequested, coverage[1].NotRun[0].Status)
	assert.Equal(t, 2, coverage[1].NotRun[0].Inputs)
}

// TestCheckEvaluationOutsideAProjectNamesNoProject checks files with no project
// in scope: the run reads no context, so the record carries none and the check
// still reaches a verdict.
func TestCheckEvaluationOutsideAProjectNamesNoProject(t *testing.T) {
	isolateCheckExecution(t)
	pinEvaluationBuild(t)
	dir := t.TempDir()
	src := writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)

	report, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{src})
	require.NoError(t, err)
	require.NotNil(t, report.Evaluation)
	assert.Nil(t, report.Evaluation.Context)
	assert.Equal(t, "2026-03-17T09:45:00Z", report.Evaluation.At)
	assert.Equal(t, check.EvaluationTool{Name: "kapi", Version: "1.3.0", Commit: "b1a3033dc"}, report.Evaluation.Tool)
	assert.Empty(t, report.Evaluation.Plugins, "no plugin serves a JSON file kapi reads itself")
	assert.Equal(t, check.VerdictPassed, report.Verdict)

	ran := map[string]int{}
	for _, c := range report.Evaluation.Analyzers {
		ran[c.ID] = c.Ran
	}
	assert.Equal(t, 1, ran["hygiene"], "the coverage names the analyzers that ran")
	assert.Contains(t, ran, "voice.similarity", "and the ones that did not")
}

// TestCheckEvaluationInAProjectReadsTheWorkspace checks a project's declared
// content: the record names the project the run read its governance from and
// the workspace revision it read at, and an empty context leaves the verdict
// alone.
func TestCheckEvaluationInAProjectReadsTheWorkspace(t *testing.T) {
	isolateCheckExecution(t)
	pinEvaluationBuild(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)
	recipe := writeCheckInput(t, dir, "kapi.yaml", `version: v1
id: prj_ffffgggghhhhiiiijjjj
name: Evaluation Fixture
defaults:
  source_language: en
collections:
  - path: app.json
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".kapi"), 0o755))

	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, recipe, "")
	app := &App{SourceLang: "en"}
	app.SetWorkspaceRoot(filepath.Join(dir, "workspace"))
	report, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)

	require.NotNil(t, report.Evaluation)
	require.NotNil(t, report.Evaluation.Context)
	assert.Equal(t, "prj_ffffgggghhhhiiiijjjj", report.Evaluation.Context.Project)
	assert.Equal(t, "Evaluation Fixture", report.Evaluation.Context.Name)

	// The revision is the workspace operation log's head, the number a later
	// reader compares two answers by.
	ws, err := app.Workspace(t.Context())
	require.NoError(t, err)
	head, err := ws.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, head, report.Evaluation.Context.Revision)

	// A project kapi has read no content from yet has an empty context. The
	// record says so, and the check reaches its verdict either way.
	assert.True(t, report.Evaluation.Context.Stale)
	assert.Contains(t, report.Evaluation.Context.StaleReason, "read no content from this project yet")
	assert.Equal(t, check.VerdictPassed, report.Verdict, "the record reports; it never changes the verdict")

	// The same run read as a person reads it: the record adds no line.
	var out bytes.Buffer
	require.NoError(t, (checkReport{report}).FormatText(&out))
	assert.NotContains(t, out.String(), "prj_ffffgggghhhhiiiijjjj")
	assert.NotContains(t, out.String(), "revision")
}
