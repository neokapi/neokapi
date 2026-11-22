package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverUpStubFlags is the declaration the server-up stubs carry: the flags
// the bowrain plumbing's server run reads.
var serverUpStubFlags = []manifest.FlagSpec{
	{Name: "project", Type: "string"}, {Name: "json", Type: "bool"}, {Name: "local", Type: "bool"},
	{Name: "timeout", Type: "string"}, {Name: "fail-on-incomplete-watch", Type: "bool"},
}

func venueRoute(flags []manifest.FlagSpec) *pluginhost.CommandRoute {
	return &pluginhost.CommandRoute{
		Plugin:  &pluginhost.Plugin{Dir: "/plugins/bowrain", Manifest: &manifest.Manifest{Plugin: "bowrain", Version: "1.0.0"}},
		Command: manifest.Command{Name: "server-up", Hidden: true, Flags: flags},
	}
}

// The embedded dispatch (the MCP up tool, the desktop) follows the same
// declaration as kapi up: a server run sends only declared flags, refuses an
// option it does not read, and refuses a plugin that declares nothing.
func TestVenueArgs_FollowTheManifestDeclaration(t *testing.T) {
	declared := venueRoute([]manifest.FlagSpec{{Name: "project", Type: "string"}, {Name: "json", Type: "bool"}})

	args, err := venueArgs(declared, "/p/kapi.yaml", UpOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"--project=/p/kapi.yaml", "--json"}, args)

	_, err = venueArgs(declared, "/p/kapi.yaml", UpOptions{Jobs: 2})
	require.ErrorContains(t, err, "--jobs applies to a run on this machine")

	args, err = venueArgs(declared, "/p/kapi.yaml", UpOptions{Jobs: 2, Local: true})
	require.NoError(t, err, "a local run reads every option")
	assert.Contains(t, args, "--jobs=2")

	_, err = venueArgs(venueRoute(nil), "/p/kapi.yaml", UpOptions{})
	require.ErrorContains(t, err, "older than this kapi")
	assert.ErrorContains(t, err, "kapi plugin update bowrain")
}

// A manifest keeps "no declaration" apart from "declares no flags" through a
// parse and a write.
func TestCommandDeclaresFlags_SurvivesARoundTrip(t *testing.T) {
	for _, tc := range []struct {
		json     string
		declares bool
	}{
		{`{"name":"server-up"}`, false},
		{`{"name":"server-up","flags":[]}`, true},
		{`{"name":"server-up","flags":[{"name":"json","type":"bool"}]}`, true},
	} {
		m := &manifest.Manifest{ManifestVersion: manifest.CurrentVersion, Plugin: "p", Version: "1.0.0", Binary: "p"}
		var c manifest.Command
		require.NoError(t, json.Unmarshal([]byte(tc.json), &c))
		m.Capabilities.Commands = []manifest.Command{c}
		data, err := m.Marshal()
		require.NoError(t, err)
		back, err := manifest.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, tc.declares, back.Capabilities.Commands[0].DeclaresFlags(), tc.json)
	}
}

// A Homebrew-installed plugin is named by the formula whose Cellar holds it,
// the beta formula included.
func TestBrewFormulaOf(t *testing.T) {
	root := t.TempDir()
	cellar := filepath.Join(root, "Cellar", "bowrain-cli-beta", "1.4.0-rc5", "share", "kapi", "plugins", "bowrain")
	require.NoError(t, os.MkdirAll(cellar, 0o755))
	link := filepath.Join(root, "share", "kapi", "plugins", "bowrain")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(cellar, link))
	assert.Equal(t, "bowrain-cli-beta", brewFormulaOf(link))
	assert.Empty(t, brewFormulaOf(t.TempDir()))
}
