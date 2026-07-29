package tmx_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadTMX drives the TMX reader over input without ever calling t.Fatal, so
// it is safe on arbitrary fuzz input.
func fuzzReadTMX(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := tmx.NewReader()
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

func fuzzTMXTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func fuzzTMXSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// seedDamagedTMX adds documents that are structurally damaged but still
// PARSEABLE. See the seedDamagedXML comment in core/formats/xml/fuzz_test.go
// for why this corpus shape matters (#1588).
//
// TMX matters here beyond its own use: it is an interchange format, so the
// documents it parses arrive from other tools and from outside the project.
// The reader also runs its decoder with Strict = false to tolerate DTDs and
// HTML entities, which widens exactly the lenient-tokenizer surface #1588 was
// about.
func seedDamagedTMX(f *testing.F) {
	f.Helper()
	const head = `<?xml version="1.0" encoding="UTF-8"?><tmx version="1.4"><header creationtool="t" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body>`
	for _, s := range []string{
		// The #1588 shape: the '<' of </tu> corrupted, so following units may
		// be absorbed into the first.
		head + `<tu><tuv xml:lang="en"><seg>one</seg></tuv></tu x/tu><tu><tuv xml:lang="en"><seg>two</seg></tuv></tu></body></tmx>`,
		// </tu> missing between two units.
		head + `<tu><tuv xml:lang="en"><seg>one</seg></tuv><tu><tuv xml:lang="en"><seg>two</seg></tuv></tu></body></tmx>`,
		// </seg> missing — the rest of the unit becomes segment text.
		head + `<tu><tuv xml:lang="en"><seg>one</tuv></tu><tu><tuv xml:lang="en"><seg>two</seg></tuv></tu></body></tmx>`,
		// A tu with no tuv at all.
		head + `<tu/></body></tmx>`,
		// A tuv with no xml:lang.
		head + `<tu><tuv><seg>one</seg></tuv></tu></body></tmx>`,
		// Two tuvs declaring the same language.
		head + `<tu><tuv xml:lang="en"><seg>one</seg></tuv><tuv xml:lang="en"><seg>two</seg></tuv></tu></body></tmx>`,
		// body never closed.
		head + `<tu><tuv xml:lang="en"><seg>one</seg></tuv></tu></tmx>`,
		// An undeclared entity — the non-strict decoder tolerates these.
		head + `<tu><tuv xml:lang="en"><seg>one &nbsp; two</seg></tuv></tu></body></tmx>`,
		// An internal DTD subset ahead of the root.
		`<?xml version="1.0"?><!DOCTYPE tmx [<!ENTITY e "x">]><tmx version="1.4"><header creationtool="t" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body><tu><tuv xml:lang="en"><seg>&e;</seg></tuv></tu></body></tmx>`,
		// Inline placeholder never closed.
		head + `<tu><tuv xml:lang="en"><seg>one <ph>{0}</seg></tuv></tu></body></tmx>`,
		// Missing header entirely.
		`<?xml version="1.0"?><tmx version="1.4"><body><tu><tuv xml:lang="en"><seg>one</seg></tuv></tu></body></tmx>`,
	} {
		f.Add([]byte(s))
	}
}

// FuzzReadTmx asserts the TMX reader never panics and always terminates on
// arbitrary input.
func FuzzReadTmx(f *testing.F) {
	fuzzTMXSeed(f, "simple.tmx")
	f.Add([]byte(`<?xml version="1.0"?><tmx version="1.4"><body><tu><tuv xml:lang="en"><seg>x</seg></tuv></tu></body></tmx>`))
	f.Add([]byte(``))
	f.Add([]byte(`<tmx/>`))
	seedDamagedTMX(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadTMX(t.Context(), data)
	})
}

// FuzzRoundTripTmx asserts read → write → read is stable in content. The TMX
// writer is generative — it rebuilds the document from the collected blocks and
// header properties — so no skeleton store or original bytes are needed.
func FuzzRoundTripTmx(f *testing.F) {
	fuzzTMXSeed(f, "simple.tmx")
	f.Add([]byte(`<?xml version="1.0"?><tmx version="1.4"><header creationtool="t" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/><body><tu><tuv xml:lang="en"><seg>Hello</seg></tuv></tu></body></tmx>`))
	seedDamagedTMX(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := fuzzReadTMX(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := fuzzTMXTexts(parts1)

		var buf bytes.Buffer
		writer := tmx.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		writer.SetLocale(model.LocaleEnglish)
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadTMX(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written TMX failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := fuzzTMXTexts(parts2)
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
