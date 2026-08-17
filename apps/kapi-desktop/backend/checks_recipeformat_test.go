package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Checks panel is a gate, and a gate must read a file the way the loop does.
// Both halves of the recipe's binding decide that: the format the item declares
// (a `.md` bound to mdx, where an `import { … }` line is structure rather than a
// paragraph of prose) and the reader config the project declares for the format
// (keyPathPatterns, which say which YAML keys hold prose at all). Read under the
// extension's default format and reader defaults, the panel reports findings
// against content no convergence run ever touches — and its one-click fix
// rewrites the file through the wrong writer.

// setupRecipeFormatProject writes a project binding both halves and returns the
// tab plus the absolute paths of the two content files.
func setupRecipeFormatProject(t *testing.T, app *App) (tabID, mdPath, yamlPath string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "demos"), 0o755))

	// The voice profile forbids a word that appears only in content the recipe
	// excludes, so a finding announces a block that should never have been read.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"), []byte(`id: house
name: House Style
vocabulary:
  forbidden_terms:
    - term: seamless
      replacement: unified
      severity: major
`), 0o644))

	mdPath = filepath.Join(dir, "docs", "page.md")
	require.NoError(t, os.WriteFile(mdPath, []byte(`# Title

import { Preview } from '@site/src/seamless';

A plain paragraph.
`), 0o644))

	yamlPath = filepath.Join(dir, "demos", "demo.yaml")
	require.NoError(t, os.WriteFile(yamlPath, []byte(`title: Northsea governance
command: ksed -i 's/our seamless integration/our unified integration/' index.html
`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage: "en",
			Formats: map[string]project.FormatDefaults{
				"yaml": {Config: map[string]any{"keyPathPatterns": []any{"title"}}},
			},
		},
		Collections: []project.Collection{
			{
				Name: "docs",
				Content: []project.ContentItem{
					{Path: "docs/*.md", Format: &project.FormatSpec{Name: "mdx"}},
				},
			},
			{
				Name: "demos",
				Content: []project.ContentItem{
					{Path: "demos/*.yaml", Format: &project.FormatSpec{Name: "yaml"}},
				},
			},
		},
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))

	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, mdPath, yamlPath
}

func TestRunChecks_ReadsTheFormatAndConfigTheRecipeDeclares(t *testing.T) {
	app := NewApp()
	tabID, _, _ := setupRecipeFormatProject(t, app)

	res, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)
	require.NotNil(t, res)

	for _, f := range res.Files {
		for _, d := range f.Findings {
			assert.NotEqual(t, "io", d.Category, "every declared file must read: %s — %s", f.Path, d.Message)
			assert.NotContains(t, d.OriginalText, "seamless",
				"the recipe binds mdx and selects only `title`, so neither the import line nor the shell command is prose: %s", f.Path)
		}
	}
}

// The one-click fix rewrites the user's file, so it reads and writes it through
// the format the recipe names rather than the one the extension suggests.
func TestApplyCheckFix_UsesTheDeclaredFormat(t *testing.T) {
	app := NewApp()
	tabID, mdPath, _ := setupRecipeFormatProject(t, app)

	before, err := os.ReadFile(mdPath)
	require.NoError(t, err)

	// The mdx reader gives the body paragraph a block id the markdown reader
	// numbers differently, so a fix addressed by that id only lands when the
	// rewrite reads the file as the recipe declares it.
	blocks, err := app.readBlocksForChecks(t.Context(), mdPath, "mdx", nil, "en")
	require.NoError(t, err)
	var target string
	for _, b := range blocks {
		if b.Translatable && b.SourceText() == "A plain paragraph." {
			target = b.ID
		}
	}
	require.NotEmpty(t, target, "the mdx reader offers the body paragraph")

	require.NoError(t, app.ApplyCheckFix(tabID, mdPath, target, "source", "A plain paragraph.", "An ordinary paragraph."))

	after, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "An ordinary paragraph.")
	assert.Contains(t, string(after), "import { Preview } from '@site/src/seamless';",
		"the import line is structure to the reader the recipe names and must survive the rewrite")
	assert.NotEqual(t, string(before), string(after))
}
