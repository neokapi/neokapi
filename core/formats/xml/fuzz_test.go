package xml_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadXML drives the XML reader over input without ever calling t.Fatal, so
// it is safe on arbitrary fuzz input. An optional skeleton store selects the
// streaming read path; nil uses the buffered one.
func fuzzReadXML(ctx context.Context, input []byte, store *format.SkeletonStore) (parts []*model.Part, hadErr bool) {
	reader := xmlfmt.NewReader()
	if store != nil {
		reader.SetSkeletonStore(store)
	}
	if err := reader.Open(ctx, testutil.RawDocFromString(string(input), model.LocaleEnglish)); err != nil {
		return nil, true
	}
	defer reader.Close()
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			hadErr = true
			continue
		}
		if result.Part != nil {
			parts = append(parts, result.Part)
		}
	}
	return parts, hadErr
}

func fuzzXMLTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func fuzzXMLSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// seedDamagedXML adds documents that are structurally damaged but still
// PARSEABLE — the region where a lenient tokenizer silently absorbs content
// instead of reporting it.
//
// This is the corpus shape #1588 lives in. A one-byte change to a valid XLIFF
// file — corrupting the '<' of a closing tag — made three trans-units collapse
// into one block with no error. The xliff package had both a read fuzzer and a
// round-trip fuzzer and neither found it, because both were seeded only with
// well-formed documents: random mutation overwhelmingly lands in "malformed and
// correctly rejected" and rarely reaches "still parses, but a terminator is
// gone". Seeding that region directly is the fix.
//
// The generic XML reader is the widest such surface in the tree, so the damage
// below is aimed at element termination and nesting rather than at any one
// vocabulary.
func seedDamagedXML(f *testing.F) {
	f.Helper()
	for _, s := range []string{
		// The #1588 shape, transposed: the '<' of a closing tag replaced by a
		// letter. Worth keeping as a pinned observation rather than as a
		// suspected defect — the result is still well-formed XML, because
		// "x/s>" is simply character data, but that one stray character turns
		// <r> from element-only content into MIXED content. The reader then
		// treats <r> as a single translatable unit with inline children, so
		// three units silently become two blocks and the source text reads
		// "x/s>twothree". No error, and none is possible: without a schema,
		// generic XML cannot know that <r> was meant to hold elements only.
		// The same input round-trips stably, which is why the round-trip
		// oracle alone would never surface it.
		"<r><s>one</s>x/s><s>two</s><s>three</s></r>",
		// Closing tag with the slash removed.
		"<r><s>one<s><s>two</s></r>",
		// Mismatched closing tag name.
		"<r><s>one</t><s>two</s></r>",
		// Missing root close.
		"<r><s>one</s><s>two</s>",
		// Extra close beyond the root.
		"<r><s>one</s></r></r>",
		// Unterminated comment swallows the remaining elements.
		"<r><s>one</s><!-- never closed <s>two</s></r>",
		// Unterminated CDATA.
		"<r><s><![CDATA[text</s><s>two</s></r>",
		// Attribute value never quoted shut.
		"<r><s name=\"a>one</s><s>two</s></r>",
		// Entity reference that is never terminated.
		"<r><s>one &amp</s><s>two</s></r>",
		// A processing instruction that never closes.
		"<r><?pi never closed <s>one</s></r>",
		// Nested elements of the same name, inner never closed.
		"<r><s>outer<s>inner</s></r>",
		// Namespace prefix declared nowhere.
		"<r><ns:s>one</ns:s><s>two</s></r>",
		// Well-formed but with the XML declaration in the wrong place.
		"<r><?xml version=\"1.0\"?><s>one</s></r>",
	} {
		f.Add([]byte(s))
	}
}

// FuzzReadXml asserts the XML reader never panics and always terminates on
// arbitrary input. This is the framework's widest parser — a 2,400-line
// configurable reader that every XML-shaped vocabulary without a dedicated
// format falls back to — so it takes arbitrary user content directly.
func FuzzReadXml(f *testing.F) {
	fuzzXMLSeed(f, "simple.xml")
	f.Add([]byte(`<?xml version="1.0"?><r><s>text</s></r>`))
	f.Add([]byte(``))
	f.Add([]byte(`<r/>`))
	f.Add([]byte(`<r><a><b><c>deep</c></b></a></r>`))
	seedDamagedXML(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadXML(t.Context(), data, nil)
	})
}

// FuzzRoundTripXml asserts read → write → read is stable in content.
//
// A skeleton store is mandatory here, not incidental: without one the XML
// writer emits only the concatenated block text, which is not a document and
// cannot be re-read. With one, the writer reconstructs the original markup and
// the re-read is a real comparison. Using the in-memory store keeps each
// iteration off the filesystem.
//
// Known open failure: fuzzing this target rediscovers #1605 within two minutes.
// An element that begins with an element child, separates two children with
// whitespace only, and ends with non-whitespace text gains one byte of
// whitespace on every round-trip, without bound — "<r><a>x</a> <b>y</b> tail</r>"
// grows a space per pass. None of the seeds below trip it; it is recorded here
// so the failure is not mistaken for a new one.
func FuzzRoundTripXml(f *testing.F) {
	fuzzXMLSeed(f, "simple.xml")
	f.Add([]byte(`<?xml version="1.0"?><r><s>one</s><s>two</s></r>`))
	f.Add([]byte(`<r><s>text</s></r>`))
	seedDamagedXML(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		store := format.NewMemorySkeletonStore()
		parts1, hadErr := fuzzReadXML(ctx, data, store)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := fuzzXMLTexts(parts1)

		var buf bytes.Buffer
		writer := xmlfmt.NewWriter()
		writer.SetSkeletonStore(store)
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadXML(ctx, buf.Bytes(), format.NewMemorySkeletonStore())
		if hadErr2 {
			t.Fatalf("re-reading written XML failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := fuzzXMLTexts(parts2)
		if len(texts1) != len(texts2) {
			t.Fatalf("round-trip changed block count: %d -> %d\ninput:  %q\noutput: %q\npass1:  %q\npass2:  %q",
				len(texts1), len(texts2), data, buf.Bytes(), texts1, texts2)
		}
		for i := range texts1 {
			if texts1[i] != texts2[i] {
				t.Fatalf("round-trip drift in source text\npass1: %q\npass2: %q\ninput: %q\noutput: %q",
					texts1, texts2, data, buf.Bytes())
			}
		}
	})
}
