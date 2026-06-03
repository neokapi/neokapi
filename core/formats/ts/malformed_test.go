package ts_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It
// is wrapped in require.NotPanics by callers so a panic fails the test with a
// clear message instead of crashing the run. The ts reader streams the input
// through encoding/xml (non-strict mode) and surfaces XML syntax errors as
// PartResult.Error during Read — never as a panic.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := ts.NewReader()

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

// TestReadMalformedSurfacesError feeds inputs the XML decoder genuinely cannot
// tokenize and asserts the parse error surfaces cleanly on the result channel
// (PartResult.Error) rather than panicking, hanging, or being silently
// swallowed.
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
			// Truncated document: a <source> is opened but the file ends
			// mid-content, so the decoder reaches EOF with elements still
			// open.
			name:  "truncated unterminated source",
			input: `<?xml version="1.0"?><!DOCTYPE TS><TS version="2.1"><context><name>Foo</name><message><source>Hello`,
		},
		{
			// Mismatched tags: <source> is closed by </translation>.
			name:  "mismatched end tag",
			input: `<TS version="2.1"><context><message><source>Hi</translation></message></context></TS>`,
		},
		{
			// A lone opening <TS> with no body and no close.
			name:  "unterminated TS element",
			input: `<TS version="2.1">`,
		},
		{
			// Unterminated XML comment swallows the rest of the document.
			name:  "unterminated comment",
			input: `<TS version="2.1"><!-- never ends <context></TS>`,
		},
		{
			// Unterminated CDATA section inside <source>.
			name:  "unterminated cdata",
			input: `<TS version="2.1"><context><message><source><![CDATA[unterminated</source></message></context></TS>`,
		},
		{
			// Raw, unescaped '<' in character data — not a valid element
			// start, so the decoder rejects it.
			name:  "raw lt in text",
			input: `<TS version="2.1"><context><message><source>a < b</source></message></context></TS>`,
		},
		{
			// Invalid UTF-8 bytes inside element content.
			name:  "invalid utf8 bytes",
			input: "<TS version=\"2.1\"><context><message><source>\xff\xfe\x00bad</source></message></context></TS>",
		},
		{
			// Broken DOCTYPE internal subset — an <!ENTITY declaration that
			// is never closed before EOF.
			name:  "broken doctype internal subset",
			input: `<?xml version="1.0"?><!DOCTYPE TS [<!ENTITY foo`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed TS/XML input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the decoder deliberately
// tolerates. The ts reader runs encoding/xml with Strict=false, HTMLAutoClose,
// and HTMLEntity, so it accepts unknown entities and well-formed fragments that
// simply lack the expected TS structure rather than rejecting them. These
// inputs must therefore not panic and not race; whether they surface an error,
// parse leniently, or yield nothing is an implementation detail we do not
// over-assert here. The single contract is robustness: no panic, no hang.
//
// Run with -race to surface any data race in the reader goroutine.
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
			// Pure garbage with no XML tokens at all: the decoder reads it
			// as a single (ignored) chardata run and reaches EOF cleanly.
			name:  "garbage bytes no markup",
			input: "@@@ %%% not xml at all ^^^",
		},
		{
			// Unknown/broken named entity. HTMLEntity tolerates it, so the
			// document parses without error.
			name:  "broken entity reference",
			input: `<TS version="2.1"><context><message><source>A &notreal; B</source></message></context></TS>`,
		},
		{
			// Well-formed fragment missing the <TS> wrapper entirely.
			name:  "missing TS context message structure",
			input: `<context><message><source>Hi</source></message></context>`,
		},
		{
			// A lone byte-order mark with no following content.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// Only an XML declaration, nothing else.
			name:  "xml declaration only",
			input: `<?xml version="1.0" encoding="UTF-8"?>`,
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

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested run of <context>
// elements to exercise the streaming decoder loop under heavy structural depth.
// Go grows goroutine stacks on demand, so this must complete without a
// stack-overflow panic or a hang.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	var b []byte
	b = append(b, `<TS version="2.1">`...)
	for range depth {
		b = append(b, `<context>`...)
	}
	for range depth {
		b = append(b, `</context>`...)
	}
	b = append(b, `</TS>`...)

	require.NotPanics(t, func() {
		readAll(t, string(b))
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking, and that a nil document is likewise rejected cleanly.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	reader := ts.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})

	reader2 := ts.NewReader()
	require.NotPanics(t, func() {
		err := reader2.Open(ctx, nil)
		require.Error(t, err)
	})
}
