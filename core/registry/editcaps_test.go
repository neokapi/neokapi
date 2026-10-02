package registry

import (
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
	assert.Equal(t, map[string][]string{"link:hyperlink": {"href"}}, info.WritableAttrs)
	assert.Equal(t, []string{"fmt:bold"}, info.Synthesizes)
	assert.True(t, reg.FormatInfo("plain").EditCapabilities.IsZero())

	info.WritableAttrs["link:hyperlink"][0] = "changed"
	assert.Equal(t, "href", reg.FormatInfo("links").WritableAttrs["link:hyperlink"][0], "a caller's copy shares nothing with the registry")

	built := 0
	reg.RegisterFormatInfo("okf_html", FormatInfo{
		Source: "okapi-bridge", HasWriter: true,
		WritableAttrs: map[string][]string{"link:hyperlink": {"href"}},
	})
	reg.SetFormatSource("okf_html", "okapi-bridge")
	reg.RegisterWriter("okf_html", func() format.DataFormatWriter { built++; return newStubWriter("okf_html") })
	assert.Zero(t, built, "a plugin's writer is not built to read its declaration")
	assert.Equal(t, []string{"href"}, reg.FormatInfo("okf_html").Writable("link:hyperlink"))
}
