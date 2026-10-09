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

// caughtCanary is the outcome of an analyzer that flagged the one canary it
// was given, which is what lets a gate's analyzer count as having run.
func caughtCanary() *check.CanaryOutcome {
	return &check.CanaryOutcome{Status: check.CanaryCaught, Probes: 1}
}

// TestShipEvaluationRecord pins the document `kapi check --ship --json`
// carries: the gate roll-up, and under `evaluation` the one record assembled
// over every gate, with the clock and the build stamps fixed. Regenerate with:
//
//	KAPI_UPDATE_GOLDEN=1 go test ./host -run TestShipEvaluationRecord
func TestShipEvaluationRecord(t *testing.T) {
	pinEvaluationBuild(t)

	gates := []verifyGateResult{
		{
			Gate: gateVoice, Pass: true, Findings: []verifyFinding{},
			Coverage: &verifyCoverage{Files: 2, Blocks: 14},
			Execution: &check.Execution{Analyzers: []check.AnalyzerExecution{
				{ID: "voice.rules", File: "docs/guide.md", Status: check.AnalyzerPassed, Canary: caughtCanary()},
				{ID: "voice.rules", File: "docs/intro.md", Status: check.AnalyzerPassed, Canary: caughtCanary()},
			}},
		},
		{
			Gate: gateChecks, Pass: false,
			Findings: []verifyFinding{{
				Gate: gateChecks, File: "docs/intro.md", Locale: "nb", Fails: true,
				Message:    "placeholder {name} is missing from the target",
				Suggestion: "restore the placeholder in the translation",
			}},
			Coverage: &verifyCoverage{Files: 2, Blocks: 14},
			Execution: &check.Execution{Analyzers: []check.AnalyzerExecution{
				{ID: "checks.target", File: "docs/guide.md", Status: check.AnalyzerPassed, Required: true, Canary: caughtCanary()},
				{ID: "checks.target", File: "docs/intro.md", Status: check.AnalyzerFindings, Required: true, Findings: 1, Canary: caughtCanary()},
			}},
		},
		{Gate: gateShip, Pass: true, Findings: []verifyFinding{}},
	}
	out := buildVerifyOutput(gates)
	out.Evaluation = buildEvaluation(
		&check.ContextProvenance{Project: "prj_aaaabbbbccccddddeeee", Name: "Acme Documentation", Revision: 412},
		[]check.EvaluationPlugin{{Name: "sourcecode", Version: "1.2.0", Serves: []string{"comments:python"}}},
		gateAnalyzers(out.Gates),
	)
	require.Equal(t, check.VerdictFailed, out.Verdict, "the record never reaches the verdict")

	raw, err := json.MarshalIndent(out, "", "  ")
	require.NoError(t, err)
	compareEvaluationGolden(t, "ship_gates", append(raw, '\n'))
}

// shipFixture is a project with one source-only collection, checked by the
// gates over its workspace. The recipe path is returned for the -p flag.
func shipFixture(t *testing.T) (*App, string) {
	t.Helper()
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
    source_only: true
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".kapi"), 0o755))
	app := &App{SourceLang: "en"}
	app.SetWorkspaceRoot(filepath.Join(dir, "workspace"))
	return app, recipe
}

func shipCommand(t *testing.T, recipe string) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set(projectFlagName, recipe))
	return cmd
}

// TestShipCarriesTheEvaluationRecord runs the gates over a project: the one
// record names the project and the workspace revision the gates read, the
// build, and the coverage of every analyzer the gates recorded, and the gates'
// verdict is what it would be without it.
func TestShipCarriesTheEvaluationRecord(t *testing.T) {
	app, recipe := shipFixture(t)
	out, err := app.computeVerify(shipCommand(t, recipe), nil)
	require.NoError(t, err)

	require.NotNil(t, out.Evaluation)
	assert.Equal(t, "2026-03-17T09:45:00Z", out.Evaluation.At)
	assert.Equal(t, check.EvaluationTool{Name: "kapi", Version: "1.3.0", Commit: "b1a3033dc"}, out.Evaluation.Tool)
	assert.Empty(t, out.Evaluation.Plugins, "no plugin serves a JSON file kapi reads itself")

	require.NotNil(t, out.Evaluation.Context)
	assert.Equal(t, "prj_ffffgggghhhhiiiijjjj", out.Evaluation.Context.Project)
	assert.Equal(t, "Evaluation Fixture", out.Evaluation.Context.Name)
	ws, err := app.Workspace(t.Context())
	require.NoError(t, err)
	head, err := ws.Head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, head, out.Evaluation.Context.Revision)

	// The coverage is the projection of what the gates recorded, so an
	// analyzer a gate ran is in it exactly as often as the gate ran it.
	covered := map[string]int{}
	for _, c := range out.Evaluation.Analyzers {
		covered[c.ID] = c.Ran
	}
	ran := map[string]int{}
	for _, run := range gateAnalyzers(out.Gates) {
		if run.Status == check.AnalyzerPassed || run.Status == check.AnalyzerFindings {
			ran[run.ID]++
		}
	}
	assert.NotEmpty(t, ran, "the checks gate records its analyzers")
	for id, n := range ran {
		assert.Equal(t, n, covered[id], "coverage of %s", id)
	}
	assert.Equal(t, 1, covered["hygiene"], "the source checks ran over the one declared file")

	// The same run, assembled again without the record: the verdict is
	// settled by the gates alone.
	again := buildVerifyOutput(out.Gates)
	assert.Equal(t, again.Verdict, out.Verdict)
	assert.Equal(t, again.Pass, out.Pass)
}

// TestShipRecordIsNotInTheHumanOutput reads the same run as a person reads it:
// the record adds no line.
func TestShipRecordIsNotInTheHumanOutput(t *testing.T) {
	app, recipe := shipFixture(t)
	out, err := app.computeVerify(shipCommand(t, recipe), nil)
	require.NoError(t, err)
	require.NotNil(t, out.Evaluation)

	text, err := captureStdout(t, func() error { return out.FormatText(os.Stdout) })
	require.NoError(t, err)
	assert.NotContains(t, text, "prj_ffffgggghhhhiiiijjjj")
	assert.NotContains(t, text, "revision")
	assert.NotContains(t, text, "2026-03-17")
}

// TestShipRecordsThePluginsItsGatesReach runs the gates over a project whose
// content a plugin reads: the comments of source files the sourcecode plugin
// locates. The gates read those files through paths of their own, and the
// record names the plugin and each language it served.
func TestShipRecordsThePluginsItsGatesReach(t *testing.T) {
	a := sourcecodeApp(t, nil)
	pinEvaluationBuild(t)
	root := sourcecodeProject(t)

	out, err := a.computeVerify(shipCommand(t, filepath.Join(root, "kapi.yaml")), nil)
	require.NoError(t, err)
	require.NotNil(t, out.Evaluation)

	require.Len(t, out.Evaluation.Plugins, 1, "one plugin served the run")
	plugin := out.Evaluation.Plugins[0]
	assert.Equal(t, "sourcecode", plugin.Name)
	assert.NotEmpty(t, plugin.Version, "the version its manifest declares")
	for language := range sourcecodeFiles {
		assert.Contains(t, plugin.Serves, "comments:"+language)
	}
}
