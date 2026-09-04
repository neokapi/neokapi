package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectRunCmd builds the `kapi run` command bound to a recipe, the way an
// embedding surface (the kapi-bowrain plugin's pre-push automation) drives
// RunFromProject: the real flag set, no --input, no --target-lang.
func projectRunCmd(t *testing.T, a *App, recipe string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewRunCmd(a, RunCmdOptions{})
	require.NoError(t, cmd.Flags().Set("project", recipe))
	cmd.SetContext(t.Context())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	return cmd, &out
}

// The findings hook is what lets a surface gate on a project run: the report
// the run prints is also handed to OnFindings, so the two never disagree.
func TestRunFromProject_OnFindingsReceivesWhatTheRunPrinted(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Acme Cloud"})
	a := processOnlyApp(t)
	cmd, out := projectRunCmd(t, a, recipe)

	var got []FlowFindings
	err := a.RunFromProject(cmd, "guard", recipe, RunCmdOptions{
		OnFindings: func(f FlowFindings) { got = append(got, f) },
	})
	require.NoError(t, err, out.String())

	require.Len(t, got, 1, "one pass, one report")
	assert.Equal(t, 1, got[0].Summary.Findings)
	assert.Equal(t, 1, got[0].Summary.Critical)
	require.Len(t, got[0].Findings, 1)
	assert.Equal(t, "dnt-check.do-not-translate", got[0].Findings[0].Rule)
	assert.Contains(t, out.String(), "CRITICAL", "the run still prints its report")
}

// A clean run hands over an empty summary rather than nothing: the caller
// learns the check ran and found nothing, which is not the same as no check.
func TestRunFromProject_OnFindingsReportsAClean(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Nonexistent Product"})
	a := processOnlyApp(t)
	cmd, out := projectRunCmd(t, a, recipe)

	var got []FlowFindings
	err := a.RunFromProject(cmd, "guard", recipe, RunCmdOptions{
		OnFindings: func(f FlowFindings) { got = append(got, f) },
	})
	require.NoError(t, err, out.String())
	require.Len(t, got, 1)
	assert.Equal(t, 0, got[0].Summary.Findings)
	assert.Empty(t, got[0].Findings)
}

// --quiet suppresses the printed report and must not suppress the gate: a
// push under -q with a gating automation has to stop on the same findings.
func TestRunFromProject_OnFindingsFiresUnderQuiet(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Acme Cloud"})
	a := processOnlyApp(t)
	a.Quiet = true
	cmd, out := projectRunCmd(t, a, recipe)

	var got []FlowFindings
	err := a.RunFromProject(cmd, "guard", recipe, RunCmdOptions{
		OnFindings: func(f FlowFindings) { got = append(got, f) },
	})
	require.NoError(t, err, out.String())
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].Summary.Findings)
	assert.NotContains(t, out.String(), "CRITICAL", "quiet prints no report")
}

// Without the hook nothing changes: an ordinary `kapi run` neither collects
// under --quiet nor pays for a report nobody reads.
func TestRunFromProject_NoHookLeavesTheRunAlone(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Acme Cloud"})
	a := processOnlyApp(t)
	a.Quiet = true
	cmd, out := projectRunCmd(t, a, recipe)
	require.NoError(t, a.RunFromProject(cmd, "guard", recipe, RunCmdOptions{}))
	assert.Empty(t, out.String())
}

// A built-in flow named in a project with no --input runs over the recipe's
// collections like a recipe flow: process-only, committed to the project
// store, no target file written. Before, it fell through to "--input (-i) is
// required" inside a project whose recipe names the files.
func TestRun_BuiltInFlowInProject_RunsOverTheCollections(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})

	out, err := runRunCmd(t, a, recipe, "pseudo-translate")
	require.NoError(t, err, out)

	assert.Equal(t, 1, storeOverlayCount(t, recipe, "targets/qps"),
		"the pass is pseudo-translate's default locale, committed to the store")
	_, statErr := os.Stat(filepath.Join(root, "src/locales/qps/messages.json"))
	assert.True(t, os.IsNotExist(statErr), "process-only: no target file")
	assert.Contains(t, out, "kapi merge")
}

// An explicit --input keeps the file path a built-in flow always had.
func TestRun_BuiltInFlowInProject_InputStillRunsTheFile(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})
	outPath := filepath.Join(t.TempDir(), "explicit.json")

	out, err := runRunCmd(t, a, recipe, "pseudo-translate", "-i", filepath.Join(root, srcRel), "-o", outPath)
	require.NoError(t, err, out)
	data, rerr := os.ReadFile(outPath)
	require.NoError(t, rerr)
	assert.NotEqual(t, `{"greeting":"Hello, world."}`, string(data))
}
