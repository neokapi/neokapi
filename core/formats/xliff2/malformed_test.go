package xliff2_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It
// wraps Open+Read in require.NotPanics so a panic fails the test with a clear
// message instead of crashing (or hanging) the run.
//
// The default XLIFF 2.x reader uses the etree DOM parser (no skeleton store),
// which fully validates the XML before walking it: malformed input surfaces as
// a PartResult.Error from the parse step, never a panic.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()

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

// TestReadMalformedSurfacesError feeds inputs the XML parser genuinely cannot
// accept and asserts the parse error surfaces cleanly on the result channel
// (PartResult.Error) rather than panicking, hanging, or being silently
// swallowed. This is the L1->L2 robustness gate for the format.
//
// Run with -race to catch any data race in the reader goroutine that drives the
// channel.
func TestReadMalformedSurfacesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Truncated document: the opening <xliff> tag is never closed,
			// so the parser reaches EOF mid-element.
			name:  "truncated unterminated xliff",
			input: `<?xml version="1.0"?><xliff version="2.0" srcLang="en"`,
		},
		{
			// Truncated mid-content: a <source> is opened but the document
			// ends before any close tag.
			name: "truncated mid source",
			input: `<?xml version="1.0"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment><source>Hello`,
		},
		{
			// Mismatched close tag: <source> is closed with </target>.
			name: "mismatched close tag",
			input: `<?xml version="1.0"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment><source>Hi</target></segment></unit></file>
</xliff>`,
		},
		{
			// Unclosed nested element: <unit> is never closed.
			name: "unclosed unit",
			input: `<?xml version="1.0"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment><source>Hi</source></segment>
  </file></xliff>`,
		},
		{
			// Broken/undefined entity reference: &nbsp; is not predefined in
			// XML and there is no DTD declaring it.
			name: "undefined entity",
			input: `<?xml version="1.0"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment><source>A&nbsp;B</source></segment></unit></file>
</xliff>`,
		},
		{
			// Unterminated entity: the ampersand opens an entity reference
			// that is never closed with a semicolon before the tag boundary.
			name: "unterminated entity",
			input: `<?xml version="1.0"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment><source>A & B</source></segment></unit></file>
</xliff>`,
		},
		{
			// Invalid raw bytes: a NUL and other control bytes are illegal in
			// XML 1.0 character content.
			name:  "invalid control bytes",
			input: "<?xml version=\"1.0\"?>\n<xliff version=\"2.0\" srcLang=\"en\"><file id=\"f1\"><unit id=\"u1\"><segment><source>\x00\x01\x02</source></segment></unit></file></xliff>",
		},
		{
			// Garbage that is not XML at all: the first non-whitespace byte is
			// not '<', so the parser rejects it immediately.
			name:  "garbage bytes",
			input: "@@@ %%% ^^^ not xml at all",
		},
		{
			// A well-formed XML document that is not XLIFF: parses fine but
			// has no <xliff> root, so the reader emits a clean
			// "no <xliff> root element found" error.
			name: "non-xliff xml document",
			input: `<?xml version="1.0"?>
<html><body><p>Hello</p></body></html>`,
		},
		{
			// A well-formed but empty non-XLIFF root.
			name:  "non-xliff single element",
			input: `<root/>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed/non-XLIFF input")
		})
	}
}

// TestReadRobustInputsDoNotPanic feeds inputs that the parser may either reject
// or tolerate; the single contract here is robustness — no panic, no hang, no
// race — regardless of whether an error surfaces. Whether a given degenerate
// input parses to zero blocks or errors is an implementation detail of the
// underlying XML parser that we deliberately do not over-assert.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadRobustInputsDoNotPanic(t *testing.T) {
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
			input: "   \n\t  ",
		},
		{
			// A lone byte-order mark with no following content.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// XML declaration only, no document element.
			name:  "xml declaration only",
			input: `<?xml version="1.0" encoding="UTF-8"?>`,
		},
		{
			// A bare '<' with nothing after it.
			name:  "lone angle bracket",
			input: "<",
		},
		{
			// Valid XLIFF root but otherwise empty: no files, no units.
			name:  "empty xliff root",
			input: `<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en"></xliff>`,
		},
		{
			// Valid file/unit but a <segment> with no <source>: the reader
			// tolerates this (spec violation) and skips the segment.
			name: "segment without source",
			input: `<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en">
  <file id="f1"><unit id="u1"><segment id="s1"></segment></unit></file></xliff>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAll(t, tt.input)
			})
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested group structure to
// exercise the recursive emitGroup path. Go grows goroutine stacks on demand,
// so this must complete without a stack-overflow panic.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 2000
	var b strings.Builder
	b.WriteString(`<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en"><file id="f1">`)
	for range depth {
		b.WriteString(`<group id="g">`)
	}
	b.WriteString(`<unit id="u1"><segment id="s1"><source>deep</source></segment></unit>`)
	for range depth {
		b.WriteString(`</group>`)
	}
	b.WriteString(`</file></xliff>`)

	var foundError bool
	var blocks int
	require.NotPanics(t, func() {
		foundError, blocks = readAll(t, b.String())
	})
	// The input is well-formed XLIFF, so it parses without error and yields
	// the single innermost translatable unit.
	assert.False(t, foundError, "well-formed deeply nested XLIFF should not error")
	assert.Equal(t, 1, blocks, "expected the innermost unit as the sole block")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking. (The nil-*document* case is covered by TestReadNilDocument
// in reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
