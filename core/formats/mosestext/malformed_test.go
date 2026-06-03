package mosestext_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/mosestext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds a range of broken, truncated, garbage, and
// binary inputs and asserts the reader neither panics nor wedges. Moses Text is
// a line-oriented plain-text format with a tolerant pseudo-XLIFF inline-markup
// surface (`<g>`/`<x>`/`<bx>`/`<ex>`/`<lb/>`, XML entities, `<mrk mtype="seg">`
// segment annotations). None of these carry a structural grammar that can be
// "invalid" — unbalanced tags, malformed attributes, dangling segment markers,
// and raw binary bytes are all treated as opaque text and pass through
// gracefully rather than surfacing a PartResult.Error. The contract this test
// pins is therefore "never panic, always drain the channel to completion": if a
// future change makes any of these inputs panic (e.g. an out-of-range slice in
// parseCodes) or deadlock the Read goroutine, this test fails loudly.
//
// Run under the race detector (`go test -race`) to catch any data race between
// the Read goroutine and the channel consumer on malformed input.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// A truncated line with no terminator at EOF: the whole tail is
			// one block. scanRawLines must not synthesize a phantom line.
			name:  "truncated line no terminator",
			input: "Hello, this line is cut off at EOF",
		},
		{
			// An opening `<g id="1">` with no matching `</g>`: the pairing
			// stack is left with a dangling id. parseCodes must emit the open
			// run and not index past the end of the text.
			name:  "unbalanced opening g tag",
			input: `Start <g id="1">middle no close`,
		},
		{
			// A lone `</g>` with no opener: the pairing stack is empty, so the
			// close-branch must tolerate popping from an empty stack.
			name:  "lone closing g tag",
			input: `text </g> trailing`,
		},
		{
			// Closing tags far outnumbering openers: repeatedly pops an empty
			// stack.
			name:  "many closing g tags no openers",
			input: `</g></g></g></g>`,
		},
		{
			// A `<g>` with no id attribute does not match the OPENCLOSE regex
			// (which requires id=) and must fall through to literal text.
			name:  "g tag missing id",
			input: `before <g>no id here</g> after`,
		},
		{
			// A malformed isolated tag (`<x` with no id and no self-close):
			// must not match ISOLATED and stay literal.
			name:  "malformed isolated tag",
			input: `value with <x broken and <bx also broken`,
		},
		{
			// A self-closing `<lb/>` at the very end and a dangling `&` entity
			// that never completes.
			name:  "dangling entity and trailing lb",
			input: `tail &amp incomplete &#13 and <lb/>`,
		},
		{
			// `<mrk mtype="seg">` opened but never closed: groupEntries must
			// consume to EOF without looping forever and emit one entry.
			name:  "unclosed mrk segment at eof",
			input: "<mrk mtype=\"seg\">segment body line one\nline two never closes",
		},
		{
			// A lone `</mrk>` close with no opener: treated as ordinary text.
			name:  "lone mrk close no open",
			input: "</mrk> orphan close marker",
		},
		{
			// Empty `<mrk mtype="seg"></mrk>` on one line: an empty segment
			// body.
			name:  "empty mrk segment",
			input: `<mrk mtype="seg"></mrk>`,
		},
		{
			// Nested / interleaved garbage of every markup kind at once.
			name:  "interleaved markup garbage",
			input: `<g id="1"><bx id="2"/></g><ex id="3"/><x id=/><lb/>&lt;&amp;&#x;`,
		},
		{
			// Raw garbage / non-UTF-8 binary bytes. decodeInlineText slices on
			// byte offsets from the regex matchers, so invalid runes must pass
			// through without panicking.
			name:  "garbage binary bytes",
			input: "bin\xff\xfe\x00\x80\x01<g id=\"\xfe\">\x7f</g>",
		},
		{
			// NUL bytes interleaved with markup and a trailing `<` that opens
			// the inline-markup path but matches nothing.
			name:  "nul bytes with markup",
			input: "a\x00b<lb/>c\x00<g id=\"\x00\">\x00",
		},
		{
			// Only a bare `<` — triggers the markup path (hasInlineMarkup) but
			// matches no code regex; must stay literal.
			name:  "bare angle bracket",
			input: "<",
		},
		{
			// Only a bare `&` — triggers the markup path via the entity check
			// but decodes to nothing.
			name:  "bare ampersand",
			input: "&",
		},
		{
			// Empty input — no lines at all.
			name:  "empty input",
			input: "",
		},
		{
			// Whitespace / control-only input.
			name:  "control chars only",
			input: "\x00\x01\x02\x03\t\v\f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := mosestext.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			// Drain the full channel. The reader is lenient by design, so we
			// don't require an error — only that nothing panics and the
			// channel closes (the loop terminates). A panic in the Read
			// goroutine would propagate here; a deadlock would trip the
			// per-test timeout. If the reader ever does surface a
			// PartResult.Error for malformed input, that is acceptable too — we
			// just must not panic or silently swallow it.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					_ = result.Error
				}
			})
		})
	}
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil (an
// otherwise-populated RawDocument) without panicking. This is distinct from a
// nil document (covered by TestReadNilDocument in reader_test.go): the reader
// must guard the embedded Reader too, or scanRawLines would dereference nil.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := mosestext.NewReader()
	doc := &model.RawDocument{
		URI:          "test://nil-reader",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nil,
	}
	require.NotPanics(t, func() {
		err := reader.Open(ctx, doc)
		require.Error(t, err)
	})
}
