package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A flow under .kapi/flows/ is a project flow, so `kapi run <flow>` resolves it
// through the project runner: the recipe's collections supply the files, the
// recipe's format bindings read them, and the run writes where a recipe flow's
// run writes. A run that treated the flow name as a file path instead opened a
// file named after the flow, or ran the tools over one file with no project
// context and wrote the result back over its own input.

// dirFlowProject writes a project with one collection of MDX pages and one
// flow file under .kapi/flows/, and returns the App, the recipe path and the
// project root.
func dirFlowProject(t *testing.T, flowName, flowYAML string) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md).
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "page.md"), []byte(mdxPage), 0o644))

	flowsDir := project.LayoutAt(dir).FlowsDir()
	require.NoError(t, os.MkdirAll(flowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(flowsDir, flowName+".yaml"), []byte(flowYAML), 0o644))

	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "dir-flow",
		Collections: []project.Collection{{
			Name:   "docs",
			Path:   "docs/**/*.md",
			Format: &project.FormatSpec{Name: "mdx"},
		}},
	}))

	a := &App{}
	a.InitRegistries()
	a.Quiet = true
	return a, recipe, dir
}

// runProjectFlowCmd drives `kapi run <flow> -p <recipe>` with the flags the run
// command carries, optionally naming an output path.
func runProjectFlowCmd(t *testing.T, a *App, flowName, recipe, out, targetLang string) error {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), flowName)
	fs := cmd.Flags()
	fs.String("target-lang", "", "")
	fs.String("source-lang", "", "")
	fs.String("output", "", "")
	fs.String("encoding", "", "")
	fs.String("trace", "", "")
	fs.String("format", "", "")
	fs.StringSlice("input", nil, "")
	fs.Int("concurrency", 0, "")
	fs.Bool("explain", false, "")
	if out != "" {
		require.NoError(t, fs.Set("output", out))
	}
	if targetLang != "" {
		require.NoError(t, fs.Set("target-lang", targetLang))
		a.TargetLang = targetLang
	}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return a.RunFromProject(cmd, flowName, recipe, RunCmdOptions{})
}

const pseudoDirFlow = `name: pseudo
description: Generate pseudo-translations for testing

steps:
  - tool: pseudo-translate
    config:
      method: extended
`

// The flow runs over the recipe's collections and reads each file under the
// format the collection binds.
func TestRunFromProject_DirFlowRunsOverTheCollections(t *testing.T) {
	a, recipe, dir := dirFlowProject(t, "pseudo", pseudoDirFlow)
	out := filepath.Join(dir, "page.qps.md")

	require.NoError(t, runProjectFlowCmd(t, a, "pseudo", recipe, out, "qps"))

	got, err := os.ReadFile(out)
	require.NoError(t, err, "the run covers the collection; -o names where it writes")
	text := string(got)
	assert.Contains(t, text, `import { Thing } from "@site/src/components/Thing";`,
		"the collection's mdx binding was not applied")
	assert.NotContains(t, text, "Ordinary prose that should be translated.",
		"the prose was left alone, so the flow did not run")
}

// The flow name is a flow, never a path. A run that fell through to the bare
// executor opened a file named after the flow and failed on the missing file.
func TestRunFromProject_DirFlowOpensNoFileNamedAfterItself(t *testing.T) {
	a, recipe, dir := dirFlowProject(t, "guard", pseudoDirFlow)

	err := runProjectFlowCmd(t, a, "guard", recipe, filepath.Join(dir, "out.md"), "qps")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "guard"))
}

// With no -o the run is process-only, as a recipe flow's run is: overlays go to
// the project store and the source file is left alone.
func TestRunFromProject_DirFlowWithoutOutputLeavesTheSource(t *testing.T) {
	a, recipe, dir := dirFlowProject(t, "pseudo", pseudoDirFlow)
	source := filepath.Join(dir, "docs", "page.md")
	before, err := os.ReadFile(source)
	require.NoError(t, err)

	require.NoError(t, runProjectFlowCmd(t, a, "pseudo", recipe, "", "qps"))

	after, err := os.ReadFile(source)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the run wrote over its own input")

	entries, err := os.ReadDir(filepath.Join(dir, "docs"))
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"page.md"}, names, "the run emitted a file beside the source")
}

// A flow declared inline on the recipe wins over a file of the same name, so a
// recipe stays the first place a project's own declarations are read from.
func TestRunFromProject_RecipeFlowWinsOverTheFlowsDirectory(t *testing.T) {
	a, recipe, dir := dirFlowProject(t, "pseudo", "name: pseudo\nsteps:\n  - tool: no-such-tool\n")

	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	loaded.Flows = map[string]*flow.StepsSpec{
		"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
	}
	require.NoError(t, project.Save(recipe, loaded))

	out := filepath.Join(dir, "page.qps.md")
	require.NoError(t, runProjectFlowCmd(t, a, "pseudo", recipe, out, "qps"))
	assert.FileExists(t, out, "the file-per-flow definition ran, and its tool does not exist")
}

// An unknown flow reports that the project has no such flow rather than trying
// to open one.
func TestRunFromProject_UnknownFlowIsReported(t *testing.T) {
	a, recipe, _ := dirFlowProject(t, "pseudo", pseudoDirFlow)

	err := runProjectFlowCmd(t, a, "nowhere", recipe, "", "qps")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nowhere")
}

// A step that names a file is rejected with the reason: a flow's steps declare
// tools, and the run's files come from the recipe or from --input.
func TestRunFromProject_DirFlowStepNamingAPathIsRejected(t *testing.T) {
	const withPaths = `name: pseudo
steps:
  - tool: pseudo-translate
    input: "locales/en.json"
    output: "locales/qps.json"
`
	a, recipe, _ := dirFlowProject(t, "pseudo", withPaths)

	err := runProjectFlowCmd(t, a, "pseudo", recipe, "", "qps")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "names an input path")
	assert.Contains(t, err.Error(), "--input")
}
