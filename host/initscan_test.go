package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTree creates each file under root with a small body.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
}

func proposeFor(t *testing.T, root string, opts ProposeOptions) []ProposedCollection {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	opts.Formats = a.FormatReg
	if opts.SourceLocale == "" {
		opts.SourceLocale = "en"
	}
	got, err := ProposeCollections(root, opts)
	require.NoError(t, err)
	return got
}

func paths(ps []ProposedCollection) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Path)
	}
	return out
}

// A docs repository: a README at the root keeps an entry of its own, the docs
// tree becomes one glob, and nothing under an ignored or dependency directory
// is read.
func TestProposeCollections_DocsRepository(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":                    "/site-out/\n*.draft.md\n",
		"README.md":                     "# Fernwell\n",
		"docs/install.md":               "# Install\n",
		"docs/guide/usage.md":           "# Usage\n",
		"docs/guide/next.draft.md":      "# Draft\n",
		"site-out/index.html":           "<p>built</p>\n",
		"node_modules/pkg/README.md":    "# dep\n",
		".github/ISSUE_TEMPLATE/bug.md": "# bug\n",
		"notes.txt":                     "plain text\n",
		"package.json":                  `{"name":"fernwell"}`,
	})

	got := proposeFor(t, root, ProposeOptions{})
	assert.Equal(t, []string{"README.md", "docs/**/*.md"}, paths(got))
	assert.Equal(t, []string{"docs/guide/usage.md", "docs/install.md"}, got[1].Files)
	assert.Equal(t, "markdown", got[1].Format)
	assert.Equal(t, "docs/: 2 Markdown files", got[1].Reason)
	assert.Empty(t, got[1].Target, "a document's destination is the project's decision")

	again := proposeFor(t, root, ProposeOptions{})
	assert.Equal(t, got, again, "the same tree gives the same proposal")
}

// A React app with i18n catalogs: the preset layout it matches is proposed
// with its target, the source-language directory is found by name, and the
// translations and configuration files are left out.
func TestProposeCollections_ReactAppWithCatalogs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"package.json":                   `{"dependencies":{"react-i18next":"^14"}}`,
		"tsconfig.json":                  `{}`,
		"public/index.html":              "<div id=\"root\"></div>\n",
		"public/locales/en/common.json":  `{"hello":"Hello"}`,
		"public/locales/fr/common.json":  `{"hello":"Bonjour"}`,
		"src/i18n/messages/en-US/a.json": `{"a":"A"}`,
		"src/i18n/messages/de-DE/a.json": `{"a":"A"}`,
		"lib/l10n/app_en.arb":            `{"@@locale":"en","title":"Title"}`,
		"lib/l10n/app_nb.arb":            `{"@@locale":"nb","title":"Tittel"}`,
	})

	got := proposeFor(t, root, ProposeOptions{Targets: true})
	byPath := map[string]ProposedCollection{}
	for _, p := range got {
		byPath[p.Path] = p
	}

	preset, ok := byPath["public/locales/en/*.json"]
	require.True(t, ok, "the react-i18next layout is proposed: %v", paths(got))
	assert.Equal(t, "public/locales/{lang}/*.json", preset.Target)
	assert.Contains(t, preset.Reason, "react-i18next")

	dirCat, ok := byPath["src/i18n/messages/en-US/*.json"]
	require.True(t, ok, "a directory named for the source language marks its catalogs: %v", paths(got))
	assert.Equal(t, "src/i18n/messages/{lang}/{path}.json", dirCat.Target)

	arb, ok := byPath["lib/l10n/app_en.arb"]
	require.True(t, ok, "a file named for the source language is its own entry: %v", paths(got))
	assert.Equal(t, "lib/l10n/app_{lang}.arb", arb.Target)

	for _, p := range got {
		for _, f := range p.Files {
			assert.NotContains(t, []string{"public/locales/fr/common.json", "src/i18n/messages/de-DE/a.json",
				"lib/l10n/app_nb.arb", "package.json", "tsconfig.json"}, f, "%s is no source content", f)
		}
	}
}

// A project with no target language gets no target on a catalog: nothing is
// translated, so there is nowhere to write.
func TestProposeCollections_NoTargetsNoTemplate(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"locales/en/app.json": `{"a":"A"}`})
	got := proposeFor(t, root, ProposeOptions{})
	require.Len(t, got, 1)
	assert.Equal(t, "locales/en/*.json", got[0].Path)
	assert.Empty(t, got[0].Target)
}

// An empty repository proposes nothing, and --framework proposes its layout
// before any file exists.
func TestProposeCollections_EmptyAndFramework(t *testing.T) {
	root := t.TempDir()
	assert.Empty(t, proposeFor(t, root, ProposeOptions{}))

	got := proposeFor(t, root, ProposeOptions{Framework: "flutter"})
	require.Len(t, got, 1)
	assert.Equal(t, "lib/l10n/app_en.arb", got[0].Path)
	assert.Equal(t, "lib/l10n/app_{lang}.arb", got[0].Target)
	assert.Empty(t, got[0].Files)

	_, err := ProposeCollections(root, ProposeOptions{Formats: newFormats().FormatReg, Framework: "nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown framework")
}

func newFormats() *App {
	a := &App{}
	a.InitRegistries()
	return a
}
