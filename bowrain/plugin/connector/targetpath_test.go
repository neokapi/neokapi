package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	coreproj "github.com/neokapi/neokapi/core/project"
)

// These cases replay the first dogfood delivery, which wrote Norwegian over
// four English masters: the old hand-rolled expander did not know {dir} and
// reconstructed the source path. The mapper now shares the local engine's
// matcher and template vocabulary, and a source-path collision is impossible
// by construction.
func TestResolveTargetPath_SharesTheLocalEngineVocabulary(t *testing.T) {
	const recipe = `
version: v1
name: t
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: demos
    base: harness/demos
    content:
      - path: "*/demo.yaml"
        target: "{dir}/demo.{lang}.yaml"
  - name: cli
    base: host/i18n
    content:
      - path: commands.json
        target: catalogs/{lang}.mo
  - name: no-target
    content:
      - path: "plain/**/*.md"
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(recipe), &p))
	require.NoError(t, p.Validate())
	c := &BowrainSourceConnector{project: &bproject.Project{Recipe: &bproject.Recipe{KapiProject: p}}}

	tests := []struct{ item, want string }{
		// The corruption shape: {dir} expands; the target is the sidecar,
		// never the master.
		{"harness/demos/07-x/demo.yaml", "harness/demos/07-x/demo.nb.yaml"},
		// A name-changing target maps fully; the source filename does not leak.
		{"host/i18n/commands.json", "host/i18n/catalogs/nb.mo"},
		// No matching item and no locale segment: locale-suffixed sibling.
		{"unmatched/readme.txt", "unmatched/readme.nb.txt"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, c.resolveTargetPath(tt.item, "nb"), tt.item)
	}

	// The guard: a matched item whose template collapses onto the source
	// still never writes in place.
	got := c.resolveTargetPath("plain/notes/a.md", "nb")
	assert.NotEqual(t, "plain/notes/a.md", got)
}
