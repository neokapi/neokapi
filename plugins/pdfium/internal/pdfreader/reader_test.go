package pdfreader

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// These contract tests were migrated from the retired in-core hand-rolled PDF
// reader (core/formats/pdf). The decoder-internal cases (TJ-array/operator
// parsing, manual FlateDecode, escaped-paren balancing) were dropped — PDFium
// handles all of that natively. What survives is the format-agnostic contract:
// real-world extraction, page/layer structure, geometry, and crash-safe
// handling of malformed input (the reason the reader lives in a plugin).
//
// They run in-process against ReadParts (cgo + libpdfium on PKG_CONFIG_PATH);
// `make test-pdfium-plugin` wires the toolchain.

func collectBlocks(parts []*model.Part) []*model.Block {
	var bs []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				bs = append(bs, b)
			}
		}
	}
	return bs
}

func blockText(bs []*model.Block) string {
	var sb strings.Builder
	for _, b := range bs {
		sb.WriteString(b.SourceText())
		sb.WriteByte(' ')
	}
	return sb.String()
}

// requireBalancedLayers asserts the part stream opens with a document
// LayerStart, closes with a LayerEnd, and that every LayerStart is matched.
func requireBalancedLayers(t *testing.T, parts []*model.Part) {
	t.Helper()
	require.NotEmpty(t, parts)
	require.Equal(t, model.PartLayerStart, parts[0].Type, "stream opens with the document layer")
	require.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "stream closes the document layer")
	depth := 0
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			depth++
		case model.PartLayerEnd:
			depth--
		}
		require.GreaterOrEqual(t, depth, 0, "LayerEnd without a matching LayerStart")
	}
	require.Zero(t, depth, "every LayerStart must be closed")
}

// migrated from TestReadCompressedRealWorldPDF / TestStartDocument: a large,
// compressed, multi-page real-world PDF inflates and yields substantive text,
// with a well-formed document layer.
func TestReadParts_RealWorld(t *testing.T) {
	data, err := os.ReadFile("testdata/TAUS-QualityDashboard-September.pdf")
	require.NoError(t, err)

	parts, err := ReadParts(data, model.LocaleEnglish, "TAUS-QualityDashboard-September.pdf", Options{})
	require.NoError(t, err)
	requireBalancedLayers(t, parts)

	root, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "pdf", root.Format)

	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "compressed PDF must inflate and extract text")
	assert.Contains(t, blockText(blocks), "TAUS")
}

// migrated from TestPageLayers / TestReadLayerStartEnd: per-page Layers carry a
// page-number property starting at 1, nested inside the document layer.
func TestReadParts_PageStructure(t *testing.T) {
	data, err := os.ReadFile("testdata/multi.pdf")
	require.NoError(t, err)

	parts, err := ReadParts(data, model.LocaleEnglish, "multi.pdf", Options{})
	require.NoError(t, err)
	requireBalancedLayers(t, parts)

	var pageLayers []*model.Layer
	for _, p := range parts {
		if p.Type != model.PartLayerStart {
			continue
		}
		l := p.Resource.(*model.Layer)
		if _, ok := l.Properties["page-number"]; ok {
			pageLayers = append(pageLayers, l)
		}
	}
	require.GreaterOrEqual(t, len(pageLayers), 1, "at least one page layer")
	assert.Equal(t, "1", pageLayers[0].Properties["page-number"], "page numbering is 1-based")
}

// the fast path (Geometry:false) emits one plain-text block per page with no
// geometry; geometry mode emits positioned blocks carrying a top-left
// GeometryAnnotation.
func TestReadParts_Modes(t *testing.T) {
	data, err := os.ReadFile("testdata/multi.pdf")
	require.NoError(t, err)

	fast, err := ReadParts(data, model.LocaleEnglish, "multi.pdf", Options{})
	require.NoError(t, err)
	fastBlocks := collectBlocks(fast)
	require.NotEmpty(t, fastBlocks)
	for _, b := range fastBlocks {
		_, hasGeo := b.Geometry()
		assert.False(t, hasGeo, "fast path carries no geometry")
		assert.NotEmpty(t, b.Properties["page-number"], "fast path stamps page-number")
	}

	geo, err := ReadParts(data, model.LocaleEnglish, "multi.pdf", Options{Geometry: true})
	require.NoError(t, err)
	geoBlocks := collectBlocks(geo)
	require.NotEmpty(t, geoBlocks)
	var positioned int
	for _, b := range geoBlocks {
		if g, ok := b.Geometry(); ok && g.BBox.W > 0 {
			positioned++
			assert.Equal(t, "top-left", g.Origin, "geometry is flipped to top-left origin")
		}
	}
	require.Positive(t, positioned, "geometry mode carries positioned blocks")
}

// an empty (but structurally valid) PDF opens cleanly and yields no text blocks
// — no error, no panic.
func TestReadParts_Empty(t *testing.T) {
	data, err := os.ReadFile("testdata/empty.pdf")
	require.NoError(t, err)

	var parts []*model.Part
	require.NotPanics(t, func() {
		parts, err = ReadParts(data, model.LocaleEnglish, "empty.pdf", Options{})
	})
	require.NoError(t, err)
	assert.Empty(t, collectBlocks(parts), "empty PDF yields no text blocks")
}

// migrated from TestReadMalformedPDF: truncated, garbage, and non-PDF byte
// sequences must be handled crash-safe. PDFium rejects them at OpenDocument, so
// ReadParts returns a clean error (vs. the old lenient scanner's silent empty);
// either way the contract is "never panic". This is the whole reason the reader
// is isolated in a plugin subprocess.
func TestReadParts_Malformed(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"nil bytes", nil},
		{"not a pdf", []byte("definitely not a pdf at all")},
		{"header only", []byte("%PDF-1.7")},
		{"truncated after stream keyword", []byte("%PDF-1.7\n4 0 obj\n<< /Length 44 >>\nstream")},
		{"flatedecode header but garbage stream", []byte("<< /Filter /FlateDecode >>\nstream\n\x00\x01\x02\xff\xfe\xfd\nendstream")},
		{"random control bytes", []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x00, 0xff}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				parts, err := ReadParts(tc.input, model.LocaleEnglish, tc.name, Options{})
				// Crash-safe contract: malformed input either errors cleanly or
				// drains to no blocks. It must never panic or hang.
				if err == nil {
					assert.Empty(t, collectBlocks(parts), "no text from malformed bytes")
				} else {
					assert.Error(t, err)
				}
			})
		})
	}
}
