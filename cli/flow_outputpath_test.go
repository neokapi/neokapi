package cli

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
		Content: []project.ContentCollection{
			{Name: "Docs", Base: "input/docs", Items: []project.ContentItem{
				{Path: "input/docs/**/*.md", Target: "output/{lang}/docs"}, // directory-mirror
			}},
			{Name: "Store", Items: []project.ContentItem{
				{Path: "input/store/*.json", Target: "output/{lang}/store/*.json"}, // legacy *.ext
			}},
		},
	}
	a := &App{projectContext: project.NewProjectContext(proj, filepath.Join(root, "p.kapi"))}

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
