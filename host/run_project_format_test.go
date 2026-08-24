package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A built-in flow run against a project must read each file with the format the
// recipe binds to it, not the one its extension suggests.
//
// The two differ for exactly the reason this repo's own kapi.yaml gives: a
// Docusaurus `.md` is MDX, and the markdown reader treats an `import { … }`
// line as a paragraph of prose — so a run that falls back to the extension
// translates the ESM imports and the page stops compiling.

const mdxPage = `import { Thing } from "@site/src/components/Thing";

# Heading

Ordinary prose that should be translated.
`

// mdxBoundProject writes a project whose collection binds .md to the mdx
// reader, plus one page carrying an ESM import.
func mdxBoundProject(t *testing.T) (*App, string, string) {
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

	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "mdx-bound",
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

// runFlowOnFile drives `kapi <flow> -i <in> -o <out> -p <recipe>`, the shape
// that reaches RunFlow through RunFromProject's built-in-flow path.
func runFlowOnFile(t *testing.T, a *App, flowName, recipe, in, out, targetLang string) error {
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
	require.NoError(t, fs.Set("input", in))
	require.NoError(t, fs.Set("output", out))
	require.NoError(t, fs.Set("target-lang", targetLang))
	a.TargetLang = targetLang
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return a.RunFromProject(cmd, flowName, recipe, RunCmdOptions{})
}

func TestRunFromProject_BuiltinFlowHonoursCollectionFormat(t *testing.T) {
	a, recipe, dir := mdxBoundProject(t)
	in := filepath.Join(dir, "docs", "page.md")
	out := filepath.Join(dir, "page.qps.md")

	require.NoError(t, runFlowOnFile(t, a, "pseudo-translate", recipe, in, out, "qps"))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	text := string(got)

	// The recipe binds mdx, so the import is code and survives byte for byte.
	assert.Contains(t, text, `import { Thing } from "@site/src/components/Thing";`,
		"the ESM import was translated — the collection's mdx binding was not applied")

	// And the run did happen, so a pass that merely does nothing cannot succeed.
	assert.NotContains(t, text, "Ordinary prose that should be translated.",
		"the prose was left alone — the flow did not run")
}

// The same assertion for the other built-in flow that reaches this path, so a
// future change that special-cases one flow does not quietly regress the rest.
func TestRunFromProject_ProjectContextReachesBuiltinFlows(t *testing.T) {
	a, recipe, _ := mdxBoundProject(t)

	// RunFromProject clears ProjectContext on the way out, so observe it from
	// the flow itself: FallbackRunE runs inside the same call.
	var seen bool
	cmd := NewEnvCommand(context.Background(), "translate")
	fs := cmd.Flags()
	fs.String("target-lang", "", "")
	fs.String("source-lang", "", "")
	fs.String("output", "", "")
	fs.String("encoding", "", "")
	fs.String("trace", "", "")
	fs.StringSlice("input", nil, "")
	fs.Int("concurrency", 0, "")
	fs.Bool("explain", false, "")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := a.RunFromProject(cmd, "translate", recipe, RunCmdOptions{
		FallbackRunE: func(Command, string, []string) error {
			seen = a.ProjectContext != nil
			return nil
		},
	})
	require.NoError(t, err)
	assert.True(t, seen, "a built-in flow ran with no project context, so no "+
		"format binding or reader config could be resolved")
}

// A project-bound run with no explicit -o commits overlays and emits no file
// (E-04 §3). That default keys off the project context too, so a built-in flow
// that never received one wrote a sibling file instead.
func TestRunFromProject_BuiltinFlowIsProcessOnlyWithoutOutput(t *testing.T) {
	a, recipe, dir := mdxBoundProject(t)
	in := filepath.Join(dir, "docs", "page.md")

	cmd := NewEnvCommand(context.Background(), "pseudo-translate")
	fs := cmd.Flags()
	fs.String("target-lang", "", "")
	fs.String("source-lang", "", "")
	fs.String("output", "", "")
	fs.String("encoding", "", "")
	fs.String("trace", "", "")
	fs.StringSlice("input", nil, "")
	fs.Int("concurrency", 0, "")
	fs.Bool("explain", false, "")
	require.NoError(t, fs.Set("input", in))
	require.NoError(t, fs.Set("target-lang", "qps"))
	a.TargetLang = "qps"
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, a.RunFromProject(cmd, "pseudo-translate", recipe, RunCmdOptions{}))

	entries, err := os.ReadDir(filepath.Join(dir, "docs"))
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"page.md"}, names,
		"the run emitted a file next to the source; a project run with no -o "+
			"commits overlays to the store and materializes with `kapi merge`")
}
