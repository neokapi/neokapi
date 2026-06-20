package odf_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// readPartsWithExtract reads an ODF document, optionally toggling the
// ExtractNonTranslatableContent flag off (parity-style) before reading.
func readPartsWithExtract(t *testing.T, data []byte, extract bool) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := odf.NewReader()
	if !extract {
		cfg, ok := reader.Config().(*odf.Config)
		require.True(t, ok, "config must be *odf.Config")
		cfg.SetExtractNonTranslatableContent(false)
	}
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odf", model.LocaleEnglish)
	require.NoError(t, reader.Open(ctx, doc))
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// zipEntry extracts the named entry bytes from a ZIP archive.
func zipEntry(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()
			var buf bytes.Buffer
			_, err = buf.ReadFrom(rc)
			require.NoError(t, err)
			return buf.Bytes()
		}
	}
	t.Fatalf("zip entry %q not found", name)
	return nil
}

// page-anchored frame (ODP) carrying image accessibility text (svg:title /
// svg:desc) but no text-box prose.
const odpAccessibilityContent = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0">
<office:body><office:presentation>
<draw:page>
<draw:frame svg:x="1cm" svg:y="2cm" svg:width="72pt" svg:height="36pt">
<svg:title>Company logo</svg:title>
<svg:desc>The full company logo in blue</svg:desc>
</draw:frame>
</draw:page>
</office:presentation></office:body></office:document-content>`

// ODT form controls bearing display attributes (form:label / form:title /
// form:help-text). Controls use explicit open/close tags so the skeleton
// round-trip is not confounded by the reader's self-closing-tag expansion.
const odtFormControlContent = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:form="urn:oasis:names:tc:opendocument:xmlns:form:1.0">
<office:body><office:text>
<office:forms>
<form:form form:name="Form1" form:apply-design-mode="false">
<form:button form:name="b1" form:label="Click me" form:title="Submit now" form:help-text="Press to submit"></form:button>
</form:form>
</office:forms>
<text:p>Body paragraph</text:p>
</office:text></office:body></office:document-content>`

// --- Finding A1: image accessibility text (svg:title / svg:desc) ---

func TestExtractAccessibilityText_ODP(t *testing.T) {
	data := makeODFZip(mimeODP, odpAccessibilityContent)
	parts := readPartsWithExtract(t, data, true)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2, "svg:title + svg:desc surface as content blocks")
	for _, b := range blocks {
		assert.False(t, b.Translatable, "accessibility text is non-translatable")
		assert.Equal(t, model.RoleCaption, b.SemanticRole(), "alt-text/long-desc is a caption")
		require.Len(t, b.Source, 1, "single verbatim source run (no inline parse)")
		require.NotNil(t, b.Source[0].Text, "source run is plain text")
		g, ok := b.Geometry()
		require.True(t, ok, "block anchored to the enclosing draw:frame")
		assert.Equal(t, 1, g.Page, "frame on presentation page 1")
		assert.InDelta(t, 72.0, g.BBox.W, 0.01)
	}

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Company logo")
	assert.Contains(t, texts, "The full company logo in blue")

	// None of the surfaced content is part of the translatable/MT payload.
	for _, b := range blocks {
		assert.False(t, b.Translatable)
	}
}

func TestExtractAccessibilityText_OffByFlag(t *testing.T) {
	data := makeODFZip(mimeODP, odpAccessibilityContent)

	on := testutil.FilterBlocks(readPartsWithExtract(t, data, true))
	off := testutil.FilterBlocks(readPartsWithExtract(t, data, false))

	require.Len(t, on, 2)
	require.Empty(t, off, "with the flag off, svg:title/svg:desc stay in skeleton")
}

func TestExtractAccessibilityText_ODTInlineUnaffected(t *testing.T) {
	// In ODT the frame sits inside text:p, so the accessibility text is already
	// captured inside the translatable paragraph — no separate caption block.
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0">
<office:body><office:text>
<text:p>Before<draw:frame><svg:title>Logo</svg:title></draw:frame>After</text:p>
</office:text></office:body></office:document-content>`
	data := makeODFZip(mimeODT, content)
	blocks := testutil.FilterBlocks(readPartsWithExtract(t, data, true))

	require.Len(t, blocks, 1, "only the enclosing translatable paragraph")
	assert.True(t, blocks[0].Translatable)
}

// --- Finding A2: form-control display attributes ---

func TestExtractFormDisplayAttributes(t *testing.T) {
	data := makeODFZip(mimeODT, odtFormControlContent)
	parts := readPartsWithExtract(t, data, true)
	blocks := testutil.FilterBlocks(parts)

	// 3 non-translatable form display strings + 1 translatable body paragraph.
	require.Len(t, blocks, 4)

	var caps []*model.Block
	var body *model.Block
	for _, b := range blocks {
		if b.Translatable {
			body = b
		} else {
			caps = append(caps, b)
		}
	}
	require.NotNil(t, body)
	assert.Equal(t, "Body paragraph", body.SourceText())

	require.Len(t, caps, 3)
	texts := testutil.BlockTexts(caps)
	assert.Contains(t, texts, "Click me")        // form:label
	assert.Contains(t, texts, "Submit now")      // form:title
	assert.Contains(t, texts, "Press to submit") // form:help-text
	for _, b := range caps {
		assert.False(t, b.Translatable)
		assert.Equal(t, "button", b.Properties["element"])
		require.Len(t, b.Source, 1)
		require.NotNil(t, b.Source[0].Text)
	}
}

func TestExtractFormDisplayAttributes_OffByFlag(t *testing.T) {
	data := makeODFZip(mimeODT, odtFormControlContent)

	on := testutil.FilterBlocks(readPartsWithExtract(t, data, true))
	off := testutil.FilterBlocks(readPartsWithExtract(t, data, false))

	require.Len(t, on, 4)
	require.Len(t, off, 1, "with the flag off only the body paragraph remains")
	assert.True(t, off[0].Translatable)
	assert.Equal(t, "Body paragraph", off[0].SourceText())
}

// form:* non-display attributes (apply-design-mode, automatic-focus) are never
// surfaced, even with the flag on.
func TestFormNonDisplayAttributesIgnored(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:form="urn:oasis:names:tc:opendocument:xmlns:form:1.0">
<office:body><office:text>
<office:forms><form:form form:name="Form1" form:apply-design-mode="false" form:automatic-focus="false"></form:form></office:forms>
<text:p>Body</text:p>
</office:text></office:body></office:document-content>`
	data := makeODFZip(mimeODT, content)
	blocks := testutil.FilterBlocks(readPartsWithExtract(t, data, true))

	require.Len(t, blocks, 1, "no display attributes → only the body paragraph")
	assert.True(t, blocks[0].Translatable)
}

// --- byte-exact skeleton round-trip with surfacing ON ---

func TestNonTranslatableContent_ByteExactSkeletonRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		mime    string
		content string
	}{
		{"odp_accessibility", mimeODP, odpAccessibilityContent},
		{"odt_form_controls", mimeODT, odtFormControlContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			data := makeODFZip(tc.mime, tc.content)
			originalXML := zipEntry(t, data, "content.xml")

			skel, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer skel.Close()

			reader := odf.NewReader()
			reader.SetSkeletonStore(skel)
			require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), "t.odf", model.LocaleEnglish)))
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			// No translation: a source round-trip must reproduce the input bytes.
			var buf bytes.Buffer
			writer := odf.NewWriter()
			writer.SetOriginalContent(data)
			writer.SetSkeletonStore(skel)
			require.NoError(t, writer.SetOutputWriter(&buf))
			require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
			require.NoError(t, writer.Close())

			outXML := zipEntry(t, buf.Bytes(), "content.xml")
			assert.Equal(t, string(originalXML), string(outXML),
				"content.xml must round-trip byte-exact with surfacing on")
		})
	}
}

// --- config plumbing ---

func TestExtractNonTranslatableContentConfig(t *testing.T) {
	cfg := &odf.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "default on")

	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	// Reset restores the opt-out default (on).
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent())
}

func TestSchemaHasExtractNonTranslatableContent(t *testing.T) {
	cfg := &odf.Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	prop, ok := s.Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema must expose the flag")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}
