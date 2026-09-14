package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	coreproj "github.com/neokapi/neokapi/core/project"
	bproject "github.com/neokapi/neokapi/host/venue/project"
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
        target: catalogs/{lang}.json
  - name: no-target
    content:
      - path: "plain/**/*.md"
  - name: package
    base: packaging
    source_only: true
    content:
      - path: nfpm.yaml
  - name: collapse
    content:
      - path: "same/*.md"
        target: "same/{filename}"
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
		{"host/i18n/commands.json", "host/i18n/catalogs/nb.json"},
		// No matching item and no locale segment: locale-suffixed sibling.
		{"unmatched/readme.txt", "unmatched/readme.nb.txt"},
		// No matching item: a segment or a file stem spelled as the source
		// locale is swapped whole, and a word that contains it is left alone.
		{"unmatched/en/app.json", "unmatched/nb/app.json"},
		{"unmatched/strings/en.json", "unmatched/strings/nb.json"},
		{"scripts/gen-refs/checks/length-check.yaml", "scripts/gen-refs/checks/length-check.nb.yaml"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, c.resolveTargetPath(tt.item, "nb"), tt.item)
	}

	// A matched item with no target template is source-only, whether the
	// collection says so or only omits the target, so a pull writes nothing.
	for _, item := range []string{"plain/notes/a.md", "packaging/nfpm.yaml"} {
		assert.Empty(t, c.resolveTargetPath(item, "nb"), item)
	}

	// The guard: a matched item whose template collapses onto the source
	// still never writes in place.
	got := c.resolveTargetPath("same/a.md", "nb")
	assert.NotEqual(t, "same/a.md", got)
}
