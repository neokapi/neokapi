package project

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/registry"
)

// resolvedClaim is a ResolvedFile with its item held by value, so two
// resolutions of one file compare equal.
type resolvedClaim struct {
	Path, Relative, Format, Collection, Pattern string
	CollectionIndex, ItemIndex                  int
	Item                                        ContentItem
}

func claims(files []ResolvedFile) []resolvedClaim {
	out := make([]resolvedClaim, 0, len(files))
	for _, f := range files {
		c := resolvedClaim{Path: f.Path, Relative: f.Relative, Format: f.Format, Collection: f.Collection,
			Pattern: f.Pattern, CollectionIndex: f.CollectionIndex, ItemIndex: f.ItemIndex}
		if f.Item != nil {
			c.Item = *f.Item
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b resolvedClaim) int { return strings.Compare(a.Relative, b.Relative) })
	return out
}

// TestResolvePaths_AgreesWithResolveContent resolves from their paths alone the
// files ResolveContent finds on disk, and resolves a path no file holds by the
// same rule.
func TestResolvePaths_AgreesWithResolveContent(t *testing.T) {
	dir := t.TempDir()
	onDiskFiles := []string{"docs/api.md", "docs/guide.md", "docs/drafts/wip.md", "docs/ignored.md", "store/ui.json", "store/skip.json", "notes.txt"}
	for _, f := range onDiskFiles {
		createFile(t, dir, f, "x")
	}
	createFile(t, dir, ".kapiignore", "docs/ignored.md\n")
	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "markdown", ".md")
	registerBuiltIn(reg, "json", ".json")
	proj := &KapiProject{
		Version:  CurrentVersion,
		Defaults: Defaults{Exclude: []string{"**/drafts/**"}},
		Collections: []Collection{
			{Name: "Reference", Content: []ContentItem{{Path: "docs/api.md", Channel: "acme/reference"}}},
			{Name: "Docs", Content: []ContentItem{{Path: "docs/**/*.md"}}},
			{Name: "Store", Content: []ContentItem{{Path: "store/ui.json", Format: &FormatSpec{Name: "markdown"}}}},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, RecipeFileName))

	onDisk, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	require.Len(t, onDisk, 3, "api.md, guide.md and ui.json; the draft is excluded and the ignored page ignored")
	assert.Equal(t, claims(onDisk), claims(ctx.ResolvePaths(reg, onDiskFiles, nil)))

	absent := ctx.ResolvePaths(reg, []string{"docs/new/page.md", "docs/drafts/new.md", "../outside.md", "/abs/page.md", "store/other.json"}, nil)
	require.Len(t, absent, 1, "only the page the Docs pattern claims: %+v", absent)
	assert.Equal(t, resolvedClaim{
		Path: filepath.Join(dir, "docs", "new", "page.md"), Relative: filepath.FromSlash("docs/new/page.md"), Format: "markdown",
		Collection: "Docs", Pattern: "docs/**/*.md", CollectionIndex: 1, Item: ContentItem{Path: "docs/**/*.md"},
	}, claims(absent)[0])
}

// TestResolvePaths_SniffsTheContentItIsGiven detects the format of a path no
// disk holds from the content supplied for it.
func TestResolvePaths_SniffsTheContentItIsGiven(t *testing.T) {
	reg := registry.NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader { return nil },
		format.FormatSignature{Extensions: []string{".html"}, Sniff: func(b []byte) bool { return bytes.Contains(b, []byte("<html")) }}, "HTML")
	reg.RegisterReader("ahtml", func() format.DataFormatReader { return nil },
		format.FormatSignature{Extensions: []string{".html"}, Sniff: func(b []byte) bool { return bytes.Contains(b, []byte("<alt-html")) }}, "Alt HTML")
	dir := t.TempDir()
	createFile(t, dir, "site/page.html", "<alt-html>on disk</alt-html>")
	proj := &KapiProject{Version: CurrentVersion, Collections: []Collection{{Path: "site/*.html"}}}
	ctx := NewProjectContext(proj, filepath.Join(dir, RecipeFileName))

	var asked []string
	content := func(rel string) (io.ReadSeeker, error) {
		asked = append(asked, rel)
		return strings.NewReader("<html><body>The page as a commit holds it.</body></html>"), nil
	}
	got := ctx.ResolvePaths(reg, []string{"site/page.html"}, content)
	require.Len(t, got, 1)
	assert.Equal(t, "html", got[0].Format, "the content given decides between the formats claiming .html, not the file on disk")
	assert.Equal(t, []string{"site/page.html"}, asked)

	require.NoError(t, os.Remove(filepath.Join(dir, "site", "page.html")))
	assert.Equal(t, "ahtml", ctx.ResolvePaths(reg, []string{"site/page.html"}, nil)[0].Format,
		"must fail: with no content the extension and priority decide")
}
