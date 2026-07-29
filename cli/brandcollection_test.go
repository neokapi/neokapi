package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A repository holding two products binds a different voice per collection, so
// the ad-hoc brand commands must answer for the file in front of them. They
// resolved defaults.brand_voice whatever the path, which handed back the wrong
// register for half the tree: coordinates governed the flow path and nothing
// else.
func TestResolveBrandProfileCmd_UsesTheFilesCollection(t *testing.T) {
	root := writeTwoProductProject(t)
	t.Chdir(root)

	a := &App{}

	got, _, err := a.ResolveBrandProfileCmd(brandProbeCmd(a), filepath.Join("mail", "en.json"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "platform voice", got.Name,
		"a file in a bowrain-coordinate collection resolves the bowrain profile")

	got, _, err = a.ResolveBrandProfileCmd(brandProbeCmd(a), filepath.Join("engine", "meta.json"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "engine voice", got.Name,
		"a file in a kapi-coordinate collection resolves the kapi profile")
}

// A path no collection claims falls back to the project default — the answer
// this command always gave, and must keep giving.
func TestResolveBrandProfileCmd_UnclaimedPathKeepsTheDefault(t *testing.T) {
	root := writeTwoProductProject(t)
	t.Chdir(root)

	a := &App{}
	got, _, err := a.ResolveBrandProfileCmd(brandProbeCmd(a), "README.md")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "default voice", got.Name)
}

// Passing no path at all is the bare `kapi brand guide` case: the project
// default, unchanged.
func TestResolveBrandProfileCmd_NoPathKeepsTheDefault(t *testing.T) {
	root := writeTwoProductProject(t)
	t.Chdir(root)

	a := &App{}
	got, _, err := a.ResolveBrandProfileCmd(brandProbeCmd(a))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "default voice", got.Name)
}

// brandProbeCmd is a brand command with none of the selector flags set — the
// flag-free path these tests are about, where resolution falls through to the
// project.
func brandProbeCmd(a *App) *cobra.Command { return NewBrandCmd(a) }

// writeTwoProductProject builds the shape this exists to serve: one project,
// two products, a distinct voice per product, and a default for content that
// belongs to neither.
func writeTwoProductProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	profile := func(name string) string {
		return "name: " + name + "\n" +
			"tone:\n  personality: [plain]\n  formality: neutral\n  emotion: neutral\n  humor: none\n" +
			"style:\n  active_voice: true\n"
	}
	write("voices/default.yaml", profile("default voice"))
	write("voices/engine.yaml", profile("engine voice"))
	write("voices/platform.yaml", profile("platform voice"))

	write("engine/meta.json", `{"a":"one"}`)
	write("mail/en.json", `{"b":"two"}`)
	write("README.md", "# claimed by no collection\n")

	write("kapi.yaml", `version: v1
defaults:
  source_language: en
  target_languages: [nb]
  brand_voice:
    profile_file: voices/default.yaml
coordinates:
  product: [kapi, bowrain]
  channel: [engine, email]
profiles:
  - when: { product: kapi }
    voice: voices/engine.yaml
  - when: { product: bowrain }
    voice: voices/platform.yaml
content:
  - name: kapi-engine
    context: { product: kapi, channel: engine }
    items:
      - path: engine/**/*.json
        target: engine/{lang}/{filename}
  - name: bowrain-email
    context: { product: bowrain, channel: email }
    items:
      - path: mail/**/*.json
        target: mail/{lang}/{filename}
`)
	return root
}
