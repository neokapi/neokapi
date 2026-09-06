package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end over the real `kapi run <flow>` command, for a flow that lives in
// its own file under .kapi/flows/ rather than inline on the recipe. Both are a
// project flow: the run resolves the recipe's collections, applies its format
// bindings and locale passes, and reports what the flow's check steps found.
//
// This drives the cobra command, so the wiring is what is under test.

const guardFileFlow = `name: guard
description: Check the do-not-translate list

steps:
  - tool: dnt-check
    config:
      terms:
        - Acme Cloud
`

// dirFlowProjectFixture writes a recipe with one XLIFF collection and no
// inline flows, plus .kapi/flows/guard.yaml holding the flow.
func dirFlowProjectFixture(t *testing.T, flowYAML string) (recipe, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "DirFlowTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{
				Path:   "src/*.xlf",
				Format: &project.FormatSpec{Name: "xliff"},
				Target: "out/{lang}/*.xlf",
			},
		},
	}))

	flowsDir := project.LayoutAt(real).FlowsDir()
	require.NoError(t, os.MkdirAll(flowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(flowsDir, "guard.yaml"), []byte(flowYAML), 0o644))

	srcAbs := filepath.Join(real, "src", "app.xlf")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	require.NoError(t, os.WriteFile(srcAbs, []byte(guardXLIFF), 0o644))
	return recipe, real
}

// The flow runs over the collection and reports its findings, the same as an
// inline recipe flow.
func TestRunCmd_FlowsDirectoryFlowRunsOverTheProject(t *testing.T) {
	recipe, root := dirFlowProjectFixture(t, guardFileFlow)

	out, err := runRunCmd(t, processOnlyApp(t), recipe, "guard", "--target-lang", "nb")
	require.NoError(t, err, out)

	assert.Contains(t, out, "CRITICAL")
	assert.Contains(t, out, "Acme Cloud")
	assert.Contains(t, out, "1 finding(s) (1 critical")
	assert.Contains(t, out, "app.xlf", "the finding names the file it is in")
	assert.NoFileExists(t, filepath.Join(root, "guard"),
		"the flow name is a flow, and a run must not read or write a file by that name")
}

// A step that names a file is rejected with the reason, rather than running the
// tools over that file outside the project.
func TestRunCmd_FlowsDirectoryStepNamingAPathIsRejected(t *testing.T) {
	const withPaths = `name: guard
steps:
  - tool: dnt-check
    input: "src/app.xlf"
    output: "src/app.xlf"
`
	recipe, _ := dirFlowProjectFixture(t, withPaths)

	out, err := runRunCmd(t, processOnlyApp(t), recipe, "guard", "--target-lang", "nb")
	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "names an input path")
	assert.Contains(t, err.Error(), "--input")
}

// A name matching neither a built-in flow, an inline one, nor a file says so.
func TestRunCmd_UnknownProjectFlowIsReported(t *testing.T) {
	recipe, _ := dirFlowProjectFixture(t, guardFileFlow)

	_, err := runRunCmd(t, processOnlyApp(t), recipe, "nowhere", "--target-lang", "nb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `flow "nowhere" not found`)
}
