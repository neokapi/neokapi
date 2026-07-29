package xliff2_test

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadXliff2 drives the XLIFF 2 reader over input without ever calling
// t.Fatal, so it is safe on arbitrary fuzz input.
func fuzzReadXliff2(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := xliff2.NewReader()
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

func fuzzXliff2Texts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

// xliff2 has no testdata/ directory — its fixtures come from the external Okapi
// corpus, which is not present in a plain checkout — so every seed here is
// inline and deliberately small.
const xliff2Head = `<?xml version="1.0"?><xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr"><file id="f1">`

// seedDamagedXliff2 adds documents that are structurally damaged but still
// PARSEABLE. This is the corpus shape that matters most in this package, for a
// direct reason: #1588 was found in XLIFF 1.2 by exhaustive single-byte
// mutation — corrupting the '<' of a </trans-unit> made the lenient tokenizer
// swallow every following unit, three units becoming one block with no error —
// and that package already had both a read fuzzer and a round-trip fuzzer.
// Both were seeded only with well-formed documents, so neither ever reached the
// region the bug lived in. XLIFF 2 is the sibling parser; these seeds put the
// fuzzer inside that region from the first iteration.
func seedDamagedXliff2(f *testing.F) {
	f.Helper()
	for _, s := range []string{
		// The #1588 shape exactly: the '<' of </unit> corrupted.
		xliff2Head + `<unit id="1"><segment><source>one</source></segment></unit>x/unit><unit id="2"><segment><source>two</source></segment></unit><unit id="3"><segment><source>three</source></segment></unit></file></xliff>`,
		// </unit> missing entirely between units.
		xliff2Head + `<unit id="1"><segment><source>one</source></segment><unit id="2"><segment><source>two</source></segment></unit></file></xliff>`,
		// </segment> missing.
		xliff2Head + `<unit id="1"><segment><source>one</source></unit><unit id="2"><segment><source>two</source></segment></unit></file></xliff>`,
		// </source> missing — following markup becomes source text.
		xliff2Head + `<unit id="1"><segment><source>one</segment></unit><unit id="2"><segment><source>two</source></segment></unit></file></xliff>`,
		// A segment with a target but no source.
		xliff2Head + `<unit id="1"><segment><target>un</target></segment></unit></file></xliff>`,
		// Inline code opened and never closed.
		xliff2Head + `<unit id="1"><segment><source>one <pc id="p1">two</source></segment></unit></file></xliff>`,
		// Mismatched inline code ids.
		xliff2Head + `<unit id="1"><segment><source><pc id="a">x</pc id="b"></source></segment></unit></file></xliff>`,
		// <file> never closed.
		xliff2Head + `<unit id="1"><segment><source>one</source></segment></unit></xliff>`,
		// The default namespace removed, so nothing is in the XLIFF namespace.
		`<?xml version="1.0"?><xliff version="2.0" srcLang="en" trgLang="fr"><file id="f1"><unit id="1"><segment><source>one</source></segment></unit></file></xliff>`,
		// An ignorable element between units.
		xliff2Head + `<unit id="1"><segment><source>one</source></segment></unit><ignorable><source> </source></ignorable><unit id="2"><segment><source>two</source></segment></unit></file></xliff>`,
	} {
		f.Add([]byte(s))
	}
}

// seedCollidingUnitIDXliff2 holds the documents whose unit ids collide. XLIFF 2
// requires a unique id on every unit, so all of these are invalid — but kapi
// accepts them, and used to produce a plausible wrong answer: the writer
// resolved every unit element through one id-keyed map entry, so a collision
// patched one unit with another's content, overwritten rather than merely
// dropped (#1599). Two ways to collide, and the fix covers both:
//
//   - two units declaring the same id;
//   - two units declaring NO id, which both resolve to the empty-string key.
func seedCollidingUnitIDXliff2(f *testing.F) {
	f.Helper()
	f.Add([]byte(xliff2Head + `<unit id="1"><segment><source>one</source></segment></unit><unit id="1"><segment><source>two</source></segment></unit></file></xliff>`))
	f.Add([]byte(xliff2Head + `<unit><segment><source>one</source></segment></unit><unit><segment><source></source></segment></unit></file></xliff>`))
	f.Add([]byte(xliff2Head + `<unit><segment><source>bare</source></segment></unit><unit id="u1"><segment><source>named</source></segment></unit><unit><segment><source>bare2</source></segment></unit></file></xliff>`))
	f.Add([]byte(xliff2Head + `<group id="g1"><unit id="1"><segment><source>one</source></segment></unit><unit id="1"><segment><source>two</source></segment></unit></group></file></xliff>`))
}

// FuzzReadXliff2 asserts the XLIFF 2 reader never panics and always terminates
// on arbitrary input. XLIFF is an interchange format: the documents it parses
// arrive from other tools, and in the packaged-handoff workflow from outside
// the project entirely.
func FuzzReadXliff2(f *testing.F) {
	seedCollidingUnitIDXliff2(f)
	f.Add([]byte(xliff2Head + `<unit id="1"><segment><source>Hello</source><target>Bonjour</target></segment></unit></file></xliff>`))
	f.Add([]byte(xliff2Head + `</file></xliff>`))
	f.Add([]byte(``))
	f.Add([]byte(`<xliff version="2.0"/>`))
	seedDamagedXliff2(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadXliff2(t.Context(), data)
	})
}

// FuzzRoundTripXliff2 asserts read → write → read is stable in content. The
// XLIFF 2 writer patches the source DOM, which travels inside the part stream
// rather than through a setter, so the round-trip is self-contained.
func FuzzRoundTripXliff2(f *testing.F) {
	seedCollidingUnitIDXliff2(f)
	f.Add([]byte(xliff2Head + `<unit id="1"><segment><source>Hello</source><target>Bonjour</target></segment></unit></file></xliff>`))
	f.Add([]byte(xliff2Head + `<unit id="1"><segment><source>one</source></segment></unit><unit id="2"><segment><source>two</source></segment></unit></file></xliff>`))
	seedDamagedXliff2(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := fuzzReadXliff2(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := fuzzXliff2Texts(parts1)

		var buf bytes.Buffer
		writer := xliff2.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		// No SetLocale: preserve whatever trgLang the source declared.
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadXliff2(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written XLIFF 2 failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := fuzzXliff2Texts(parts2)
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
