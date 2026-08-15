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

// The first thing a newcomer does after `kapi init`, driven end to end: the
// scaffolded content recipe, the collection `kapi add` writes into it, and the
// `check` flow the scaffold ships — on a project with no target languages,
// which is the only shape the content scaffold produces.

// scaffoldContentProject runs the real InitProject content scaffold, points its
// collection at two markdown files, and returns the App and recipe path.
func scaffoldContentProject(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md).
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	res, err := InitProject(dir, InitOptions{Name: "demo"})
	require.NoError(t, err)

	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "index.md"),
		[]byte("# Index\n\nWe leverage synergy to circle back.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(docs, "operations.md"),
		[]byte("# Operations\n\nRun the thing, then run it again.\n"), 0o644))

	// What `kapi add docs/**/*.md` writes: a collection with a pattern and a
	// format, and NO target — the shape `kapi add` can produce today.
	proj, err := project.Load(res.RecipePath)
	require.NoError(t, err)
	proj.Collections = append(proj.Collections, project.Collection{
		Name: "docs", Path: "docs/**/*.md", Format: &project.FormatSpec{Name: "markdown"},
	})
	require.NoError(t, project.Save(res.RecipePath, proj))

	a := &App{}
	a.InitRegistries()
	a.Quiet = true
	return a, res.RecipePath, dir
}

// runFlow drives `kapi run <flow>` over the project, with the flags the project
// flow path reads. A target language given here lands both on the flag (which
// is what marks it Changed) and on the App, as the CLI's flag binding does.
func runFlow(t *testing.T, a *App, flowName, recipe, targetLang string) error {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "run")
	fs := cmd.Flags()
	fs.String("target-lang", "", "")
	fs.String("source-lang", "", "")
	fs.String("output", "", "")
	fs.String("encoding", "", "")
	fs.String("trace", "", "")
	fs.StringSlice("input", nil, "")
	fs.Int("concurrency", 0, "")
	fs.Bool("explain", false, "")
	a.TargetLang = targetLang
	if targetLang != "" {
		require.NoError(t, fs.Set("target-lang", targetLang))
	}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return a.RunFromProject(cmd, flowName, recipe, RunCmdOptions{})
}

// markdownFiles lists the content files the collection glob tracks, which is
// the number that grew on every run.
func markdownFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "docs"))
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// The scaffold's own suggestion has to work on the project the scaffold just
// created. `check`'s single step reads source text, so the flow is monolingual
// and needs no target language — a project with no target languages is exactly
// what the content scaffold writes.
func TestScaffoldedCheckFlowRunsOnItsOwnProject(t *testing.T) {
	a, recipe, dir := scaffoldContentProject(t)

	err := runFlow(t, a, "check", recipe, "")
	require.NoError(t, err, "`kapi run check` must run on the monolingual project `kapi init` scaffolds")

	assert.ElementsMatch(t, []string{"index.md", "operations.md"}, markdownFiles(t, dir),
		"a check flow annotates; it writes no documents")
}

// Passing the target language the guard used to demand is the other way in, and
// it is the one that multiplied data: a sibling written beside each file sits
// inside the collection glob that supplied it, so the glob re-tracks it as
// source. Three runs, same two files.
func TestCheckRunWithTargetLangDoesNotGrowTheProject(t *testing.T) {
	a, recipe, dir := scaffoldContentProject(t)

	for pass := 1; pass <= 3; pass++ {
		require.NoError(t, runFlow(t, a, "check", recipe, "en"), "pass %d", pass)
		assert.ElementsMatch(t, []string{"index.md", "operations.md"}, markdownFiles(t, dir),
			"pass %d wrote into the tracked collection", pass)
	}
}

// A single-file project took the process-only path already; the batch path is
// the one that wrote. Both file counts have to behave the same way, or the bug
// returns the moment a project gains a second file.
func TestCheckRunWritesNothingForOneFileOrMany(t *testing.T) {
	a, recipe, dir := scaffoldContentProject(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "docs", "operations.md")))

	require.NoError(t, runFlow(t, a, "check", recipe, "en"))
	assert.Equal(t, []string{"index.md"}, markdownFiles(t, dir))
}

// The state directory is meant to be committed, so the rule that keeps the
// store, the caches and the redaction vault out of the commit has to be there
// from the moment the directory is.
func TestInitProjectWritesTheStateIgnoreRule(t *testing.T) {
	dir := t.TempDir()
	res, err := InitProject(dir, InitOptions{Name: "demo"})
	require.NoError(t, err)

	path := filepath.Join(res.StateDir, project.StateGitignoreFilename)
	content, rerr := os.ReadFile(path)
	require.NoError(t, rerr, "`kapi init` must write %s", path)
	assert.Equal(t, project.StateGitignore, string(content))
	assert.Contains(t, string(content), project.WorkDirName+"/",
		"work/ holds the store, the caches and the redaction vault")
}
