package formats

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/require"
)

// TestDeclaredEncodingIsMetadataOnly pins what RawDocument.Encoding does today:
// it is carried onto the layer and nothing decodes by it.
//
// Readers treat the byte stream as UTF-8 whatever the document declares, and no
// writer consumes BaseFormatWriter.Encoding, so `--encoding` and the recipe's
// `defaults.encoding` select a label rather than a codec. A single-byte legacy
// file therefore reaches the content model as invalid UTF-8. Tracked in
// https://github.com/neokapi/neokapi/issues/1714; this test fails the moment
// that changes, which is the signal to rewrite it as the positive assertion.
func TestDeclaredEncodingIsMetadataOnly(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	// "greeting=Grüße\n" in ISO-8859-1: ü is 0xFC, ß is 0xDF — both invalid
	// UTF-8 on their own.
	latin1 := []byte("greeting=Gr\xfc\xdfe\n")

	reader, err := reg.NewReader("properties")
	require.NoError(t, err)
	doc := &model.RawDocument{
		SourceLocale: model.LocaleID("de"),
		Encoding:     "ISO-8859-1",
		Reader:       io.NopCloser(bytes.NewReader(latin1)),
	}
	require.NoError(t, reader.Open(context.Background(), doc))
	t.Cleanup(func() { _ = reader.Close() })

	var layer *model.Layer
	var texts []string
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		switch pr.Part.Type {
		case model.PartLayerStart:
			layer, _ = pr.Part.Resource.(*model.Layer)
		case model.PartBlock:
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				texts = append(texts, model.RenderRunsWithData(b.Source))
			}
		}
	}

	require.NotNil(t, layer)
	require.Equal(t, "ISO-8859-1", layer.Encoding, "the declaration reaches the layer")
	require.Equal(t, []string{"Gr\xfc\xdfe"}, texts,
		"the bytes reach the content model undecoded — the declaration selected no codec")
}
