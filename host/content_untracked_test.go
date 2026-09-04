package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// untrackedFixture writes a project whose recipe governs `docs/` and the app
// catalog, alongside content the recipe never mentions: a support surface that
// appeared afterwards, a target file the catalog produces, a binary nothing can
// read, and the governance documents themselves.
func untrackedFixture(t *testing.T) (*project.KapiProject, string) {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("docs/index.md", "# Docs\n")
	write("docs/berths.md", "# Berths\n")
	write("app/en.json", `{"a":"Alert"}`)
	write("app/nb.json", `{"a":"Varsel"}`)
	write("support/faq.md", "# FAQ\n")
	write("support/escalation.md", "# Escalation\n")
	write("build/report.bin", "\x00\x01")
	write("vendor/bundled.md", "# Vendored\n")
	write("voice.yaml", "name: Northsea\n")
	write("terms.json", `{"schemaVersion":"1.0","kind":"kapi-terms","concepts":[]}`)

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Exclude:         []string{"vendor/**"},
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
			TermsSource:     "terms.json",
		},
		Collections: []project.Collection{
			{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
			{Name: "app", Path: "app/en.json", Target: "app/{lang}.json"},
		},
	}
	require.NoError(t, project.Save(filepath.Join(root, project.RecipeFileName), proj))
	return proj, root
}

func untrackedPaths(t *testing.T, prefixes ...string) []string {
	t.Helper()
	app := &App{}
	app.InitRegistries()
	proj, root := untrackedFixture(t)

	res, err := app.UntrackedContent(proj, filepath.Join(root, project.RecipeFileName), prefixes)
	require.NoError(t, err)
	assert.True(t, res.Untracked, "the document must say which question it answers")
	assert.Equal(t, len(res.Files), res.Total)

	paths := make([]string, 0, len(res.Files))
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

// TestUntrackedContent_ReportsWhatNoCollectionGoverns is the refresh question:
// content on disk that the recipe never declared. Every other listing starts
// from the collections, so a surface added after the recipe was written is
// visible to nothing until this subtraction names it.
func TestUntrackedContent_ReportsWhatNoCollectionGoverns(t *testing.T) {
	assert.Equal(t, []string{"support/escalation.md", "support/faq.md"}, untrackedPaths(t))
}

// TestUntrackedContent_Omissions: each thing the report must NOT contain, and
// the reason it would be wrong. A false positive here is expensive — it invites
// a second collection over content the recipe already governs, or a collection
// over a file kapi cannot read.
func TestUntrackedContent_Omissions(t *testing.T) {
	paths := untrackedPaths(t)
	cases := []struct {
		path string
		why  string
	}{
		{"docs/index.md", "a file a collection pattern matches is tracked"},
		{"app/en.json", "a bare content entry tracks its file"},
		{"app/nb.json", "a target the recipe produces is governed content, not an undeclared surface"},
		{"build/report.bin", "kapi has no reader for the extension, so there is no collection to propose"},
		{"vendor/bundled.md", "the project's exclude list is honoured"},
		{project.RecipeFileName, "the recipe is governance, not governed content"},
		{"voice.yaml", "the bound voice profile is context the project applies, not content it holds"},
		{"terms.json", "the bound terms source is context the project applies, not content it holds"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			assert.NotContains(t, paths, tc.path, tc.why)
		})
	}
}

// TestUntrackedContent_NarrowsToASubtree: the report takes the same path
// arguments as `kapi ls`, so a refresh can ask about one directory.
func TestUntrackedContent_NarrowsToASubtree(t *testing.T) {
	assert.Empty(t, untrackedPaths(t, "docs/"))
	assert.Equal(t, []string{"support/escalation.md", "support/faq.md"}, untrackedPaths(t, "support"))
}

// TestUntrackedContent_DetectsTheFormat: a proposal needs the format the
// collection would declare, not only the path.
func TestUntrackedContent_DetectsTheFormat(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	proj, root := untrackedFixture(t)

	res, err := app.UntrackedContent(proj, filepath.Join(root, project.RecipeFileName), nil)
	require.NoError(t, err)
	require.NotEmpty(t, res.Files)
	assert.Equal(t, "markdown", res.Files[0].Format)
}

// TestUntrackedContent_RelativeRecipePath: `kapi ls --untracked` inside a
// project resolves its recipe relative to the working directory, which makes
// the walk root ".". The report was empty and exit 0 — indistinguishable from a
// project with nothing to declare.
func TestUntrackedContent_RelativeRecipePath(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	proj, root := untrackedFixture(t)
	t.Chdir(root)

	res, err := app.UntrackedContent(proj, project.RecipeFileName, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Total, "a relative recipe path reports the same files as an absolute one")
}

// TestUntrackedContent_RefusesABrokenPattern: the tracked set is used by
// subtraction, so a pattern that cannot expand does not shorten the report — it
// reports an entire declared collection as untracked. Failing names the typo;
// succeeding proposes duplicate collections over governed content.
func TestUntrackedContent_RefusesABrokenPattern(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	proj, root := untrackedFixture(t)
	proj.Collections[0].Content[0].Path = "docs/[unclosed*.md"

	_, err := app.UntrackedContent(proj, filepath.Join(root, project.RecipeFileName), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docs")
	assert.Contains(t, err.Error(), "cannot be expanded")
}

// TestUntrackedContent_OnlyTheClaimingItemsTargetsAreTracked: a source is
// claimed by the first item that matches it, so only that item's targets are
// files the loop writes. A copy at a later item's target is content nothing
// governs, and the report says so.
func TestUntrackedContent_OnlyTheClaimingItemsTargetsAreTracked(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("docs/index.md", "# Index\n")
	write("docs/berths.md", "# Berths\n")
	write("i18n/nb/index.md", "# Indeks\n")
	write("i18n/nb/berths.md", "# Kaiplasser\n")

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{Name: "docs", Content: []project.ContentItem{
				{Path: "docs/index.md"},
				{Path: "docs/**/*.md", Target: "i18n/{lang}/{path}.md"},
			}},
		},
	}
	require.NoError(t, project.Save(filepath.Join(root, project.RecipeFileName), proj))

	app := &App{}
	app.InitRegistries()
	res, err := app.UntrackedContent(proj, filepath.Join(root, project.RecipeFileName), nil)
	require.NoError(t, err)
	paths := make([]string, 0, len(res.Files))
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"i18n/nb/index.md"}, paths,
		"index.md is source-only, so the glob's target for it is a file nothing produces")
}
