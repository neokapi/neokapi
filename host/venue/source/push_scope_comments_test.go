package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/formats"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// A push is authoritative over the patterns its scope declares, and the venue
// deletes what the scope covers and the push did not send. An item that holds
// nothing a push sends must declare no pattern, or the venue could delete items
// the push never read.

// commentsFixture is a project with translated JSON content, an item declared
// for its comments alone, an item declaring `comments: true` over Go files no
// format reads, and an item declaring `comments: true` over YAML files a reader
// parses, each with one file on disk, and an item declared for its comments
// alone whose files are all gone.
func commentsFixture(t *testing.T) *BowrainSourceConnector {
	t.Helper()
	spelled := func(s string) coreproj.ContentComments {
		var c coreproj.ContentComments
		require.NoError(t, yaml.Unmarshal([]byte(s), &c))
		return c
	}
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Collections: []coreproj.Collection{
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
			{Name: "code", Content: []coreproj.ContentItem{{Path: "code/**/*.go", Comments: spelled("{only: true}")}}},
			{Name: "legacy", Content: []coreproj.ContentItem{{Path: "legacy/*.go", Comments: spelled("true")}}},
			{Name: "config", Content: []coreproj.ContentItem{{Path: "config/*.yaml", Comments: spelled("true")}}},
			{Name: "notes", Content: []coreproj.ContentItem{{Path: "notes/*.md", Comments: spelled("{only: true}")}}},
		},
	})
	require.NoError(t, err)
	for rel, body := range map[string]string{
		"locales/en.json": `{"greeting":"Hello"}`,
		"code/parse/x.go": "package parse\n\n// Parse reads the input.\nfunc Parse() {}\n",
		"legacy/old.go":   "package legacy\n\n// Old stays.\nfunc Old() {}\n",
		"config/app.yaml": "# Greets the reader.\ngreeting: Hello\n",
	} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
	}
	conn := NewLocalConnector(&host.App{}, proj, reg)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestPushScopeLeavesOutItemsDeclaredForTheirCommentsAlone(t *testing.T) {
	scope := []string(commentsFixture(t).pushScope(nil))
	assert.Contains(t, scope, "locales/en.json", "translated content declares its pattern")
	assert.Contains(t, scope, "config/*.yaml", "comments: true over files a reader parses keeps its values, and its pattern")
	assert.NotContains(t, scope, "code/**/*.go", "an item declared comments: {only: true} holds nothing a push sends")
	assert.NotContains(t, scope, "notes/*.md", "a comments-only item whose files are all gone declares nothing either")
}

func TestPushScopeLeavesOutItemsWhoseFilesNoReaderOpens(t *testing.T) {
	scope := []string(commentsFixture(t).pushScope(nil))
	assert.NotContains(t, scope, "legacy/*.go", "comments: true over files no format reads holds nothing a push sends")
	assert.Contains(t, scope, "locales/en.json")
}

func TestPushScopeLeavesOutNamedCommentsOnlyFiles(t *testing.T) {
	conn := commentsFixture(t)
	scope := []string(conn.pushScope([]string{"locales/en.json", "code/parse/x.go", "legacy/old.go", "config/app.yaml", "code"}))
	assert.Contains(t, scope, "locales/en.json")
	assert.Contains(t, scope, "config/app.yaml", "a named file with values stays in scope")
	assert.Contains(t, scope, "code", "a named directory is authority the caller asked for")
	assert.NotContains(t, scope, "code/parse/x.go", "a named file declared for its comments alone holds nothing a push sends")
	assert.NotContains(t, scope, "legacy/old.go", "a named file no format reads holds nothing a push sends")
}

// A recipe whose content does not resolve gives no claims to read, and an item's
// own declaration still keeps its pattern, and a file named under it, out.
func TestPushScopeKeepsTheDeclarationWhenTheRecipeDoesNotResolve(t *testing.T) {
	var only coreproj.ContentComments
	require.NoError(t, yaml.Unmarshal([]byte("{only: true}"), &only))
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Collections: []coreproj.Collection{
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
			{Name: "broken", Content: []coreproj.ContentItem{{Path: "broken/[.json"}}},
			{Name: "notes", Content: []coreproj.ContentItem{{Path: "notes/*.md", Comments: only}}},
		},
	})
	require.NoError(t, err)
	for rel, body := range map[string]string{"locales/en.json": `{"greeting":"Hello"}`, "notes/a.md": "<!-- A note. -->\n# Notes\n"} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
	}
	conn := NewLocalConnector(&host.App{}, proj, reg)
	t.Cleanup(func() { conn.Close() })
	_, rerr := conn.projectContext().ResolveContent(reg)
	require.Error(t, rerr, "the fixture's recipe does not resolve")

	assert.NotContains(t, []string(conn.pushScope(nil)), "notes/*.md")
	named := []string(conn.pushScope([]string{"locales/en.json", "notes/a.md"}))
	assert.Contains(t, named, "locales/en.json")
	assert.NotContains(t, named, "notes/a.md")
}

// An item that claims a file a reader parses holds values, so it keeps its
// pattern whichever of its files the recipe resolves first.
func TestPushScopeKeepsItemsWithValues(t *testing.T) {
	var beside coreproj.ContentComments
	require.NoError(t, yaml.Unmarshal([]byte("true"), &beside))
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Collections: []coreproj.Collection{
			{Name: "comments-first", Content: []coreproj.ContentItem{{Path: "first/*", Comments: beside}}},
			{Name: "values-first", Content: []coreproj.ContentItem{{Path: "second/*", Comments: beside}}},
		},
	})
	require.NoError(t, err)
	for rel, body := range map[string]string{
		"first/a.go":    "package first\n\n// A reads.\nfunc A() {}\n",
		"first/b.yaml":  "# B.\ngreeting: Hello\n",
		"second/a.yaml": "# A.\ngreeting: Hello\n",
		"second/b.go":   "package second\n\n// B reads.\nfunc B() {}\n",
	} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
	}
	conn := NewLocalConnector(&host.App{}, proj, reg)
	t.Cleanup(func() { conn.Close() })

	scope := []string(conn.pushScope(nil))
	assert.Contains(t, scope, "first/*", "a comments-only file resolved before a file with values")
	assert.Contains(t, scope, "second/*", "a file with values resolved before a comments-only file")
}
