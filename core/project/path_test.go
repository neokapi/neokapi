package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobFixedPrefix(t *testing.T) {
	assert.Equal(t, "web/docs/", GlobFixedPrefix("web/docs/**/*.mdx"))
	assert.Equal(t, "src/", GlobFixedPrefix("src/*.json"))
	assert.Equal(t, "core/i18n/builtins/", GlobFixedPrefix("core/i18n/builtins/metadata.json"))
	assert.Empty(t, GlobFixedPrefix("metadata.json"))
	assert.Equal(t, "a/", GlobFixedPrefix("a/{x,y}/z.md"))
}

func TestResolveTargetPath(t *testing.T) {
	// Tree mirroring via {path} (relative to the glob's fixed prefix).
	got := ResolveTargetPath(
		"web/docs/**/*.mdx", "",
		"web/i18n/{lang}/docusaurus-plugin-content-docs/current/{path}.mdx",
		"web/docs/kapi/overview.mdx", "nb",
	)
	assert.Equal(t, "web/i18n/nb/docusaurus-plugin-content-docs/current/kapi/overview.mdx", got)

	// Literal item path: rel is just the filename.
	got = ResolveTargetPath(
		"core/i18n/builtins/metadata.json", "",
		"core/i18n/catalogs/{lang}.mo", "core/i18n/builtins/metadata.json", "nb",
	)
	assert.Equal(t, "core/i18n/catalogs/nb.mo", got)

	// Legacy bare `*` expands to the source basename without extension.
	got = ResolveTargetPath("src/*.json", "", "out/{lang}/*.json", "src/messages.json", "fr")
	assert.Equal(t, "out/fr/messages.json", got)

	// {filename}.
	got = ResolveTargetPath("docs/**/*.md", "", "l10n/{lang}/{filename}", "docs/a/b.md", "de")
	assert.Equal(t, "l10n/de/b.md", got)
}

// The double-extension regression: input `input/docs/*.md` + target
// `output/{lang}/docs/*.md` must yield `…/api.md`, never `…/api.md.md`.
func TestResolveTargetPath_NoDoubleExtension(t *testing.T) {
	got := ResolveTargetPath("input/docs/*.md", "", "output/{lang}/docs/*.md", "input/docs/api-reference.md", "fr-FR")
	assert.Equal(t, "output/fr-FR/docs/api-reference.md", got)
}

// Directory-mirror target: no token/wildcard needed — the source tree (relative
// to base) is reproduced under the per-language root.
func TestResolveTargetPath_DirectoryMirror(t *testing.T) {
	// Default base = glob fixed prefix (input/docs/) → mirrors just the filename.
	got := ResolveTargetPath("input/docs/*.md", "", "output/{lang}/docs", "input/docs/api.md", "fr")
	assert.Equal(t, "output/fr/docs/api.md", got)

	// Trailing slash is also a directory target.
	got = ResolveTargetPath("input/docs/*.md", "", "output/{lang}/docs/", "input/docs/api.md", "fr")
	assert.Equal(t, "output/fr/docs/api.md", got)

	// Recursive glob → base input/ → the whole subtree mirrors under output/{lang}.
	got = ResolveTargetPath("input/**/*", "", "output/{lang}", "input/docs/guides/intro.md", "de")
	assert.Equal(t, "output/de/docs/guides/intro.md", got)
}

// An explicit base controls how much of the source path is mirrored.
func TestResolveTargetPath_ExplicitBase(t *testing.T) {
	// base=input → rel keeps docs/ ; directory target mirrors it.
	got := ResolveTargetPath("input/docs/*.md", "input", "output/{lang}", "input/docs/api.md", "fr")
	assert.Equal(t, "output/fr/docs/api.md", got)

	// {dir}/{name}.{ext} tokens with base.
	got = ResolveTargetPath("input/store/**/*", "input/store", "out/{lang}/{dir}/{name}.{ext}", "input/store/ui/labels.json", "ja")
	assert.Equal(t, "out/ja/ui/labels.json", got)
}

func TestExpandTemplate_Tokens(t *testing.T) {
	tmpl := "{lang}-{dir}-{name}-{ext}-{filename}-{path}-{relpath}"
	got := ResolvePathPattern(tmpl, "fr")
	got = ExpandTemplate(got, "ui/labels.json")
	assert.Equal(t, "fr-ui-labels-json-labels.json-ui/labels-ui/labels.json", got)
}
