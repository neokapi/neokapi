package pluginhost_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/plugin/manifest"
)

func TestLookupTombstone(t *testing.T) {
	tomb, ok := pluginhost.LookupTombstone("llm")
	require.True(t, ok, "llm should be retired")
	assert.Equal(t, "llm", tomb.Plugin)
	assert.NotEmpty(t, tomb.RetiredIn)
	assert.Contains(t, tomb.Notice(), "retired")
	assert.Contains(t, tomb.Notice(), "kapi plugins prune")

	_, ok = pluginhost.LookupTombstone("sat")
	assert.False(t, ok, "active plugins are not retired")
}

// A retired plugin stays listed (so it can be surfaced and pruned) but is inert:
// it contributes nothing to any dispatch table.
func TestRetiredPluginIsInertButListed(t *testing.T) {
	retired := &pluginhost.Plugin{
		Source: pluginhost.Source{Order: 1, Label: "user"},
		Manifest: &manifest.Manifest{
			Plugin: "llm", Version: "0.1.0", Binary: "kapi-llm",
			Capabilities: manifest.Capabilities{
				Commands: []manifest.Command{{Name: "generate"}},
				Formats:  []manifest.Format{{Name: "llm-format"}},
			},
		},
	}
	active := &pluginhost.Plugin{
		Source: pluginhost.Source{Order: 1, Label: "user"},
		Manifest: &manifest.Manifest{
			Plugin: "sat", Version: "1.0.0", Binary: "kapi-sat",
			Capabilities: manifest.Capabilities{
				Commands: []manifest.Command{{Name: "segment"}},
			},
		},
	}

	h := pluginhost.NewHost([]*pluginhost.Plugin{retired, active}, nil)

	// Still listed, flagged retired.
	llm := h.Plugin("llm")
	require.NotNil(t, llm)
	require.NotNil(t, llm.Retired, "discovered tombstoned plugin must be marked retired")
	assert.Equal(t, "llm", llm.Retired.Plugin)
	assert.Nil(t, h.Plugin("sat").Retired)

	// Inert: no dispatch routes for the retired plugin.
	assert.Nil(t, h.CommandRoute("generate"), "retired plugin command must not dispatch")
	assert.Nil(t, h.FormatRoute("llm-format"), "retired plugin format must not dispatch")

	// Active plugin is unaffected.
	assert.NotNil(t, h.CommandRoute("segment"))
}
