package sample

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OkapiMart uses a single `input/*` wildcard and relies on auto-detection plus
// two `defaults.formats[...].priority` pins. This registers stub okf_* formats
// mirroring the real okapi-bridge extension claims (including the .srt and .txt
// collisions) and asserts every shared input file resolves to the intended
// engine — locking the wildcard design without needing the real plugin.
func TestOkapiMartWildcardResolution(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("okapimart", dir))

	reg := registry.NewFormatRegistry()
	// extension → formats claiming it (matches okapi-bridge manifest.json).
	claims := map[string][]string{
		".json":       {"okf_json"},
		".yaml":       {"okf_yaml"},
		".html":       {"okf_html", "okf_html5"},
		".properties": {"okf_properties"},
		".xml":        {"okf_xml"},
		".md":         {"okf_markdown"},
		".srt":        {"okf_regex", "okf_vtt"},
		".txt":        {"okf_baseplaintext", "okf_basetable", "okf_plaintext"},
	}
	seen := map[string]bool{}
	for ext, fmts := range claims {
		for _, f := range fmts {
			if !seen[f] {
				reg.RegisterFormatInfo(registry.FormatID(f), registry.FormatInfo{
					Extensions: []string{ext},
					Source:     "okapi-bridge",
					HasReader:  true,
				})
				reg.SetFormatPriority(registry.FormatID(f), format.DefaultPluginPriority)
				seen[f] = true
			}
		}
	}

	proj, err := project.Load(filepath.Join(dir, "project.kapi"))
	require.NoError(t, err)
	ctx := project.NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)

	got := map[string]string{} // base filename → detected format
	for _, f := range files {
		got[filepath.Base(f.Path)] = f.Format
	}

	want := map[string]string{
		"store-ui.json":             "okf_json",
		"product-catalog.yaml":      "okf_yaml",
		"about-us.html":             "okf_html", // tiebreak over okf_html5
		"error-messages.properties": "okf_properties",
		"release-notes.xml":         "okf_xml",
		"changelog.md":              "okf_markdown",
		"onboarding-video.srt":      "okf_vtt",       // pinned over okf_regex
		"admin-guide.txt":           "okf_plaintext", // pinned over okf_basetable/…
	}
	for name, fmtName := range want {
		assert.Equal(t, fmtName, got[name], "wrong engine for %s", name)
	}
}
