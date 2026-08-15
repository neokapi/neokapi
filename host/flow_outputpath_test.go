package host

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ad-hoc `-o` template expansion shares the core path-token vocabulary, supports
// the directory-mirror form, and the legacy `*` (stem).
func TestExpandAdhocOutputTemplate(t *testing.T) {
	tests := []struct {
		name  string
		tmpl  string
		input string
		base  string
		lang  string
		want  string
	}{
		{"tokens", "out/{lang}/{name}.{ext}", "input/api.md", "", "fr", "out/fr/api.md"},
		{"dir-mirror with base", "out/{lang}", "docs/api.md", "docs", "fr", "out/fr/api.md"},
		{"dir-mirror trailing slash", "out/{lang}/", "x/api.md", "", "fr", "out/fr/api.md"},
		{"legacy star keeps extension", "out/{lang}/*.json", "src/messages.json", "", "fr", "out/fr/messages.json"},
		{"no double extension", "out/{lang}/{name}.{ext}", "docs/a.md", "", "de", "out/de/a.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandAdhocOutputTemplate(tt.tmpl, tt.input, tt.base, tt.lang)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}

// In project mode, a tool/flow run resolves the output via the matched content
// item's target through the one core resolver — including the directory-mirror
// form and the old double-extension-prone `*.ext` form (now correct).
func TestProjectItemTargetPath(t *testing.T) {
	root := t.TempDir()
	proj := &project.KapiProject{
		Version:  "v1",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []project.Collection{
			{Name: "Docs", Content: []project.ContentItem{
				{Path: "input/docs/**/*.md", Target: "output/{lang}/docs"}, // directory-mirror
			}},
			{Name: "Store", Content: []project.ContentItem{
				{Path: "input/store/*.json", Target: "output/{lang}/store/*.json"}, // legacy *.ext
			}},
		},
	}
	a := &App{ProjectContext: project.NewProjectContext(proj, filepath.Join(root, "kapi.yaml"))}

	got, ok := a.projectItemTargetPath(filepath.Join(root, "input/docs/api-reference.md"), "fr")
	require.True(t, ok)
	assert.Equal(t, filepath.Join(root, "output/fr/docs/api-reference.md"), got)

	// The form that used to yield `.json.json` now resolves cleanly.
	got, ok = a.projectItemTargetPath(filepath.Join(root, "input/store/checkout.json"), "fr")
	require.True(t, ok)
	assert.Equal(t, filepath.Join(root, "output/fr/store/checkout.json"), got)

	// A file outside any content pattern: no project target → caller falls back.
	_, ok = a.projectItemTargetPath(filepath.Join(root, "elsewhere/readme.md"), "fr")
	assert.False(t, ok)
}

// A destination inside the collection that supplied the input is self-feeding:
// the glob reads back whatever the run writes, and the project doubles on every
// pass. There is no safe sibling to fall back to, so the run is refused and the
// message names the two ways out.
func TestResolveOutputPath_RefusesADestinationTheCollectionTracks(t *testing.T) {
	root := t.TempDir()
	proj := &project.KapiProject{
		Version:  "v1",
		Defaults: project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{
			{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
			{Name: "site", Content: []project.ContentItem{
				{Path: "site/**/*.md", Target: "build/{lang}/{name}.md"},
			}},
		},
	}
	a := &App{ProjectContext: project.NewProjectContext(proj, filepath.Join(root, "kapi.yaml"))}
	a.TargetLang = "en"

	_, err := a.resolveOutputPath(filepath.Join(root, "docs/index.md"), "")
	require.Error(t, err, "docs/index_en.md is matched by docs/**/*.md")
	assert.Contains(t, err.Error(), "docs/**/*.md")
	assert.Contains(t, err.Error(), "target:")

	// A collection WITH a target resolves normally — the fallback is never reached.
	out, err := a.resolveOutputPath(filepath.Join(root, "site/index.md"), "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "build/en/index.md"), out)

	// An explicit -o is the user's own choice of destination, and wins.
	out, err = a.resolveOutputPath(filepath.Join(root, "docs/index.md"), filepath.Join(root, "out/{lang}"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "out/en/index.md"), out)
}

// Outside a project there are no collections to feed, so the sibling default
// stands — that is the ad-hoc `kapi run <flow> -i file` shape.
func TestResolveOutputPath_AdHocKeepsTheSibling(t *testing.T) {
	a := &App{}
	a.TargetLang = "fr"

	out, err := a.resolveOutputPath(filepath.Join("docs", "index.md"), "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("docs", "index_fr.md"), out)
}

// With no target language the sibling would be `index_.md` — a name for
// nothing. Refuse and say what is missing instead of writing it.
func TestResolveOutputPath_RefusesWithNoTargetLanguage(t *testing.T) {
	a := &App{}

	_, err := a.resolveOutputPath(filepath.Join("docs", "index.md"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no output destination")
}
