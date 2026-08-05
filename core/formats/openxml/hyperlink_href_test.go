package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A hyperlink's destination is not in the `<w:hyperlink>` element — `r:id` names
// a relationship, and only resolving it against the part's .rels yields a URL.
// The reader preserves the start tag verbatim for the skeleton write path and
// does not synthesise an `href` onto it (upstream Okapi's reference output never
// carries one). For a long time that left the canonical layer empty too, so
// every cross-format export produced `[text]()`.
//
// These tests pin both halves: the canonical attribute is populated, and the
// verbatim markup is untouched.

// docxWith builds a minimal .docx around the given document.xml body and rels.
func docxWith(t *testing.T, body, rels string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`+
		`</Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>`+
		`</Relationships>`)
	add("word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+rels+`</Relationships>`)
	add("word/document.xml", `<?xml version="1.0" encoding="UTF-8"?>`+
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" `+
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<w:body>`+body+`</w:body></w:document>`)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// readRuns reads a .docx and returns the runs of its first block carrying any.
func readRuns(t *testing.T, data []byte) []model.Run {
	t.Helper()
	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          "test.docx",
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		b, ok := pr.Part.Resource.(*model.Block)
		if !ok || len(b.Source) == 0 {
			continue
		}
		for _, run := range b.Source {
			if run.PcOpen != nil && run.PcOpen.Type == TypeHyperlink {
				return b.Source
			}
		}
	}
	t.Fatal("no block with a hyperlink was read")
	return nil
}

// pcOpen returns the first hyperlink PcOpen run.
func pcOpen(t *testing.T, runs []model.Run) *model.PcOpenRun {
	t.Helper()
	for _, r := range runs {
		if r.PcOpen != nil && r.PcOpen.Type == TypeHyperlink {
			return r.PcOpen
		}
	}
	t.Fatal("no hyperlink PcOpen run")
	return nil
}

func TestHyperlinkExternalTargetReachesAttrHref(t *testing.T) {
	data := docxWith(t,
		`<w:p><w:r><w:t>Click </w:t></w:r>`+
			`<w:hyperlink r:id="rId9"><w:r><w:t>here</w:t></w:r></w:hyperlink>`+
			`<w:r><w:t> for more.</w:t></w:r></w:p>`,
		`<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/docs" TargetMode="External"/>`)

	open := pcOpen(t, readRuns(t, data))
	assert.Equal(t, "https://example.com/docs", open.Attrs[model.AttrHref],
		"the relationship target should reach the canonical href attribute")

	// The verbatim markup the skeleton path replays must be untouched — in
	// particular it must NOT gain a synthesized href attribute.
	assert.Contains(t, open.Data, `r:id="rId9"`)
	assert.NotContains(t, open.Data, "href=")
}

func TestHyperlinkTooltipReachesAttrTitle(t *testing.T) {
	data := docxWith(t,
		`<w:p><w:hyperlink r:id="rId9" w:tooltip="Read the docs" w:history="1">`+
			`<w:r><w:t>here</w:t></w:r></w:hyperlink></w:p>`,
		`<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com" TargetMode="External"/>`)

	open := pcOpen(t, readRuns(t, data))
	assert.Equal(t, "https://example.com", open.Attrs[model.AttrHref])
	assert.Equal(t, "Read the docs", open.Attrs[model.AttrTitle])

	// Every non-r:id attribute still round-trips verbatim.
	assert.Contains(t, open.Data, `w:tooltip="Read the docs"`)
	assert.Contains(t, open.Data, `w:history="1"`)
}

// An internal jump has no relationship at all: w:anchor names a bookmark in the
// same document, which is a #fragment destination.
func TestHyperlinkAnchorBecomesFragment(t *testing.T) {
	data := docxWith(t,
		`<w:p><w:hyperlink w:anchor="section2"><w:r><w:t>see below</w:t></w:r></w:hyperlink></w:p>`,
		``)

	open := pcOpen(t, readRuns(t, data))
	assert.Equal(t, "#section2", open.Attrs[model.AttrHref])
}

// A dangling r:id — the relationship is missing — must not invent a destination.
func TestHyperlinkUnresolvedRelationshipHasNoHref(t *testing.T) {
	data := docxWith(t,
		`<w:p><w:hyperlink r:id="rIdMissing"><w:r><w:t>broken</w:t></w:r></w:hyperlink></w:p>`,
		``)

	open := pcOpen(t, readRuns(t, data))
	assert.Empty(t, open.Attrs[model.AttrHref],
		"an unresolvable relationship should leave the destination empty, not guess one")
	assert.Contains(t, open.Data, `r:id="rIdMissing"`, "the markup still round-trips")
}
