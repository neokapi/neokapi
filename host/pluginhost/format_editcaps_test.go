//go:build !js

package pluginhost

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href"}}, info.EditCapabilities.WritableAttrs)
	assert.Equal(t, []string{"fmt:bold"}, info.EditCapabilities.Synthesizes)
	assert.Empty(t, pool.Active(), "reading the declaration starts no daemon")
}

// linkWriter is a built-in writer that declares it writes a link's href.
type linkWriter struct{ format.BaseFormatWriter }

func (*linkWriter) Write(context.Context, <-chan *model.Part) error { return nil }

func (*linkWriter) WritableAttrs() map[string][]string {
	return map[string][]string{"link:hyperlink": {"href"}}
}

func (*linkWriter) WriteAttr([]model.Run, int, string, string) ([]model.Run, error) {
	return nil, errors.New("unused")
}

// AD-007 precedence: a plugin's writer replaces a built-in's of the same name,
// and the built-in's declarations leave with it. A plugin whose manifest
// declares nothing leaves the format declaring nothing, so the change service
// refuses set_attribute rather than report applied for bytes the plugin's
// writer never spells.
func TestRegisterModeCFormats_PluginWriterReplacesBuiltinDeclarations(t *testing.T) {
	bin := buildFakeDaemon(t)
	plugin := makePluginWithBridge(t, "fmt-plugin", bin, "fakefmt", []string{".fakefmt"})
	host := NewHost([]*Plugin{plugin}, nil)
	pool := daemonPoolWithBridgeEnv(t)
	t.Cleanup(pool.Shutdown)

	reg := registry.NewFormatRegistry()
	reg.RegisterWriter("fakefmt", func() format.DataFormatWriter {
		return &linkWriter{format.BaseFormatWriter{FormatName: "fakefmt"}}
	})
	reg.SetFormatSource("fakefmt", registry.SourceBuiltIn)
	require.Equal(t, []string{"href"}, reg.FormatInfo("fakefmt").EditCapabilities.Writable("link:hyperlink"))

	RegisterModeCFormats(host, pool, reg)

	info := reg.FormatInfo("fakefmt")
	require.NotNil(t, info)
	assert.Equal(t, "fmt-plugin", info.Source)
	assert.True(t, info.EditCapabilities.IsZero(), "the built-in's declaration left with its writer: %+v", info.EditCapabilities)
}
