package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A content item that names no `target:` has no destination in any language.
// Its files extract into the project store with the rest of the source, and no
// locale pass runs over them, so a convergence run writes nothing beside them.

const noTargetConfig = "info:\n  description: The desktop app for the tide tables\n"

// noTargetProject is a project with one translated file the seeded content
// memory answers, a source-only collection over build/config.yml, and the
// collections in extra after them.
func noTargetProject(t *testing.T, extra ...project.Collection) (*App, *EnvCommand, string, string) {
	t.Helper()
	colls := append([]project.Collection{
		{
			Name: "app",
			Content: []project.ContentItem{{
				Path:   "src/app.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "site/locales/{lang}.json",
			}},
		},
		{
			Name:       "build",
			SourceOnly: true,
			Content: []project.ContentItem{{
				Path:   "build/config.yml",
				Format: &project.FormatSpec{Name: "yaml"},
			}},
		},
	}, extra...)
	a, cmd, recipe, dir := recipeFormatProject(t, map[string]string{
		"src/app.json":     `{"title":"Tide window"}`,
		"build/config.yml": noTargetConfig,
		"build/other.yml":  "# Settings for the installer build.\nname: tides\n",
	}, colls, nil)
	seedMemory(t, a, recipe, map[string]string{"Tide window": "Tidevannsvindu"})
	return a, cmd, recipe, dir
}

// TestConverge_WritesNothingBesideANoTargetFile: the translated file is
// delivered, the source-only file gets no sibling in the target language, and
// both files extract into the project store.
func TestConverge_WritesNothingBesideANoTargetFile(t *testing.T) {
	a, cmd, recipe, dir := noTargetProject(t)

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged)
	body, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nb.json"))
	require.NoError(t, err, "the translated file is delivered")
	assert.Contains(t, string(body), "Tidevannsvindu")

	assert.NoFileExists(t, filepath.Join(dir, "build", "config_nb.yml"),
		"a file whose item names no target gets no file in any language")
	assert.Equal(t, 2, out.ExtractedFiles, "the source-only file extracts beside the translated one")
}

// TestConverge_ANoTargetFileUnderACommentPattern: a comments-only collection
// whose pattern covers the source-only file's directory, and so every sibling
// a pass could write there, leaves the run converging.
func TestConverge_ANoTargetFileUnderACommentPattern(t *testing.T) {
	a, cmd, recipe, dir := noTargetProject(t, project.Collection{
		Name:       "build-comments",
		SourceOnly: true,
		Content: []project.ContentItem{{
			Path:     "build/*.yml",
			Comments: project.ContentComments{Declared: true, Only: true},
		}},
	})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged)
	assert.FileExists(t, filepath.Join(dir, "site", "locales", "nb.json"))
	assert.NoFileExists(t, filepath.Join(dir, "build", "config_nb.yml"))
}

// TestConverge_ARecipeOfNoTargetFilesStillRuns: a recipe that names target
// languages and holds no file with a target is still a run over its source,
// never one with no content to catch up.
func TestConverge_ARecipeOfNoTargetFilesStillRuns(t *testing.T) {
	a, cmd, recipe, dir := recipeFormatProject(t,
		map[string]string{"build/config.yml": noTargetConfig},
		[]project.Collection{{
			Name:       "build",
			SourceOnly: true,
			Content: []project.ContentItem{{
				Path:   "build/config.yml",
				Format: &project.FormatSpec{Name: "yaml"},
			}},
		}}, nil)

	out := converge(t, a, cmd, recipe)
	assert.Equal(t, 1, out.ExtractedFiles, "the file extracts into the project store")
	assert.NoFileExists(t, filepath.Join(dir, "build", "config_nb.yml"))
}
