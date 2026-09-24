package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project keeps its flows inline on the recipe or one file per flow in the
// directory the recipe names with flows_dir, and both run the same way: over the recipe's collections,
// through the project runner, with the recipe's bindings and the findings its
// check steps report. These drive the file-per-flow half through the local
// run_flow action, which is the surface #2426 was found on.

const guardDirFlow = `name: guard
description: Check the do-not-translate list

steps:
  - tool: dnt-check
    config:
      terms:
        - Acme Cloud
`

// dirFlowFixture writes a project whose `guard` flow lives in flows/guard.yaml,
// named by the recipe's flows_dir, rather than inline on the recipe.
func dirFlowFixture(t *testing.T, rules ...project.AutomationSpec) *project.Project {
	t.Helper()
	isolateKapi(t)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe := &project.Recipe{
		Version:  coreproj.CurrentVersion,
		Name:     "DirFlowTest",
		FlowsDir: "flows",
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []coreproj.Collection{
			{
				Path:   "src/*.xlf",
				Format: &coreproj.FormatSpec{Name: "xliff"},
				Target: "out/{lang}/*.xlf",
			},
		},
		Automations: rules,
	}
	proj, err := project.InitProject(real, recipe)
	require.NoError(t, err)

	src := filepath.Join(real, "src", "app.xlf")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(automationXLIFF), 0o644))

	flowsDir := proj.FlowsDirPath()
	require.NoError(t, os.MkdirAll(flowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(flowsDir, "guard.yaml"), []byte(guardDirFlow), 0o644))

	found, err := project.FindProject(real)
	require.NoError(t, err)
	return found
}

// The flow runs over the collection and its findings gate the push, the same
// contract an inline recipe flow has.
func TestDirFlow_RunsOverTheCollectionsAndGates(t *testing.T) {
	automationApp(t)
	proj := dirFlowFixture(t,
		runFlowRule("checks-gate", project.HookPrePush, map[string]string{"flow": "guard", "fail_on_error": "true"}))
	cmd, stdout, _ := hookCmd(t)

	err := runLocalAutomations(cmd, proj, project.HookPrePush)
	require.Error(t, err)
	assert.Equal(t, cli.ExitGate, cli.ExitCode(cmd, err))
	assert.Contains(t, err.Error(), `flow "guard" found 1 finding(s), 1 failing`)
	assert.Contains(t, stdout.String(), "Acme Cloud")
}

// The flow name is never a path: nothing named after the flow is opened or
// written, and the collection's source file is left as it was.
func TestDirFlow_OpensNoFileNamedAfterTheFlow(t *testing.T) {
	automationApp(t)
	proj := dirFlowFixture(t,
		runFlowRule("checks", project.HookPrePush, map[string]string{"flow": "guard"}))
	src := filepath.Join(proj.Root, "src", "app.xlf")
	before, err := os.ReadFile(src)
	require.NoError(t, err)

	cmd, _, _ := hookCmd(t)
	require.NoError(t, runLocalAutomations(cmd, proj, project.HookPrePush))

	assert.NoFileExists(t, filepath.Join(proj.Root, "guard"))
	after, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the run wrote over its own input")
}
