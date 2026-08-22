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
// resolved defaults.voice whatever the path, which handed back the wrong
// register for half the tree: the context space governed the flow path and
// nothing else.
func TestResolveVoiceProfileCmd_UsesTheFilesCollection(t *testing.T) {
	root := writeTwoProductProject(t)
	t.Chdir(root)

	a := &App{}

	got, _, err := a.ResolveVoiceProfileCmd(voiceProbeCmd(a), filepath.Join("mail", "en.json"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "platform voice", got.Name,
		"a file in a bowrain-channel collection resolves the bowrain profile")

	got, _, err = a.ResolveVoiceProfileCmd(voiceProbeCmd(a), filepath.Join("engine", "meta.json"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "engine voice", got.Name,
		"a file in a kapi-channel collection resolves the kapi profile")
}

// A path no collection claims falls back to the project default — the answer
// this command always gave, and must keep giving.
func TestResolveVoiceProfileCmd_UnclaimedPathKeepsTheDefault(t *testing.T) {
	root := writeTwoProductProject(t)
	t.Chdir(root)

	a := &App{}
	got, _, err := a.ResolveVoiceProfileCmd(voiceProbeCmd(a), "README.md")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "default voice", got.Name)
}

// Passing no path at all is the bare `kapi voice guide` case: the project
// default, unchanged.
func TestResolveVoiceProfileCmd_NoPathKeepsTheDefault(t *testing.T) {
	root := writeTwoProductProject(t)
	t.Chdir(root)

	a := &App{}
	got, _, err := a.ResolveVoiceProfileCmd(voiceProbeCmd(a))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "default voice", got.Name)
}

// voiceProbeCmd is a brand command with none of the selector flags set — the
// flag-free path these tests are about, where resolution falls through to the
// project.
func voiceProbeCmd(a *App) *cobra.Command { return NewVoiceCmd(a) }

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
  voice:
    profile_file: voices/default.yaml
profiles:
  kapi:
    channels: [engine]
    voice: voices/engine.yaml
  bowrain:
    channels: [email]
    voice: voices/platform.yaml
collections:
  - name: kapi-engine
    channel: kapi/engine
    content:
      - path: engine/**/*.json
        target: engine/{lang}/{filename}
  - name: bowrain-email
    channel: bowrain/email
    content:
      - path: mail/**/*.json
        target: mail/{lang}/{filename}
`)
	return root
}
