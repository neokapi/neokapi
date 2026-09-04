package project

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claim is where one file is expected to resolve: the collection and item that
// claimed it, and the channel that item carries.
type claim struct {
	collection string
	collIdx    int
	item       int
	pattern    string
	channel    string
}

// TestResolveContent_FirstMatchingItemClaimsAFile pins the rule the recipes
// document: a file takes the first item that matches it, in recipe order across
// collections, and later items never see it. A file that matched two items used
// to resolve once per item, so one file was recorded at two points and the
// content memory read it as a surface disagreeing with itself (#2288).
func TestResolveContent_FirstMatchingItemClaimsAFile(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		collections []Collection
		want        map[string]claim
	}{
		{
			name:  "explicit path before glob: the explicit item claims its file",
			files: []string{"docs/api.md", "docs/guide.md"},
			collections: []Collection{{Name: "Docs", Channel: "acme/docs", Content: []ContentItem{
				{Path: "docs/api.md", Channel: "acme/reference"},
				{Path: "docs/**/*.md"},
			}}},
			want: map[string]claim{
				"docs/api.md":   {"Docs", 0, 0, "docs/api.md", "acme/reference"},
				"docs/guide.md": {"Docs", 0, 1, "docs/**/*.md", ""},
			},
		},
		{
			name:  "glob before explicit path: declaration order wins, the explicit item claims nothing",
			files: []string{"docs/api.md", "docs/guide.md"},
			collections: []Collection{{Name: "Docs", Channel: "acme/docs", Content: []ContentItem{
				{Path: "docs/**/*.md"},
				{Path: "docs/api.md", Channel: "acme/reference"},
			}}},
			want: map[string]claim{
				"docs/api.md":   {"Docs", 0, 0, "docs/**/*.md", ""},
				"docs/guide.md": {"Docs", 0, 0, "docs/**/*.md", ""},
			},
		},
		{
			name:  "non-overlapping items resolve as before",
			files: []string{"docs/guide.md", "store/ui.json"},
			collections: []Collection{
				{Name: "Docs", Content: []ContentItem{{Path: "docs/*.md"}}},
				{Name: "Store", Content: []ContentItem{{Path: "store/*.json"}}},
			},
			want: map[string]claim{
				"docs/guide.md": {"Docs", 0, 0, "docs/*.md", ""},
				"store/ui.json": {"Store", 1, 0, "store/*.json", ""},
			},
		},
		{
			name:  "overlap across collections goes to the earlier collection",
			files: []string{"docs/api.md", "docs/guide.md"},
			collections: []Collection{
				{Path: "docs/**/*.md"},
				{Name: "Reference", Content: []ContentItem{{Path: "docs/api.md", Channel: "acme/reference"}}},
			},
			want: map[string]claim{
				"docs/api.md":   {"", 0, 0, "docs/**/*.md", ""},
				"docs/guide.md": {"", 0, 0, "docs/**/*.md", ""},
			},
		},
		{
			name: "the kapimart shape: the catalogue resolves once, at the reference point",
			files: []string{
				"src/en/error-messages.properties",
				"src/en/store-ui.json",
				"src/en/product-catalog.yaml",
				"src/en/email-templates.html",
				"src/de/error-messages.properties",
			},
			collections: []Collection{{Name: "Online Store", Base: "src", Channel: "kapimart/store", Content: []ContentItem{
				{Path: "en/error-messages.properties", Target: "{lang}", Channel: "kapimart/reference"},
				{Path: "en/*.{json,yaml,properties,html}", Target: "{lang}"},
			}}},
			want: map[string]claim{
				"src/en/error-messages.properties": {"Online Store", 0, 0, "src/en/error-messages.properties", "kapimart/reference"},
				"src/en/store-ui.json":             {"Online Store", 0, 1, "src/en/*.{json,yaml,properties,html}", ""},
				"src/en/product-catalog.yaml":      {"Online Store", 0, 1, "src/en/*.{json,yaml,properties,html}", ""},
				"src/en/email-templates.html":      {"Online Store", 0, 1, "src/en/*.{json,yaml,properties,html}", ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				createFile(t, dir, f, "x")
			}
			reg := registry.NewFormatRegistry()
			registerBuiltIn(reg, "markdown", ".md")
			registerBuiltIn(reg, "json", ".json")
			registerBuiltIn(reg, "yaml", ".yaml")
			registerBuiltIn(reg, "properties", ".properties")
			registerBuiltIn(reg, "html", ".html")

			proj := &KapiProject{Version: CurrentVersion, Collections: tt.collections}
			ctx := NewProjectContext(proj, filepath.Join(dir, RecipeFileName))

			files, err := ctx.ResolveContent(reg)
			require.NoError(t, err)

			got := map[string]claim{}
			for _, rf := range files {
				rel := filepath.ToSlash(rf.Relative)
				_, dup := got[rel]
				require.False(t, dup, "%s resolved twice", rel)
				require.NotNil(t, rf.Item)
				got[rel] = claim{rf.Collection, rf.CollectionIndex, rf.ItemIndex, rf.Pattern, rf.Item.Channel}

				// The path-to-item walks answer the same question from the other
				// direction and must name the same claimant.
				assert.Equal(t, rf.Collection, proj.CollectionForPath(rel), "CollectionForPath disagrees for %s", rel)
				channels, _, cerr := proj.declaredChannelsFor(GovernancePoint{Path: rel})
				require.NoError(t, cerr)
				require.NotEmpty(t, channels)
				assert.Equal(t, rf.Item.Channel, channels[0], "declaredChannelsFor disagrees for %s", rel)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveContent_ClaimOrderIsRecipeOrder checks the returned list keeps the
// recipe's own order: the claiming item's position decides where a file
// appears, so a listing reads top to bottom the way the recipe does.
func TestResolveContent_ClaimOrderIsRecipeOrder(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "docs/api.md", "x")
	createFile(t, dir, "docs/changelog.md", "x")
	createFile(t, dir, "docs/guide.md", "x")
	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "markdown", ".md")

	proj := &KapiProject{Version: CurrentVersion, Collections: []Collection{{Name: "Docs", Content: []ContentItem{
		{Path: "docs/changelog.md"},
		{Path: "docs/api.md"},
		{Path: "docs/**/*.md"},
	}}}}
	ctx := NewProjectContext(proj, filepath.Join(dir, RecipeFileName))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	var order []string
	for _, rf := range files {
		order = append(order, filepath.ToSlash(rf.Relative)+"@"+rf.Pattern)
	}
	assert.Equal(t, []string{
		"docs/changelog.md@docs/changelog.md",
		"docs/api.md@docs/api.md",
		"docs/guide.md@docs/**/*.md",
	}, order)
}
