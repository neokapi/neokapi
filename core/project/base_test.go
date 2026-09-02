package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestJoinBase(t *testing.T) {
	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{"no base leaves the path alone", "", "docs/*.md", "docs/*.md"},
		{"no path stays empty", "web", "", ""},
		{"a base prefixes the path", "web", "docs/*.md", "web/docs/*.md"},
		{"a trailing slash is not doubled", "web/", "docs/*.md", "web/docs/*.md"},
		{"an absolute path escapes the base", "web", "/etc/passwd", "/etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, JoinBase(tt.base, tt.path))
		})
	}
}

func TestCollection_EffectiveItems_FoldsBase(t *testing.T) {
	const src = `version: v1
collections:
  - name: docs
    base: bowrain/web/docs
    content:
      - path: docs/**/*.mdx
        target: i18n/{lang}/current/{path}.mdx
      - path: blog/*.md
        base: blog
        target: i18n/{lang}/blog/{filename}
`
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	require.NoError(t, p.Validate())

	items := p.Collections[0].EffectiveItems()
	require.Len(t, items, 2)

	assert.Equal(t, "bowrain/web/docs/docs/**/*.mdx", items[0].Path)
	assert.Equal(t, "bowrain/web/docs/i18n/{lang}/current/{path}.mdx", items[0].Target)
	assert.Empty(t, items[0].Base,
		"an item declaring no base of its own relativizes against the joined pattern's fixed prefix")

	assert.Equal(t, "bowrain/web/docs/blog/*.md", items[1].Path)
	assert.Equal(t, "bowrain/web/docs/i18n/{lang}/blog/{filename}", items[1].Target)
	assert.Equal(t, "bowrain/web/docs/blog", items[1].Base,
		"an item's own base is written relative to the collection's")
}

// TestCollection_BaseTargetsMatchUnbased pins the equivalence a `base:` is for:
// writing the prefix once produces exactly the paths that spelling it out on
// every line does.
func TestCollection_BaseTargetsMatchUnbased(t *testing.T) {
	const based = `version: v1
collections:
  - name: docs
    base: web
    content:
      - path: docs/**/*.md
        target: i18n/{lang}/current/{path}.md
`
	const spelledOut = `version: v1
collections:
  - name: docs
    content:
      - path: web/docs/**/*.md
        target: web/i18n/{lang}/current/{path}.md
`
	var a, b KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(based), &a))
	require.NoError(t, yaml.Unmarshal([]byte(spelledOut), &b))

	const source = "web/docs/guide/intro.md"
	ai := a.Collections[0].EffectiveItems()[0]
	bi := b.Collections[0].EffectiveItems()[0]

	assert.Equal(t, bi.Path, ai.Path)
	assert.Equal(t,
		ResolveTargetPath(bi.Path, bi.Base, bi.Target, source, "nb"),
		ResolveTargetPath(ai.Path, ai.Base, ai.Target, source, "nb"))
	assert.Equal(t, "web/i18n/nb/current/guide/intro.md",
		ResolveTargetPath(ai.Path, ai.Base, ai.Target, source, "nb"))
}

func TestCollection_EffectiveItems_BareEntry(t *testing.T) {
	const src = `version: v1
collections:
  - path: "src/**/*.json"
    target: "out/{lang}/{relpath}"
`
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	require.NoError(t, p.Validate())

	require.True(t, p.Collections[0].IsBareEntry())
	items := p.Collections[0].EffectiveItems()
	require.Len(t, items, 1)
	assert.Equal(t, "src/**/*.json", items[0].Path)
	assert.Equal(t, "out/{lang}/{relpath}", items[0].Target)
}

func TestKapiProject_IterateContent_FoldsBase(t *testing.T) {
	const src = `version: v1
collections:
  - name: docs
    base: web
    content:
      - path: docs/*.md
  - path: README.md
`
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	require.NoError(t, p.Validate())

	items := p.IterateContent()
	require.Len(t, items, 2)
	assert.Equal(t, "web/docs/*.md", items[0].Item.Path)
	assert.Equal(t, "docs", items[0].Collection.Name)
	assert.Equal(t, "README.md", items[1].Item.Path)
	assert.Empty(t, items[1].Collection.Name)
}

func TestKapiProject_RetiredKeys(t *testing.T) {
	tests := []struct {
		name    string
		recipe  string
		wantSub string
	}{
		{
			"an unknown version is rejected",
			"version: v7\ncollections: []\n",
			`unsupported version "v7"`,
		},
		{
			"a stray content: key is rejected by name",
			"version: v1\ncontent:\n  - path: a.md\n",
			"content: is no longer a recipe key. Use collections",
		},
		{
			"a stray coordinates: block names what replaced it",
			"version: v1\ncoordinates:\n  product: [a]\n",
			"coordinates: is no longer a recipe key. Use profiles",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p KapiProject
			require.NoError(t, yaml.Unmarshal([]byte(tt.recipe), &p))
			err := p.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}
