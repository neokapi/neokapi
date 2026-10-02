package pluginhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
)

// A plugin format's manifest declarations of what its writer writes reach the
// registry at discovery, as generative does, so a caller reads them without
// launching the plugin.
func TestRegisterModeCFormats_CarriesEditDeclarations(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})
	f := &plugin.Manifest.Capabilities.Formats[0]
	f.WritableAttrs = map[string][]string{"link:hyperlink": {"href"}}
	f.Synthesizes = []string{"fmt:bold"}

	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	reg := registry.NewFormatRegistry()
	RegisterModeCFormats(host, pool, reg)

	info := reg.FormatInfo("fakefmt")
	require.NotNil(t, info)
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href"}}, info.WritableAttrs)
	assert.Equal(t, []string{"fmt:bold"}, info.Synthesizes)
	assert.Empty(t, pool.Active(), "reading the declaration starts no daemon")
}
