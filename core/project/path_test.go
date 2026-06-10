package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobFixedPrefix(t *testing.T) {
	assert.Equal(t, "web/docs/docs/", GlobFixedPrefix("web/docs/docs/**/*.mdx"))
	assert.Equal(t, "src/", GlobFixedPrefix("src/*.json"))
	assert.Equal(t, "core/i18n/builtins/", GlobFixedPrefix("core/i18n/builtins/metadata.json"))
	assert.Equal(t, "", GlobFixedPrefix("metadata.json"))
	assert.Equal(t, "a/", GlobFixedPrefix("a/{x,y}/z.md"))
}

func TestResolveTargetPath(t *testing.T) {
	// Tree mirroring: {path} is relative to the item pattern's fixed prefix.
	got := ResolveTargetPath(
		"web/docs/docs/**/*.mdx",
		"web/docs/i18n/{lang}/docusaurus-plugin-content-docs/current/{path}.mdx",
		"web/docs/docs/kapi/overview.mdx",
		"nb",
	)
	assert.Equal(t, "web/docs/i18n/nb/docusaurus-plugin-content-docs/current/kapi/overview.mdx", got)

	// Literal item path: {path} is the basename without extension.
	got = ResolveTargetPath(
		"core/i18n/builtins/metadata.json",
		"core/i18n/catalogs/{lang}.mo",
		"core/i18n/builtins/metadata.json",
		"nb",
	)
	assert.Equal(t, "core/i18n/catalogs/nb.mo", got)

	// Legacy bare `*` expands to the source basename without extension.
	got = ResolveTargetPath("src/*.json", "out/{lang}/*.json", "src/messages.json", "fr")
	assert.Equal(t, "out/fr/messages.json", got)

	// {filename} and {basename}.
	got = ResolveTargetPath("docs/**/*.md", "l10n/{lang}/{filename}", "docs/a/b.md", "de")
	assert.Equal(t, "l10n/de/b.md", got)
}
