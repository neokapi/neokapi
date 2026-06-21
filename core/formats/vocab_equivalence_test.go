package formats_test

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/jsx"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVocabEquivalence is the cross-format Vocabulary-axis V2 equivalence
// check (docs/internals/format-maturity.md §2.2): one shared fixture
// sentence — bold, italic, link — read through each participating format
// must yield the SAME canonical run-Type sequence
// ([fmt:bold, fmt:italic, link:hyperlink]). The shared `want` value is the
// reference: every participant is asserted equal to it, so they are
// transitively identical to one another.
//
// Construct coverage decisions, grounded in each reader's code:
//
//   - The fixture deliberately uses only bold/italic/link. These three
//     map to identical canonical types across html, markdown, xliff(1.2),
//     and jsx (html htmlSemanticTypes; markdown reader.go emphasis/strong/
//     link switch; xliff reader.go ctypeToSpanType; jsx rich-jsx pack which
//     extends common-formatting).
//
//   - IMAGE is not in the shared fixture only to keep it minimal; the former
//     markdown divergence (a format-local "link:image") has been resolved —
//     the markdown reader now emits the canonical "media:image" matching
//     html/xliff (core/formats/markdown/reader.go), carrying src/alt/title as
//     format-neutral Attrs. Cross-format image fidelity is covered by the
//     core/formats TestCrossFormat_* suite.
//
//   - OPENXML is excluded from this table: its native fixture is a .docx
//     binary, which needs an in-test OOXML/zip builder this file does not
//     have. TODO(vocab-equivalence): add an openxml case once a minimal
//     docx-builder fixture helper exists (it would carry <w:b>/<w:i>/
//     <w:hyperlink> and assert the same [fmt:bold, fmt:italic,
//     link:hyperlink] sequence — openxml's reader already maps these, see
//     core/formats/openxml/vocabulary.go + reader_test.go).
//
//   - jsx's native interchange is the typed .klf envelope, so its reader
//     pass-through validates that canonical types carried in KLF survive
//     unchanged rather than markup→type inference. It still produces the
//     same canonical sequence and is a legitimate participant.
func TestVocabEquivalence(t *testing.T) {
	// The shared canonical Type sequence for "bold, italic, link".
	want := []string{"fmt:bold", "fmt:italic", "link:hyperlink"}

	cases := []struct {
		format string
		runs   func(t *testing.T) []model.Run
	}{
		{
			format: "html",
			runs: func(t *testing.T) []model.Run {
				return firstBlockRuns(t, htmlfmt.NewReader(), testutil.RawDocFromString(
					`<html><body><p>A <b>bold</b> b <i>italic</i> c `+
						`<a href="https://example.com">link</a> d</p></body></html>`,
					model.LocaleEnglish))
			},
		},
		{
			format: "markdown",
			runs: func(t *testing.T) []model.Run {
				return firstBlockRuns(t, markdown.NewReader(), testutil.RawDocFromString(
					"A **bold** b *italic* c [link](https://example.com) d",
					model.LocaleEnglish))
			},
		},
		{
			format: "xliff",
			runs: func(t *testing.T) []model.Run {
				return firstBlockRuns(t, xliff.NewReader(), testutil.RawDocFromString(
					xliff12Fixture, model.LocaleEnglish))
			},
		},
		{
			format: "jsx",
			runs: func(t *testing.T) []model.Run {
				return firstBlockRuns(t, jsx.NewReader(), jsxKLFFixture(t))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			got := canonicalRunTypes(tc.runs(t))
			assert.Equal(t, want, got,
				"%s must read the shared fixture into the canonical Type sequence", tc.format)
		})
	}
}

// vocabReader is the minimal reader surface the equivalence cases need;
// every participating format's *Reader satisfies it.
type vocabReader interface {
	Open(context.Context, *model.RawDocument) error
	Read(context.Context) <-chan model.PartResult
	Close() error
}

// firstBlockRuns opens the reader on raw, drains the stream, and returns
// the source runs of the first translatable block.
func firstBlockRuns(t *testing.T, r vocabReader, raw *model.RawDocument) []model.Run {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, r.Open(ctx, raw))
	defer r.Close()
	blocks := testutil.CollectBlocks(t, r.Read(ctx))
	require.NotEmpty(t, blocks, "expected at least one block")
	return blocks[0].SourceRuns()
}

var canonicalTypeRE = regexp.MustCompile(`^(fmt|link|media|code):`)

// canonicalRunTypes collects the canonical Type of each opening / standalone
// inline run (PcOpen + Ph), in document order, filtered to the canonical
// run-type namespaces. Closing runs are skipped so each construct appears
// once at its opening (XLIFF 1.2 <ept> carries no ctype, so its PcClose is
// untyped anyway — collecting opens keeps the comparison reader-agnostic).
func canonicalRunTypes(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		var typ string
		switch {
		case r.PcOpen != nil:
			typ = r.PcOpen.Type
		case r.Ph != nil:
			typ = r.Ph.Type
		default:
			continue
		}
		if canonicalTypeRE.MatchString(typ) {
			out = append(out, typ)
		}
	}
	return out
}

// xliff12Fixture is an XLIFF 1.2 trans-unit whose <source> carries three
// ctype-typed inline-code pairs: bold, italic, link.
const xliff12Fixture = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="eq">
    <body>
      <trans-unit id="1">
        <source>A <bpt id="1" ctype="bold">&lt;b&gt;</bpt>bold<ept id="1">&lt;/b&gt;</ept> b <bpt id="2" ctype="italic">&lt;i&gt;</bpt>italic<ept id="2">&lt;/i&gt;</ept> c <bpt id="3" ctype="link">&lt;a&gt;</bpt>link<ept id="3">&lt;/a&gt;</ept> d</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

// jsxKLFFixture builds an in-memory .klf (jsx's native interchange) whose
// single block carries bold/italic/link inline-code runs, then returns it
// as a RawDocument for the jsx reader.
func jsxKLFFixture(t *testing.T) *model.RawDocument {
	t.Helper()
	file := &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "vocab-equivalence", Version: "0.0.1"},
		Project:       klf.ProjectInfo{ID: "vocab-equivalence", SourceLocale: "en"},
		Documents: []klf.Document{{
			ID:           "eq",
			DocumentType: klf.DocumentTypeJSX,
			Path:         "eq.tsx",
			Blocks: []klf.Block{{
				ID:           "eq-1",
				Hash:         "eq0001",
				Translatable: true,
				Type:         klf.BlockTypeJSXElement,
				Source: []klf.Run{
					{Text: &klf.TextRun{Text: "A "}},
					{PcOpen: &klf.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
					{Text: &klf.TextRun{Text: "bold"}},
					{PcClose: &klf.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
					{Text: &klf.TextRun{Text: " b "}},
					{PcOpen: &klf.PcOpenRun{ID: "2", Type: "fmt:italic", Data: "<i>"}},
					{Text: &klf.TextRun{Text: "italic"}},
					{PcClose: &klf.PcCloseRun{ID: "2", Type: "fmt:italic", Data: "</i>"}},
					{Text: &klf.TextRun{Text: " c "}},
					{PcOpen: &klf.PcOpenRun{ID: "3", Type: "link:hyperlink", Data: `<a href="https://example.com">`}},
					{Text: &klf.TextRun{Text: "link"}},
					{PcClose: &klf.PcCloseRun{ID: "3", Type: "link:hyperlink", Data: "</a>"}},
					{Text: &klf.TextRun{Text: " d"}},
				},
			}},
		}},
	}
	data, err := klf.Marshal(file)
	require.NoError(t, err)
	return &model.RawDocument{URI: "eq.klf", Reader: io.NopCloser(bytes.NewReader(data))}
}
