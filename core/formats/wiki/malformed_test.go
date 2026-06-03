package wiki_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAllWiki drives Open+Read for the supplied reader and drains the result
// channel, returning whether any clean PartResult.Error surfaced and the number
// of translatable blocks emitted. Both Open and the channel drain run inside
// require.NotPanics so a panic fails the test with a clear message instead of
// crashing (or hanging) the run.
//
// The wiki reader is a line-based, best-effort markup tokenizer (see
// reader.go): like most markup readers it is deliberately lenient and rarely
// returns a parse error — unbalanced templates/links/tables fall through to
// plain text rather than being rejected. The contract these tests assert is
// therefore robustness, not rejection: no panic, no hang, and a sane result
// (a clean PartResult.Error OR a best-effort parse), never a crash.
func readAllWiki(t *testing.T, reader *wiki.Reader, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; any parse work happens
		// later, during Read.
		err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
			if result.Part != nil && result.Part.Type == model.PartBlock {
				blocks++
			}
		}
	})
	return foundError, blocks
}

// newDefaultReader builds the default (DokuWiki) reader, matching the okf_wiki
// bridge contract.
func newDefaultReader() *wiki.Reader { return wiki.NewReader() }

// newMediaWikiVariantReader builds a reader pinned to the MediaWiki dialect so
// the MediaWiki-specific template / link / table / infobox parsing paths are
// exercised against malformed input.
func newMediaWikiVariantReader() *wiki.Reader {
	reader := wiki.NewReader()
	reader.Config().(*wiki.Config).Variant = wiki.VariantMediaWiki
	return reader
}

// malformedInputs are inputs that stress the markup tokenizer's slicing and
// scanning paths — unterminated templates/links, unbalanced tables and inline
// markup, garbage and binary bytes, invalid UTF-8, and empty input. Each is run
// against both the DokuWiki (default) and MediaWiki readers under -race.
var malformedInputs = []struct {
	name  string
	input string
}{
	{
		name:  "empty input",
		input: "",
	},
	{
		// A lone byte-order mark with no following content.
		name:  "lone bom",
		input: "\uFEFF",
	},
	{
		// Whitespace-only input.
		name:  "whitespace only",
		input: "   \n\t  \n",
	},
	{
		// Template opened but never closed — the closing `}}` never arrives.
		name:  "unterminated template",
		input: "Lead text {{Infobox name",
	},
	{
		// Unterminated template that spans lines (the multi-line infobox
		// path) with no closing `}}`.
		name:  "unterminated multiline template",
		input: "{{Infobox\n| key = value\n| other = thing\nmore text",
	},
	{
		// A run of opening braces with nothing to balance them.
		name:  "many open braces",
		input: "{{{{{{{{",
	},
	{
		// A run of closing braces with nothing opened.
		name:  "many close braces",
		input: "}}}}}}}}",
	},
	{
		// Link opened but never closed.
		name:  "unterminated link",
		input: "See [[Target page",
	},
	{
		// Named link whose pipe alt-text segment is never closed.
		name:  "unterminated named link",
		input: "See [[Target|alt text that never ends",
	},
	{
		// A run of opening link brackets.
		name:  "many open brackets",
		input: "[[[[[[[[",
	},
	{
		// A run of closing link brackets.
		name:  "many close brackets",
		input: "]]]]]]]]",
	},
	{
		// Interleaved openers that never resolve — exercises the
		// best-token scan in tokenizeRuns / the MediaWiki extract path.
		name:  "interleaved unterminated openers",
		input: "{{[[{{[[ trailing text",
	},
	{
		// MediaWiki table opened with `{|` and never closed with `|}`.
		name:  "unterminated table",
		input: "{|\n| cell one\n| cell two\n",
	},
	{
		// Table markup with rows but a missing close, plus a header row.
		name:  "unbalanced table rows",
		input: "{| class=\"wikitable\"\n! Header\n|-\n| a || b\n|-\n| c",
	},
	{
		// Closing table marker with nothing opened.
		name:  "stray table close",
		input: "| stray cell\n|}\n",
	},
	{
		// Unbalanced inline emphasis / bold markers (DokuWiki `**`/`//`,
		// MediaWiki `'''`).
		name:  "unbalanced inline markup",
		input: "**bold start //italic '''mw bold __underline ''mw italic",
	},
	{
		// Unterminated HTML-ish / nowiki spans.
		name:  "unterminated nowiki",
		input: "text <nowiki>raw markup that never closes [[ {{",
	},
	{
		// Pure garbage / control bytes.
		name:  "garbage bytes",
		input: "@@@ %%% ^^^ \x01\x02\x03\x04\x05",
	},
	{
		// Invalid UTF-8: a lone continuation byte and a truncated
		// multi-byte sequence amid markup.
		name:  "invalid utf-8",
		input: "head \xff\xfe text [[\xc3 link]] {{\x80 tmpl}}",
	},
	{
		// NUL bytes embedded in otherwise plausible markup.
		name:  "embedded nul bytes",
		input: "a\x00b == Heading\x00 == c\x00 [[link\x00]]",
	},
	{
		// Truncated header markers (opening `==` with no close).
		name:  "unterminated heading",
		input: "== Heading with no closing equals",
	},
	{
		// Trailing dangling pipe / markers right at EOF.
		name:  "dangling markers at eof",
		input: "text [[ {{ '' // ** {| |",
	},
}

// TestReadMalformedDoesNotPanic feeds malformed, garbage, truncated, and
// invalid-UTF-8 input to both the default (DokuWiki) and MediaWiki readers and
// asserts the reader is robust: Open+Read never panic and the channel always
// drains to completion (no hang). Whether a given input surfaces a clean
// PartResult.Error or parses best-effort is an implementation detail we do not
// over-assert — the single contract is "no crash, no hang".
//
// Run with -race to surface any data race in the reader goroutine that drives
// the result channel.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()

	variants := []struct {
		name      string
		newReader func() *wiki.Reader
	}{
		{name: "dokuwiki", newReader: newDefaultReader},
		{name: "mediawiki", newReader: newMediaWikiVariantReader},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range malformedInputs {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					require.NotPanics(t, func() {
						readAllWiki(t, v.newReader(), tt.input)
					})
				})
			}
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds deeply nested unterminated markup to
// exercise the tokenizer's scanning loops without a matching close in sight. Go
// grows goroutine stacks on demand, so any recursion must complete without a
// stack-overflow panic; iterative scanning must complete without hanging.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()

	const depth = 5000
	inputs := []struct {
		name  string
		input string
	}{
		{
			name:  "deep open templates",
			input: strings.Repeat("{{", depth) + "deep" + strings.Repeat("}}", depth),
		},
		{
			name:  "deep open links",
			input: strings.Repeat("[[", depth) + "deep" + strings.Repeat("]]", depth),
		},
		{
			name:  "deep unterminated templates",
			input: strings.Repeat("{{T|", depth) + "tail",
		},
		{
			name:  "deep unterminated links",
			input: strings.Repeat("[[L|", depth) + "tail",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, newReader := range []func() *wiki.Reader{newDefaultReader, newMediaWikiVariantReader} {
				require.NotPanics(t, func() {
					readAllWiki(t, newReader(), tt.input)
				})
			}
		})
	}
}

// TestReadWellFormedSanity is a positive control: plain markup with balanced
// constructs parses cleanly and yields at least one translatable block, proving
// the malformed-input tests above are exercising a working parser rather than a
// no-op.
func TestReadWellFormedSanity(t *testing.T) {
	t.Parallel()

	foundError, blocks := readAllWiki(t, newDefaultReader(),
		"This is a normal paragraph of text.\n\nAnother paragraph here.")
	assert.False(t, foundError, "well-formed wiki text should not error")
	assert.Positive(t, blocks, "expected at least one translatable block")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// (and the nil-document case) without panicking, returning a clean error.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	require.NotPanics(t, func() {
		reader := wiki.NewReader()
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})

	require.NotPanics(t, func() {
		reader := wiki.NewReader()
		err := reader.Open(ctx, nil)
		require.Error(t, err)
	})
}
