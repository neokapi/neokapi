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

// FuzzRoundTripMarkdown asserts read → write → read is stable in content: a
// document that parses cleanly, written back and re-read, yields the same
// source texts. Without a skeleton store the writer rebuilds from the event
// stream, so formatting legitimately changes; the extracted text must not.
//
// Known open failure: fuzzing this target rediscovers #1603 within seconds. A
// paragraph that begins with an inline HTML tag ("<A> 0") loses the tag in the
// rebuild path, which promotes the space after it to leading whitespace, which
// markdown then strips — so the text drifts from " 0" to "0". The skeleton
// write path is correct for the same input. None of the seeds below trip it;
// it is recorded here so the failure is not mistaken for a new one.
func FuzzRoundTripMarkdown(f *testing.F) {
	markdownSeed(f, "simple.md")
	f.Add([]byte("# Hello\n\nA paragraph.\n"))
	f.Add([]byte("## Heading\n\n- one\n- two\n"))
	seedDamagedMarkdown(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := fuzzReadMarkdown(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := markdownSourceTexts(parts1)

		var buf bytes.Buffer
		writer := markdown.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		writer.SetLocale(model.LocaleEnglish)
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadMarkdown(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written Markdown failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := markdownSourceTexts(parts2)
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
