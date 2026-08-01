package markdown_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadMarkdown drives the Markdown reader over input without ever calling
// t.Fatal, so it is safe on arbitrary fuzz input.
func fuzzReadMarkdown(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := markdown.NewReader()
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

func markdownSourceTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func markdownSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// seedDamagedMarkdown adds documents that are structurally damaged but still
// PARSEABLE. See the seedDamagedPO comment in core/formats/po/fuzz_test.go for
// why this corpus shape matters (#1588).
//
// Markdown has no notion of "malformed" — every byte sequence is a valid
// document — so the read target can only assert crash-freedom. The interesting
// region is instead unterminated constructs, because Markdown's block
// containers (fences, HTML blocks, front matter) run to a terminator and
// swallow everything up to it when that terminator is missing. That is the
// direct analogue of #1588, where a damaged closing tag absorbed the rest of
// the file.
func seedDamagedMarkdown(f *testing.F) {
	f.Helper()
	for _, s := range []string{
		// Unterminated fenced code block: everything after it may be absorbed
		// into the fence and never extracted.
		"# Title\n\n```go\ncode\n\nA paragraph that should still be extracted.\n",
		// Fence opened with three backticks, closed with four.
		"```\ncode\n````\n\nAfter.\n",
		// Unterminated front matter — the whole document can become front matter.
		"---\ntitle: Meta\n\n# Heading\n\nBody text.\n",
		// Front matter closed with the wrong marker.
		"---\ntitle: Meta\n...\n\n# Heading\n",
		// Unterminated raw HTML block.
		"<div>\n\n# Heading inside an unclosed div\n\nParagraph.\n",
		// Unterminated HTML comment: the rest of the file is comment body.
		"# Before\n\n<!-- comment never closed\n\n# After\n",
		// Emphasis opened and never closed.
		"A paragraph with **bold that never closes and more text.\n",
		// Link syntax with an unclosed bracket.
		"See [the docs](https://example.com for details.\n",
		// A list whose marker changes mid-list — two lists, or one?
		"- first\n- second\n* third\n* fourth\n",
		// Setext heading underline directly against a paragraph.
		"Paragraph text\n===\nMore text\n",
		// Table with a mismatched column count in the body row.
		"| a | b |\n| - | - |\n| 1 |\n\nAfter the table.\n",
		// Nested blockquote that de-indents mid-way.
		"> quoted\n>> deeper\nlazy continuation\n\nAfter.\n",
	} {
		f.Add([]byte(s))
	}
}

// FuzzReadMarkdown asserts the Markdown reader never panics and always
// terminates on arbitrary input. Markdown is lenient by design — there is no
// "malformed" rejection — so the invariant here is purely crash-freedom and
// bounded resources.
func FuzzReadMarkdown(f *testing.F) {
	markdownSeed(f, "simple.md")
	f.Add([]byte("# Hello\n\nWorld.\n"))
	f.Add([]byte(""))
	f.Add([]byte("- a\n- b\n"))
	f.Add([]byte("Text with `code` and [link](https://example.com).\n"))
	seedDamagedMarkdown(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadMarkdown(t.Context(), data)
	})
}

// tripMarkdown runs one read → write pass through the writer's rebuild path,
// reporting the emitted bytes and the source texts the read produced. ok is
// false when the input does not parse cleanly or the writer declines it — those
// carry only the no-panic contract.
func tripMarkdown(ctx context.Context, data []byte) (out []byte, texts []string, ok bool) {
	parts, hadErr := fuzzReadMarkdown(ctx, data)
	if hadErr || len(testutil.FilterBlocks(parts)) == 0 {
		return nil, nil, false
	}
	var buf bytes.Buffer
	writer := markdown.NewWriter()
	if err := writer.SetOutputWriter(&buf); err != nil {
		return nil, nil, false
	}
	writer.SetLocale(model.LocaleEnglish)
	if err := writer.Write(ctx, testutil.PartsToChannel(parts)); err != nil {
		return nil, nil, false
	}
	writer.Close()
	return buf.Bytes(), markdownSourceTexts(parts), true
}

// FuzzRoundTripMarkdown asserts the rebuild write path converges: no block is
// lost on the way out, and a second pass over the writer's own output changes
// neither the bytes nor the extracted text.
//
// Idempotence rather than "pass1 == pass2" is deliberate. Without a skeleton
// store the writer rebuilds from the event stream and drops constructs it has
// no Markdown spelling for — inline HTML above all — which is advertised
// lossiness in markup. But dropping one at a block edge moves the whitespace
// beside it to a position Markdown cannot represent, so the FIRST pass does
// legitimately change the text there. Asserting it does not is asserting
// something this path cannot deliver; what it must deliver, and what #1603 was,
// is that the change happens once and then stops. A text that keeps changing
// hashes to a new content key every run (AD-036) and silently stops matching
// content memory.
//
// The skeleton write path preserves the markup and is checked separately.
//
// Two known open failures remain, both older than #1603 and both confirmed to
// reproduce on a writer with the #1603 fix reverted. None of the seeds below
// trip them; they are recorded here so neither is mistaken for a new one.
//
//   - #1632, rediscovered in about a second with "#<A>". Same family as #1603 —
//     a dropped construct whose loss does not stay confined to markup — but the
//     text left behind begins with a bare block marker, so the emitted
//     paragraph re-reads as an empty heading. It needs escaping, not the
//     boundary trim #1603 called for.
//   - #1633, rediscovered in seconds with "> a\n> b". The rebuild path ignores
//     BlockPropLinePrefix, so a multi-line blockquote loses its prefix and one
//     block re-reads as several. That one violates the block-count half of the
//     contract rather than the idempotence half.
func FuzzRoundTripMarkdown(f *testing.F) {
	markdownSeed(f, "simple.md")
	f.Add([]byte("# Hello\n\nA paragraph.\n"))
	f.Add([]byte("## Heading\n\n- one\n- two\n"))
	// #1603: a construct dropped at a block edge promoted the whitespace
	// beside it to a position markdown strips on re-read.
	f.Add([]byte("<A> 0"))
	f.Add([]byte("trail <A>"))
	f.Add([]byte("# <A> heading"))
	f.Add([]byte("| <A> a | b |\n| --- | --- |\n| c | d |\n"))
	seedDamagedMarkdown(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		out1, texts1, ok := tripMarkdown(ctx, data)
		if !ok {
			return // only the no-panic contract applies to a non-clean parse
		}

		out2, texts2, ok2 := tripMarkdown(ctx, out1)
		if !ok2 {
			t.Fatalf("re-reading written Markdown failed; round-trip is not stable\ninput:  %q\noutput: %q", data, out1)
		}
		if len(texts1) != len(texts2) {
			t.Fatalf("round-trip changed block count: %d -> %d\ninput:  %q\noutput: %q\npass1:  %q\npass2:  %q",
				len(texts1), len(texts2), data, out1, texts1, texts2)
		}

		_, texts3, ok3 := tripMarkdown(ctx, out2)
		if !ok3 {
			t.Fatalf("re-reading the writer's own output failed\nout1: %q\nout2: %q", out1, out2)
		}
		if !bytes.Equal(out1, out2) {
			t.Fatalf("rebuild output is not idempotent\ninput: %q\nout1:  %q\nout2:  %q", data, out1, out2)
		}
		if len(texts2) != len(texts3) {
			t.Fatalf("block count drifted on the third pass: %d -> %d\ninput: %q", len(texts2), len(texts3), data)
		}
		for i := range texts2 {
			if texts2[i] != texts3[i] {
				t.Fatalf("source text drifted after the first pass\npass2: %q\npass3: %q\ninput: %q\nout2: %q",
					texts2, texts3, data, out2)
			}
		}
	})
}
