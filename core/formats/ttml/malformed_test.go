package ttml_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains a TTML reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It is
// wrapped in require.NotPanics by callers so a panic fails the test with a clear
// message instead of crashing the run. The withSkeleton flag selects the
// skeleton path (findPTextRanges + byte slicing) versus the simple path; both
// must survive malformed input.
func readAll(t *testing.T, input string, withSkeleton bool) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := ttml.NewReader()

	if withSkeleton {
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer store.Close()
		reader.SetSkeletonStore(store)
	}

	require.NotPanics(t, func() {
		// Open only validates the document/reader; parse handling happens
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

// malformedInputs is the shared corpus of broken/garbage/truncated TTML fed to
// both the simple and skeleton read paths.
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
		// XML prolog only, no document element at all.
		name:  "prolog only",
		input: `<?xml version="1.0" encoding="UTF-8"?>`,
	},
	{
		// Truncated mid-tag: the opening <tt is never closed.
		name:  "truncated mid open tag",
		input: `<?xml version="1.0"?><tt xmlns="http://www.w3.org/ns/ttml`,
	},
	{
		// Truncated inside a <p>: the caption text is opened but the document
		// ends before </p>, </body>, or </tt>. parseCaptions must not run off
		// the end of the buffer.
		name: "truncated inside p",
		input: `<?xml version="1.0"?><tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="00:00:01.000" end="00:00:04.000">Hello wor`,
	},
	{
		// <body> opened but never closed: bodySlice finds no </body> and falls
		// back, and the decoder hits EOF mid-stream.
		name:  "unclosed body",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p>Caption text`,
	},
	{
		// Mismatched namespace prefixes on a tag pair — a real XML
		// well-formedness violation Go's decoder rejects (cf. okapi example1).
		name: "mismatched namespace prefix",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><head><okp:foo>x</lilt:foo></head>` +
			`<body><div><p begin="0s" end="1s">Hi</p></div></body></tt>`,
	},
	{
		// Plain mismatched/unclosed tags: <p> closed by </q>.
		name: "mismatched tag names",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="0s">Caption</q></div></body></tt>`,
	},
	{
		// No <tt>/<body>/<p> structure at all — just stray text.
		name:  "no ttml structure",
		input: `just some plain text with no markup at all`,
	},
	{
		// <tt> with content but no <body> and no <p>.
		name:  "missing body and p",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><head><styling/></head></tt>`,
	},
	{
		// Self-closing <body/> — bodySlice returns nil (no </body>), falling
		// back to whole-document scan; there is nothing to extract.
		name:  "self closing body",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body/></tt>`,
	},
	{
		// Broken/undefined entity reference inside caption text.
		name: "broken entity",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="0s" end="1s">A &notAnEntity; B</p></div></body></tt>`,
	},
	{
		// Dangling ampersand at EOF inside caption text.
		name: "dangling ampersand",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="0s" end="1s">trailing &`,
	},
	{
		// Garbage / invalid bytes where markup is expected.
		name:  "garbage bytes",
		input: "@@@ \x00\x01\x02 %%% ^^^ </></>",
	},
	{
		// Invalid UTF-8 continuation bytes embedded in caption text.
		name: "invalid utf8 in caption",
		input: "<tt xmlns=\"http://www.w3.org/ns/ttml\"><body><div>" +
			"<p begin=\"0s\" end=\"1s\">bad\xff\xfebytes</p></div></body></tt>",
	},
	{
		// Bad timestamp attributes: non-numeric begin/end/dur. The reader
		// stores these verbatim as properties and never parses them, so this
		// must extract cleanly without panicking on the junk values.
		name: "bad timestamp attrs",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="not-a-time" end="????" dur="-1z">Caption</p></div></body></tt>`,
	},
	{
		// Empty timestamp attributes.
		name: "empty timestamp attrs",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="" end="" dur="">Caption</p></div></body></tt>`,
	},
	{
		// Stray closing tags with nothing opened.
		name:  "stray closing tags",
		input: `</p></body></tt>`,
	},
	{
		// <body>/</body> present but the <p> closing tag is missing, so
		// findCloseTag must return -1 cleanly rather than slicing past EOF.
		name: "body closed but p unclosed",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="0s" end="1s">no closing p tag</div></body></tt>`,
	},
	{
		// Nested <p> elements — exercises the depth counter in parseCaption
		// and findPTextRanges without a clean single close.
		name: "nested p elements",
		input: `<tt xmlns="http://www.w3.org/ns/ttml"><body><div>` +
			`<p begin="0s">outer <p>inner</p> still outer</p></div></body></tt>`,
	},
	{
		// Only an opening <p> with no attributes and no close, after </body>
		// search would fail — stresses bodySlice + decoder EOF together.
		name:  "lone open p",
		input: `<body><p>`,
	},
}

// TestReadMalformedDoesNotPanic feeds truncated, garbage, mismatched, and
// otherwise broken TTML through the simple (non-skeleton) read path. The TTML
// reader is robust by design: parseCaptions breaks its token loop on any XML
// decode error and yields fewer/no captions rather than surfacing a
// PartResult.Error (parse errors are graceful, not fatal — see reader.go's
// parseCaptions doc comment). The load-bearing contract is therefore
// robustness: no panic, no hang, never a crash on malformed bytes.
//
// Run with -race to surface any data race in the reader goroutine that drives
// the result channel.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAll(t, tt.input, false)
			})
		})
	}
}

// TestReadMalformedSkeletonDoesNotPanic runs the same corpus through the
// skeleton read path (findPTextRanges + findCloseTag byte slicing +
// SkeletonStore writes). This path does explicit byte-offset arithmetic on the
// raw document, so it is the one most at risk of an out-of-bounds slice on
// truncated or mismatched input; it must survive the same corpus without
// panicking.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadMalformedSkeletonDoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAll(t, tt.input, true)
			})
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds deeply nested <p>/<span> elements to
// exercise the recursive depth counters in parseCaption and findPTextRanges. Go
// grows goroutine stacks on demand, so this must complete without a
// stack-overflow panic on either read path.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	input := `<tt xmlns="http://www.w3.org/ns/ttml"><body><div><p begin="0s">` +
		strings.Repeat("<span>", depth) + "deep" + strings.Repeat("</span>", depth) +
		`</p></div></body></tt>`

	require.NotPanics(t, func() {
		readAll(t, input, false)
	})
	require.NotPanics(t, func() {
		readAll(t, input, true)
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking, returning a clean error.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := ttml.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}

// (The nil-document case is covered by TestReadNilDocument in reader_test.go.)

// TestReadEmptyEmitsNoBlocks confirms empty input is handled as graceful empty
// (no blocks, no error) rather than an error or panic — the documented
// graceful-empty contract for content-free input.
func TestReadEmptyEmitsNoBlocks(t *testing.T) {
	t.Parallel()
	foundError, blocks := readAll(t, "", false)
	assert.False(t, foundError, "empty TTML should not surface a parse error")
	assert.Zero(t, blocks, "empty TTML should yield no translatable blocks")
}
