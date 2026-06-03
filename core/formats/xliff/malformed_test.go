// okapi-filter: xliff
package xliff_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader is an io.Reader that always fails, modelling an input that cannot
// be read to completion (e.g. a truncated stream or I/O fault).
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }

// drainMalformed wraps Open+Read in require.NotPanics, drains the result
// channel, and reports whether a clean PartResult.Error surfaced plus the
// number of translatable blocks emitted. The NotPanics wrappers turn any panic
// in the reader goroutine into a clear test failure instead of crashing the run
// (or hanging). Run under -race, it also surfaces data races in the goroutine
// that drives the channel.
func drainMalformed(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := xliff.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; parse errors surface
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

// TestReadMalformedSurfacesError feeds inputs the encoding/xml tokenizer
// genuinely cannot parse and asserts the parse error surfaces cleanly on the
// result channel (PartResult.Error) rather than panicking, hanging, or being
// silently swallowed. The reader runs the decoder with Strict=false, but the
// XML tokenizer still hard-errors on truncated/unterminated tags and broken
// entity syntax — those errors are wrapped as `xliff: parsing: …` and pushed
// onto the channel (see reader.go readContent).
//
// This is the L1->L2 robustness gate: malformed input must degrade to a clean
// error, never a crash.
func TestReadMalformedSurfacesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Truncated mid-document: the <source> element and the whole
			// envelope are left open, so the tokenizer hits EOF inside an
			// unterminated tag/element and reports a clean error.
			name: "truncated unterminated source",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="t">
    <body>
      <trans-unit id="1"><source>Hello`,
		},
		{
			// Unclosed start tag: the opening <source attribute list is
			// never terminated with '>'. The tokenizer cannot finish the
			// start element before EOF.
			name: "unterminated start tag",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file><body><trans-unit id="1"><source xml:lang="en" `,
		},
		{
			// Garbage following a partial declaration: an unquoted '<' in
			// attribute position and stray markup characters the tokenizer
			// cannot resolve before EOF.
			name: "garbage markup characters",
			input: `<?xml version="1.0"?>
<xliff version="1.2"><file <body> > & < ; "`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, blocks := drainMalformed(t, tt.input)
			assert.True(t, foundError, "expected a clean parse error for malformed XLIFF input")
			// A clean error must abort before emitting a spurious block.
			assert.Zero(t, blocks, "malformed XLIFF must not emit translatable blocks")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the reader deliberately
// tolerates or that simply contain no XLIFF content. The reader sanitizes C0
// control characters and falls back to Windows-1252 for undeclared non-UTF-8
// bytes (see transcodeToUTF8), and the decoder runs with Strict=false — so some
// of these parse leniently to an empty result rather than erroring. The single
// contract asserted here is robustness: no panic, no hang, and no spurious
// translatable block. Whether an input surfaces an error or yields a clean
// empty result is an implementation detail we do not over-assert.
//
// Run under -race to surface any data race in the reader goroutine.
func TestReadLenientInputsDoNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "whitespace only",
			input: "   \n\t\r ",
		},
		{
			// Invalid/garbage bytes including NUL and C0 controls: the
			// reader's sanitizeXMLControlChars replaces these with U+FFFD
			// before decoding, so the decoder must not be fed raw control
			// bytes. No XLIFF structure → no blocks.
			name:  "raw control and high bytes",
			input: "\x00\x01\x02\x03\xff\xfe random \x7f bytes",
		},
		{
			// A well-formed but non-XLIFF XML document. The reader walks it
			// without matching any <file>/<trans-unit>, so it emits nothing
			// translatable and does not error.
			name:  "non-xliff xml document",
			input: `<?xml version="1.0" encoding="UTF-8"?><root><item>hello</item></root>`,
		},
		{
			// HTML-ish markup that is not XLIFF.
			name:  "html-ish document",
			input: `<html><body><p>hello world</p></body></html>`,
		},
		{
			// Lone byte-order mark (U+FEFF) with no following content.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// XLIFF envelope present but truncated on a balanced prefix
			// (everything that opened has closed). The decoder reaches EOF
			// cleanly; no trans-unit means no blocks.
			name: "balanced empty xliff",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2"><file><body></body></file></xliff>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var foundError bool
			var blocks int
			require.NotPanics(t, func() {
				foundError, blocks = drainMalformed(t, tt.input)
			})
			_ = foundError // tolerated either way; the contract is no panic.
			assert.Zero(t, blocks, "non-XLIFF / lenient input must not emit translatable blocks")
		})
	}
}

// TestReadRecoverableXLIFFDoesNotPanic feeds structurally-broken XLIFF that the
// non-strict (Strict=false) encoding/xml decoder deliberately recovers from
// rather than rejecting — mismatched/unclosed tags and unescaped/broken entity
// references. This matches okapi's tolerant XLIFFFilter behavior and the
// reader's own lenient parsing contract. Because the decoder recovers, these
// inputs may still emit a translatable block; the contract asserted here is
// only robustness: no panic and no hang. (The genuinely unrecoverable
// truncations are covered by TestReadMalformedSurfacesError.)
//
// Run under -race to surface any data race in the reader goroutine.
func TestReadRecoverableXLIFFDoesNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Mismatched close tag: </xliff> appears where </source> et al.
			// were expected. Strict=false disables tag-matching, so the
			// decoder recovers instead of erroring.
			name: "mismatched close tag",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file><body><trans-unit id="1"><source>Hi</xliff>`,
		},
		{
			// Unclosed elements: well-formed start tags with the matching end
			// tags missing entirely before EOF.
			name: "unclosed elements",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2"><file><body><trans-unit id="1"><source>Hi`,
		},
		{
			// Broken entity reference: '&' begins an entity but no ';'
			// terminator follows. Strict=false treats the literal '&' as
			// text rather than rejecting the malformed reference.
			name: "broken entity reference",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file><body><trans-unit id="1"><source>tom &amp jerry</source></trans-unit></body></file></xliff>`,
		},
		{
			// Bare ampersand with no entity name at all.
			name: "bare ampersand",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2"><file><body><trans-unit id="1"><source>a & b</source></trans-unit></body></file></xliff>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				// Robustness contract only: never panic, never hang. Whether
				// the lenient decoder recovers to a block or surfaces an
				// error is an implementation detail.
				drainMalformed(t, tt.input)
			})
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds deeply nested groups to exercise the
// reader's recursive/stacked element handling (preserveWSStack, groupStack, and
// parseTransUnit's depth loop). Go grows goroutine stacks on demand, so this
// must complete without a stack-overflow panic and without leaving the stream
// hanging.
//
// Run under -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 2000
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2"><file source-language="en" target-language="fr" original="t"><body>`)
	for range depth {
		b.WriteString(`<group id="g">`)
	}
	b.WriteString(`<trans-unit id="1"><source>deep</source></trans-unit>`)
	for range depth {
		b.WriteString(`</group>`)
	}
	b.WriteString(`</body></file></xliff>`)

	var foundError bool
	var blocks int
	require.NotPanics(t, func() {
		foundError, blocks = drainMalformed(t, b.String())
	})
	// The document is well-formed, so it parses without error and yields the
	// single innermost trans-unit as a translatable block.
	assert.False(t, foundError, "well-formed deeply nested XLIFF should not error")
	assert.Equal(t, 1, blocks, "expected the innermost trans-unit as the sole block")
}

// TestReadIOErrorSurfaces verifies that when the underlying reader fails
// mid-read, io.ReadAll's error is reported on the PartResult channel
// (`xliff: reading: …`) rather than swallowed or turned into a panic/hang.
func TestReadIOErrorSurfaces(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := xliff.NewReader()

	err := reader.Open(ctx, testutil.RawDocFromReader(errReader{}, "test://invalid", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
				assert.Contains(t, result.Error.Error(), "xliff:",
					"I/O error should be wrapped with the format prefix")
			}
		}
	})
	assert.True(t, foundError, "an I/O read failure must surface on the result channel")
}

// TestOpenNilDocument verifies Open rejects a nil document without panicking.
func TestOpenNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := xliff.NewReader()

	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, nil)
	})
	require.Error(t, err)
}

// TestOpenNilReader verifies Open rejects a document whose Reader is nil
// without panicking.
func TestOpenNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := xliff.NewReader()

	doc := &model.RawDocument{URI: "test://input", SourceLocale: model.LocaleEnglish}
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, doc)
	})
	require.Error(t, err)
}
