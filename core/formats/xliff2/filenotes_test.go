package xliff2_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringReader wraps strings.NewReader to satisfy io.ReadCloser.
type stringReader struct{ io.Reader }

func (stringReader) Close() error { return nil }

func newStringReader(s string) io.ReadCloser {
	return stringReader{Reader: strings.NewReader(s)}
}

// collectAllParts drains a PartResult channel into a flat slice, failing
// the test on any emitted error.
func collectAllParts(t *testing.T, ch <-chan model.PartResult) []*model.Part {
	t.Helper()
	var parts []*model.Part
	for r := range ch {
		require.NoError(t, r.Error)
		parts = append(parts, r.Part)
	}
	return parts
}

func TestXLIFF2_FileNotesRoundTrip(t *testing.T) {
	// Extract emits a file with kapi bookkeeping notes; merge parses the
	// same file and surfaces them on the layer so downstream code doesn't
	// re-parse XML.

	buf := &bytes.Buffer{}
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(buf))
	w.SetFileNotes([]xliff2.FileNote{
		xliff2.BatchIDNote("batch-abc-123"),
		xliff2.SourceFileNote("src/locales/en/app.json"),
		xliff2.SourceHashNote("sha256:deadbeef"),
	})

	// Minimal layer + one block so the writer has something to emit.
	layer := &model.Layer{
		ID:             "file-f1",
		Name:           "f1",
		Format:         "xliff2",
		Locale:         "en",
		IsMultilingual: true,
		Properties: map[string]string{
			"target-language": "fr",
		},
	}
	block := &model.Block{
		ID:     "u1",
		Source: []model.Run{{Text: &model.TextRun{Text: "Hello, world."}}},
	}
	block.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
	})

	parts := make(chan *model.Part, 3)
	parts <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	parts <- &model.Part{Type: model.PartBlock, Resource: block}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))

	output := buf.String()
	// Notes should appear before <unit> per XLIFF 2 ordering (notes are
	// children of <file>, before <group>/<unit>).
	assert.Contains(t, output, `<note id="batch-id" category="kapi">batch-abc-123</note>`)
	assert.Contains(t, output, `<note id="source-file" category="kapi">src/locales/en/app.json</note>`)
	assert.Contains(t, output, `<note id="source-hash" category="kapi">sha256:deadbeef</note>`)

	// Round-trip: the reader parses the same output and surfaces the notes
	// on the emitted Layer via the file-note:<category>:<id> property
	// convention.
	r := xliff2.NewReader()
	doc := &model.RawDocument{Reader: newStringReader(output)}
	require.NoError(t, r.Open(context.Background(), doc))
	allParts := collectAllParts(t, r.Read(context.Background()))
	require.NoError(t, r.Close())

	var foundLayer *model.Layer
	for _, p := range allParts {
		if p.Type == model.PartLayerStart {
			foundLayer = p.Resource.(*model.Layer)
			break
		}
	}
	require.NotNil(t, foundLayer)

	assert.Equal(t, "batch-abc-123", xliff2.BatchIDFromLayer(foundLayer))
	assert.Equal(t, "src/locales/en/app.json",
		xliff2.FilePropertyFromLayer(foundLayer, xliff2.FileNoteCategoryKapi, xliff2.FileNoteIDSourceFile))
	assert.Equal(t, "sha256:deadbeef",
		xliff2.FilePropertyFromLayer(foundLayer, xliff2.FileNoteCategoryKapi, xliff2.FileNoteIDSourceHash))
}

func TestXLIFF2_FileNotes_LayerCarryOverOnReemit(t *testing.T) {
	// A reader→writer round-trip (no kapi explicit notes set) preserves
	// file-level notes verbatim — important because a file that already
	// rode through once shouldn't lose its bookkeeping on a pass-through.

	const input = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.2" xmlns="urn:oasis:names:tc:xliff:document:2.2" srcLang="en" trgLang="fr">
  <file id="f1">
    <notes>
      <note id="batch-id" category="kapi">batch-xyz</note>
    </notes>
    <unit id="u1">
      <segment id="s1"><source>Hi</source></segment>
    </unit>
  </file>
</xliff>`

	r := xliff2.NewReader()
	doc := &model.RawDocument{Reader: newStringReader(input)}
	require.NoError(t, r.Open(context.Background(), doc))
	parts := collectAllParts(t, r.Read(context.Background()))
	require.NoError(t, r.Close())

	w := xliff2.NewWriter()
	buf := &bytes.Buffer{}
	require.NoError(t, w.SetOutputWriter(buf))

	// Feed the parts back into the writer to re-emit.
	partsCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		partsCh <- p
	}
	close(partsCh)
	require.NoError(t, w.Write(context.Background(), partsCh))

	got := buf.String()
	// Byte-exact is covered by skeleton elsewhere; here we check the
	// note survived the DOM->Parts->DOM bounce.
	assert.Contains(t, got, `<note id="batch-id" category="kapi">batch-xyz</note>`)
}

// Ensure the writer's explicit SetFileNotes wins over a note with the
// same (category, id) carried through from the reader — re-extraction
// must overwrite stale batch ids.
func TestXLIFF2_FileNotes_ExplicitOverridesLayer(t *testing.T) {
	layer := &model.Layer{
		ID: "file-f1", Name: "f1", Format: "xliff2", Locale: "en",
		Properties: map[string]string{
			"target-language": "fr",
			// Old batch id carried on the layer from a prior read.
			"file-note:kapi:batch-id": "stale-batch",
		},
	}
	block := &model.Block{
		ID:     "u1",
		Source: []model.Run{{Text: &model.TextRun{Text: "x"}}},
	}
	block.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
	})

	w := xliff2.NewWriter()
	buf := &bytes.Buffer{}
	require.NoError(t, w.SetOutputWriter(buf))
	w.SetFileNotes([]xliff2.FileNote{xliff2.BatchIDNote("fresh-batch")})

	ch := make(chan *model.Part, 2)
	ch <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	ch <- &model.Part{Type: model.PartBlock, Resource: block}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))

	out := buf.String()
	assert.Contains(t, out, `>fresh-batch<`)
	assert.NotContains(t, out, `stale-batch`)
}

// Shim: make sure BaseFormatWriter satisfies the Output interface we need.
var _ format.DataFormatWriter = xliff2.NewWriter()
