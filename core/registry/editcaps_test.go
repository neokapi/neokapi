package registry

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// linkWriter declares that it writes a link's href and new bold codes.
type linkWriter struct{ stubWriter }

func (*linkWriter) WritableAttrs() map[string][]string {
	return map[string][]string{"link:hyperlink": {"href"}}
}

func (*linkWriter) WriteAttr([]model.Run, int, string, string) ([]model.Run, error) {
	return nil, errors.New("unused")
}
func (*linkWriter) Synthesizes() []string { return []string{"fmt:bold"} }
func (*linkWriter) SynthesizeCode(format.CodeSite) (model.Run, model.Run, error) {
	return model.Run{}, model.Run{}, errors.New("unused")
}

// A built-in writer's edit capabilities are probed once at registration; a
// plugin format's come from its manifest, without the plugin's writer being
// built; a writer that declares nothing has none.
func TestFormatInfoEditCapabilities(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterWriter("links", func() format.DataFormatWriter {
		return &linkWriter{stubWriter{format.BaseFormatWriter{FormatName: "links"}}}
	})
	reg.RegisterWriter("plain", func() format.DataFormatWriter { return newStubWriter("plain") })

	info := reg.FormatInfo("links")
	require.NotNil(t, info)
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href"}}, info.EditCapabilities.WritableAttrs)
	assert.Equal(t, []string{"fmt:bold"}, info.EditCapabilities.Synthesizes)
	assert.True(t, reg.FormatInfo("plain").EditCapabilities.IsZero())

	info.EditCapabilities.WritableAttrs["link:hyperlink"][0] = "changed"
	assert.Equal(t, "href", reg.FormatInfo("links").EditCapabilities.WritableAttrs["link:hyperlink"][0], "a caller's copy shares nothing with the registry")

	built := 0
	reg.RegisterFormatInfo("okf_html", FormatInfo{Source: "okapi-bridge", HasWriter: true})
	reg.SetFormatSource("okf_html", "okapi-bridge")
	reg.RegisterWriter("okf_html", func() format.DataFormatWriter { built++; return newStubWriter("okf_html") })
	reg.SetEditCapabilities("okf_html", format.EditCapabilities{WritableAttrs: map[string][]string{"link:hyperlink": {"href"}}})
	assert.Zero(t, built, "a plugin's writer is not built to read its declaration")
	assert.Equal(t, []string{"href"}, reg.FormatInfo("okf_html").EditCapabilities.Writable("link:hyperlink"))
}

// A plugin that replaces a built-in format's writer replaces its declaration
// too: the built-in's no longer describes the writer, so a plugin that
// declares nothing leaves the format declaring nothing.
func TestPluginWriterReplacesBuiltInEditCapabilities(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterWriter("html", func() format.DataFormatWriter {
		return &linkWriter{stubWriter{format.BaseFormatWriter{FormatName: "html"}}}
	})
	require.False(t, reg.FormatInfo("html").EditCapabilities.IsZero())

	reg.RegisterFormatInfo("html", FormatInfo{Source: "html-plugin", HasWriter: true})
	reg.SetFormatSource("html", "html-plugin")
	reg.RegisterWriter("html", func() format.DataFormatWriter { return newStubWriter("html") })
	assert.True(t, reg.FormatInfo("html").EditCapabilities.IsZero(), "the built-in's declaration left with its writer")

	reg.SetEditCapabilities("html", format.EditCapabilities{Synthesizes: []string{"fmt:italic"}})
	assert.Equal(t, format.EditCapabilities{Synthesizes: []string{"fmt:italic"}}, reg.FormatInfo("html").EditCapabilities)
	reg.SetEditCapabilities("html", format.EditCapabilities{})
	assert.True(t, reg.FormatInfo("html").EditCapabilities.IsZero(), "an empty declaration replaces one")
}

// The declaration is a field of its own: FormatInfo takes none of
// EditCapabilities' methods, so encoding it never omits a format that
// declares nothing, and the declaration encodes under its own key.
func TestFormatInfoEncodesEditCapabilitiesUnderItsOwnKey(t *testing.T) {
	_, promoted := any(FormatInfo{}).(interface{ IsZero() bool })
	assert.False(t, promoted, "FormatInfo has no IsZero of its declaration's")

	out, err := json.Marshal(struct {
		Info FormatInfo `json:"info,omitzero"`
	}{Info: FormatInfo{Name: "plain"}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"name":"plain"`, "a format that declares nothing is still encoded")
	assert.NotContains(t, string(out), "edit_capabilities")

	out, err = json.Marshal(FormatInfo{Name: "links", EditCapabilities: format.EditCapabilities{Synthesizes: []string{"fmt:bold"}}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"edit_capabilities":{"synthesizes":["fmt:bold"]}`)
}
