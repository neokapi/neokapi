package tmx_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layerFromParts returns the document Layer carried by the LayerStart/LayerEnd
// parts. Both parts reference the same *model.Layer, so either works.
func layerFromParts(t *testing.T, parts []*model.Part) *model.Layer {
	t.Helper()
	require.NotEmpty(t, parts)
	layer, ok := parts[len(parts)-1].Resource.(*model.Layer)
	require.True(t, ok, "last part should be the document Layer")
	return layer
}

// layerNotes returns the NoteAnnotation items attached to the layer, or nil.
func layerNotes(t *testing.T, layer *model.Layer) []*model.NoteAnnotation {
	t.Helper()
	v, ok := layer.Anno(model.AnnoNote)
	if !ok {
		return nil
	}
	notes, ok := v.(*model.Notes)
	require.True(t, ok, "AnnoNote payload should be *model.Notes")
	return notes.Items
}

// #928 finding B: header-level <note> text must be reachable as semantic
// metadata. It is carried on the header Data part (Properties["notes"]) AND as
// a layer-scoped model.NoteAnnotation — never as translatable content.
func TestHeaderNoteAsLayerAnnotation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="XYZTool" creationtoolversion="1.0" datatype="PlainText"
    segtype="sentence" adminlang="en-us" srclang="en-us" o-tmf="abc">
    <note>A header note.</note>
  </header>
  <body>
    <tu>
      <tuv xml:lang="en-us"><seg>Content</seg></tuv>
      <tuv xml:lang="fr"><seg>Contenu</seg></tuv>
    </tu>
  </body>
</tmx>`
	parts := readTMX(t, input)

	// The note text is reachable as a structured layer-level annotation.
	layer := layerFromParts(t, parts)
	notes := layerNotes(t, layer)
	require.Len(t, notes, 1)
	assert.Equal(t, "A header note.", notes[0].Text)
	assert.Equal(t, "tmx", notes[0].From)
	assert.Equal(t, "general", notes[0].Annotates)

	// And it remains carried on the header Data part (back-compat).
	var headerData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			if data, ok := p.Resource.(*model.Data); ok && data.Name == "tmx-header" {
				headerData = data
				break
			}
		}
	}
	require.NotNil(t, headerData)
	assert.Equal(t, "A header note.", headerData.Properties["notes"])

	// The note must NOT become a translatable block.
	for _, b := range testifyBlocks(parts) {
		assert.NotContains(t, b.SourceText(), "A header note.",
			"header note must not surface as translatable content")
	}
}

// Multiple header notes each surface as their own NoteAnnotation item, and the
// joined string property carries them all (newline-separated) for back-compat.
func TestHeaderMultipleNotesAsLayerAnnotations(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="XYZTool" srclang="en">
    <note>First header note</note>
    <note>Second header note</note>
  </header>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Content</seg></tuv>
    </tu>
  </body>
</tmx>`
	parts := readTMX(t, input)

	layer := layerFromParts(t, parts)
	notes := layerNotes(t, layer)
	require.Len(t, notes, 2)
	assert.Equal(t, "First header note", notes[0].Text)
	assert.Equal(t, "Second header note", notes[1].Text)

	var headerData *model.Data
	for _, p := range parts {
		if data, ok := p.Resource.(*model.Data); ok && data.Name == "tmx-header" {
			headerData = data
			break
		}
	}
	require.NotNil(t, headerData)
	assert.Equal(t, "First header note\nSecond header note", headerData.Properties["notes"])
}

// A TMX with no header note carries no layer-level note annotation.
func TestHeaderWithoutNoteHasNoLayerAnnotation(t *testing.T) {
	input := wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour</seg></tuv>
    </tu>`)
	parts := readTMX(t, input)
	layer := layerFromParts(t, parts)
	assert.Empty(t, layerNotes(t, layer))
}

// Surfacing the header note as an annotation does not change the byte-exact
// round-trip: annotations are not part of the serialized stream.
func TestHeaderNoteByteExactRoundTrip(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" srclang="en" datatype="plaintext">
    <note>A header note.</note>
  </header>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello World</seg>
      </tuv>
    </tu>
  </body>
</tmx>
`
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "header-note TMX roundtrip should be byte-exact")
}

// testifyBlocks filters the block parts out of a part slice.
func testifyBlocks(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				out = append(out, b)
			}
		}
	}
	return out
}
