package markdown_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
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

// markdownContentKeys returns the sorted multiset of block content-identity
// keys — model.ComputeContentHash(SourceText), the sha256-over-trimmed-source
// key that content memory and the sync diff engine key on. Comparing keys (not
// raw bytes) is what makes the round-trip contract match the guarantee the
// product actually depends on: two outputs that differ only in formatting the
// key folds out (edge/trailing whitespace, indented-vs-fenced code) are the
// same content and must not read as a regression.
//
// Whitespace-only blocks are skipped: their trimmed source text is empty, so
// they carry no content identity (nothing keys on them, nothing to match in
// memory). A block that is only a form feed disappearing across a rebuild is not
// content loss, so it must not read as one.
func markdownContentKeys(parts []*model.Part) []string {
	var keys []string
	for _, b := range testutil.FilterBlocks(parts) {
		if strings.TrimSpace(b.SourceText()) == "" {
			continue
		}
		keys = append(keys, model.ComputeContentHash(b.SourceText()))
	}
	sort.Strings(keys)
	return keys
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
// reporting the emitted bytes and the content-identity keys the read produced.
// ok is false when the input does not parse cleanly or the writer declines it —
// those carry only the no-panic contract.
func tripMarkdown(ctx context.Context, data []byte) (out []byte, keys []string, ok bool) {
	parts, hadErr := fuzzReadMarkdown(ctx, data)
	keys = markdownContentKeys(parts)
	if hadErr || len(keys) == 0 {
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
	return buf.Bytes(), keys, true
}

// FuzzRoundTripMarkdown asserts the rebuild write path is a fixed point of
// BLOCK CONTENT IDENTITY: reading its output and rebuilding again does not gain,
// lose, or change any block's content key.
//
// The contract is content identity, not bytes, on purpose. Without a skeleton
// store the writer rebuilds from the event stream and is deliberately lossy for
// *formatting* — it drops constructs it has no Markdown spelling for (inline
// HTML above all), re-renders indented code as fenced, and trims boundary
// whitespace. The guarantee the product actually depends on is narrower: a
// block's content key — model.ComputeContentHash(SourceText), the
// sha256-over-trimmed-source key content memory and the sync diff engine use
// (AD-036) — must survive a round-trip, or memory matches silently stop hitting.
// Byte idempotence is strictly stronger than that and flags cosmetic churn the
// key folds out (edge/trailing whitespace, indented-vs-fenced code, e.g. the
// former #1654) as if it were corruption. Comparing keys catches every real
// regression — a block that changes kind and text (#1632), splits (#1633),
// re-reads as a table (#1651), drifts interior whitespace (#1652), or vanishes
// (#1652) — while treating a formatting-only difference as the accepted
// representation change it is.
//
// The skeleton write path preserves the markup byte-for-byte and is checked
// separately; that is where byte fidelity is asserted.
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
	// #1632: dropping the inline HTML leaves a bare "#", which re-parses as an
	// empty heading unless the rebuild path escapes the leading marker.
	f.Add([]byte("#<A>"))
	// #1633: a multi-line blockquote whose "> " line prefix the rebuild path
	// must re-establish, or the single block splits into loose paragraphs.
	f.Add([]byte("> a\n> b"))
	// #1651: a paragraph whose continuation line forms a GFM table delimiter row
	// ("|0\n-|") re-reads as a table unless the rebuild path escapes the bar; an
	// all-empty GFM header row loses its header flag and accumulates one blank
	// row per pass unless the reader tags the header row group.
	f.Add([]byte("|0\n-|"))
	f.Add([]byte("<A> |0\n-|"))
	f.Add([]byte("|  |\n| --- |\n|  |\n| 00 |"))
	// #1652: a hard break (2+ trailing spaces) must be captured the same however
	// many spaces the source used; dropping inline HTML must not let the residue
	// reform a tag that re-reads as nothing.
	f.Add([]byte("0     \n0"))
	f.Add([]byte("<<A>A>"))
	// #1654: an indented code block re-renders as fenced with a trailing newline.
	// Under the content-identity contract this is an accepted representation
	// change (the key is trim-stable); the seed guards that it stays that way.
	f.Add([]byte("    0"))
	// #1657: a paragraph starting with 3+ backticks but with another backtick
	// later on the line is not a fence, so it must be left literal; escaping only
	// the first backtick used to invent a code span and drift the content.
	f.Add([]byte("```0`0``"))
	// #1659: a multi-line setext heading is rebuilt as single-line ATX, so its
	// embedded newline split it into a heading plus a loose paragraph unless
	// the rebuild path folds the heading onto one line. The first input is
	// also committed under testdata/fuzz.
	f.Add([]byte("0\n0\n="))
	f.Add([]byte("> 0\n> 0\n> ==="))
	// #1661: a hard break's spelling rides a placeholder, so whether the source
	// used spaces or a backslash the rebuild path emits a soft break and the
	// re-read keys are the same; a paragraph mixing hard and soft breaks keeps
	// its blockquote marker on every line.
	f.Add([]byte("a\\\nb"))
	f.Add([]byte("> a  \n> b\n> c"))
	// #2434: a blockquote with a lazy continuation line (CommonMark 5.1) was
	// judged by its first continuation line alone, so the rebuild path wrote
	// it back without its opening marker and it split into a paragraph plus a
	// quote. The fuzz reproducer is also committed under testdata/fuzz.
	f.Add([]byte("> a\nb\n> c"))
	// #2431: a soft break keeps the trailing whitespace or carriage return
	// before its newline, so the rebuilt block carries those bytes too.
	f.Add([]byte("a\t\nb"))
	f.Add([]byte("a\r\nb\r\n"))
	f.Add([]byte("> a \r\n> b"))
	seedDamagedMarkdown(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		out1, keys1, ok := tripMarkdown(ctx, data)
		if !ok {
			return // only the no-panic contract applies to a non-clean parse
		}

		// Reading the rebuilt document must still yield blocks — a rebuild whose
		// output re-reads as nothing has lost content outright.
		out2, keys2, ok2 := tripMarkdown(ctx, out1)
		if !ok2 {
			t.Fatalf("re-reading the rebuilt Markdown lost every block\ninput:  %q\noutput: %q", data, out1)
		}
		// No block may be gained or lost crossing into the rebuild: the lossiness
		// is intra-block formatting, never a block-count change (a blockquote that
		// splits, a table row that accumulates, a block that vanishes). The first
		// pass may re-REPRESENT a block — dropping inline HTML, escaping a residual
		// marker — which can change its source text (and so its key); what it must
		// not do is change how many blocks there are.
		if len(keys1) != len(keys2) {
			t.Fatalf("round-trip changed block count: %d -> %d\ninput: %q\nout1:  %q", len(keys1), len(keys2), data, out1)
		}

		// From the first rebuild on, content identity is a fixed point: the key
		// multiset does not drift. Compared by ComputeContentHash, so a
		// formatting-only difference the key folds out (edge/trailing whitespace,
		// indented-vs-fenced code) is not a failure — only a real identity change
		// (interior text drift, an accumulating row) is.
		_, keys3, ok3 := tripMarkdown(ctx, out2)
		if !ok3 {
			t.Fatalf("re-reading the writer's own output lost every block\nout1: %q\nout2: %q", out1, out2)
		}
		if !slices.Equal(keys2, keys3) {
			t.Fatalf("block content identity drifted after the first pass\ninput: %q\nout2: %q\nkeys2: %v\nkeys3: %v",
				data, out2, keys2, keys3)
		}
	})
}
