package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/change"
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
// against content no convergence run ever touches.

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

// An edit rewrites the user's file, so the change service reads and writes it
// through the format the recipe names rather than the one the extension
// suggests: read as markdown, the mdx import line is a paragraph, and writing
// that back corrupts the page.
func TestApply_WritesThroughTheDeclaredFormat(t *testing.T) {
	app := NewApp()
	tabID, mdPath, _ := setupRecipeFormatProject(t, app)

	before, err := os.ReadFile(mdPath)
	require.NoError(t, err)

	page := readVia(t, app, tabID, change.ReadRequest{Doc: "docs/page.md"})
	var para *change.BlockRead
	for i, b := range page.Blocks {
		assert.NotContains(t, b.Text, "import {", "the mdx reader keeps the import line out of the prose")
		if b.Text == "A plain paragraph." {
			para = &page.Blocks[i]
		}
	}
	require.NotNil(t, para, "the mdx reader offers the body paragraph")

	res := applyVia(t, app, tabID, change.Set{Ops: []change.Op{setText(para.Ref, para.Rev, "An ordinary paragraph.")}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	after, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "An ordinary paragraph.")
	assert.Contains(t, string(after), "import { Preview } from '@site/src/seamless';",
		"the import line is structure to the reader the recipe names and must survive the write")
	assert.NotEqual(t, string(before), string(after))
}
