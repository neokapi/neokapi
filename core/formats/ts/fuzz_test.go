package ts_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadTS drives the Qt Linguist reader over input without ever calling
// t.Fatal, so it is safe on arbitrary fuzz input.
func fuzzReadTS(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := ts.NewReader()
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

func fuzzTSTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func fuzzTSSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// seedDamagedTS adds documents that are structurally damaged but still
// PARSEABLE. See the seedDamagedXML comment in core/formats/xml/fuzz_test.go
// for why this corpus shape matters (#1588) — .ts is the same family of
// message-container XML that the bug lived in, so the same damage applies:
// a lost <message> or <context> terminator is where units go missing quietly.
func seedDamagedTS(f *testing.F) {
	f.Helper()
	const head = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE TS []><TS version="2.1" language="fr" sourcelanguage="en">`
	for _, s := range []string{
		// The #1588 shape: the '<' of </message> corrupted, so the closing tag
		// becomes text and the following messages may be absorbed.
		head + `<context><name>C</name><message><source>one</source><translation>un</translation>x/message><message><source>two</source><translation>deux</translation></message></context></TS>`,
		// </message> missing entirely between two messages.
		head + `<context><name>C</name><message><source>one</source><translation>un</translation><message><source>two</source><translation>deux</translation></message></context></TS>`,
		// <context> never closed.
		head + `<context><name>C</name><message><source>one</source><translation>un</translation></message></TS>`,
		// A message with no <source>.
		head + `<context><name>C</name><message><translation>un</translation></message></context></TS>`,
		// A message with two <source> elements.
		head + `<context><name>C</name><message><source>one</source><source>two</source><translation>un</translation></message></context></TS>`,
		// numerus="yes" with no <numerusform> children.
		head + `<context><name>C</name><message numerus="yes"><source>one</source><translation></translation></message></context></TS>`,
		// numerus with a stray non-numerusform child.
		head + `<context><name>C</name><message numerus="yes"><source>one</source><translation><numerusform>un</numerusform><other>x</other></translation></message></context></TS>`,
		// Two contexts sharing a name.
		head + `<context><name>C</name><message><source>one</source><translation>un</translation></message></context><context><name>C</name><message><source>two</source><translation>deux</translation></message></context></TS>`,
		// Missing <name> on the context.
		head + `<context><message><source>one</source><translation>un</translation></message></context></TS>`,
		// The TS root never closed.
		head + `<context><name>C</name><message><source>one</source><translation>un</translation></message></context>`,
	} {
		f.Add([]byte(s))
	}
}

// FuzzReadTs asserts the Qt Linguist reader never panics and always terminates
// on arbitrary input.
func FuzzReadTs(f *testing.F) {
	fuzzTSSeed(f, "simple.ts", "plurals.ts", "bilingual.ts")
	f.Add([]byte(`<?xml version="1.0"?><TS version="2.1"><context><name>C</name><message><source>x</source><translation>y</translation></message></context></TS>`))
	f.Add([]byte(``))
	f.Add([]byte(`<TS/>`))
	seedDamagedTS(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadTS(t.Context(), data)
	})
}

// FuzzRoundTripTs asserts read → write → read is stable in content. The .ts
// writer is generative — it reconstructs the whole document from the part
// stream — so no skeleton store or original bytes are needed.
func FuzzRoundTripTs(f *testing.F) {
	fuzzTSSeed(f, "simple.ts", "plurals.ts", "bilingual.ts")
	f.Add([]byte(`<?xml version="1.0"?><TS version="2.1" language="fr" sourcelanguage="en"><context><name>C</name><message><source>Hello</source><translation>Bonjour</translation></message></context></TS>`))
	// #1606: a '>' inside an attribute value truncated the captured <TS …>
	// tag, so kapi wrote a document it could not read back.
	f.Add([]byte(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE TS []><TS version="2.1" language="a>b" sourcelanguage="en"><context><name>C</name><message><source>Hello</source><translation>Bonjour</translation></message></context></TS>`))
	seedDamagedTS(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := fuzzReadTS(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := fuzzTSTexts(parts1)

		var buf bytes.Buffer
		writer := ts.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadTS(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written .ts failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := fuzzTSTexts(parts2)
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
