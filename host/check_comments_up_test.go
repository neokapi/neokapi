package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// declareGoComments adds a source-only collection over a Go file to a recipe,
// with or without `comments: true`.
func declareGoComments(t *testing.T, recipe string, comments bool) string {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Collections = append(proj.Collections, project.Collection{
		Name:       "code",
		SourceOnly: true,
		Content:    []project.ContentItem{{Path: "code/*.go", Comments: comments}},
	})
	require.NoError(t, project.Save(recipe, proj))
	goFile := filepath.Join(filepath.Dir(recipe), "code", "parse.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(goFile), 0o755))
	require.NoError(t, os.WriteFile(goFile, []byte("package code\n\n// Parse parses.\nfunc Parse() {}\n"), 0o644))
	return goFile
}

// A convergence run never reads a file declared for its comments. The run is
// strict about unreadable files, so a Go file reaching the flow would fail it;
// the second case declares the same file without `comments: true` and proves
// that it does.
func TestUpNeverConvergesACommentsOnlyCollection(t *testing.T) {
	t.Run("declared for its comments", func(t *testing.T) {
		a, cmd, recipe := newSelfSeedProject(t)
		goFile := declareGoComments(t, recipe, true)
		before, err := os.ReadFile(goFile)
		require.NoError(t, err)
		require.NoError(t, cmd.Flags().Set("fail-on-unknown", "true"))

		out := runFreshConverge(t, a, cmd, recipe)
		require.Len(t, out.Locales, 1)
		assert.Equal(t, 100, out.Locales[0].Pct["translated"], "the ordinary collection still converges")

		after, err := os.ReadFile(goFile)
		require.NoError(t, err)
		assert.Equal(t, before, after, "the Go source is untouched")
		matches, err := filepath.Glob(filepath.Join(filepath.Dir(goFile), "*"))
		require.NoError(t, err)
		assert.Equal(t, []string{goFile}, matches, "nothing was written beside it")
	})

	t.Run("must fail: the same file without comments: true reaches the flow", func(t *testing.T) {
		a, cmd, recipe := newSelfSeedProject(t)
		declareGoComments(t, recipe, false)
		require.NoError(t, cmd.Flags().Set("fail-on-unknown", "true"))
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 3, noChecks: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse.go")
	})
}

// The ship gate checks a comments-only collection through the same comment
// layer as `kapi check`, instead of failing to read the file.
func TestShipCheckReadsGoComments(t *testing.T) {
	root, _ := sourceShipFixture(t)
	recipe := filepath.Join(root, "kapi.yaml")
	goFile := declareGoComments(t, recipe, true)
	require.NoError(t, os.WriteFile(goFile, []byte("package code\n\n// Parse reads the the input.\nfunc Parse() {}\n"), 0o644))

	out, err := (&App{}).computeVerify(sourceShipCommand(t, root), nil)
	require.NoError(t, err)
	qa, ok := gateByName(out, gateChecks)
	require.True(t, ok)
	found := false
	for _, f := range qa.Findings {
		if filepath.Base(f.File) == "parse.go" && f.Block == "func/Parse" {
			found = true
		}
	}
	assert.True(t, found, "the doubled word in the Go comment is reported: %+v", qa.Findings)
	assert.Equal(t, 2, qa.Coverage.Files, "the JSON content and the Go file were both checked")
}
