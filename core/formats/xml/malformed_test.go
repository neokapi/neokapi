package xml_test

import (
	"strings"
	"testing"

	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It
// is wrapped in require.NotPanics by callers so a panic fails the test with a
// clear message instead of crashing the run.
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()

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

// TestReadMalformedSurfacesError feeds inputs the XML tokenizer genuinely cannot
// parse and asserts the parse error surfaces cleanly on the result channel
// (PartResult.Error) rather than panicking or being silently swallowed. The
// reader runs Go's encoding/xml decoder twice over the bytes — first an ITS
// pre-scan (its.ExtractRules) for embedded rules, then the main element walk —
// so a well-formedness violation is reported by whichever pass reaches it first.
// Either way the failure arrives as a single PartResult.Error.
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
			// Truncated document: the root element is opened but never
			// closed, so the decoder reaches EOF with elements still on
			// the stack and reports an unexpected EOF.
			name:  "truncated unterminated element",
			input: `<root><message>Hello World`,
		},
		{
			// Mismatched end tag: </a> closes before its child <b>, which
			// violates well-formedness (element type mismatch).
			name:  "mismatched tags",
			input: `<a><b>text</a></b>`,
		},
		{
			// Unclosed nested element: text accumulates but the closing
			// tags never arrive before EOF.
			name:  "unclosed nested element",
			input: `<root><child>text`,
		},
		{
			// Unknown/broken entity reference: encoding/xml only resolves
			// the five predefined entities (&lt; &gt; &amp; &apos; &quot;),
			// so an undeclared entity is an "invalid character entity" error.
			name:  "unknown entity reference",
			input: `<root>&badentity;</root>`,
		},
		{
			// Bare ampersand that does not begin a valid entity reference.
			name:  "bare ampersand",
			input: `<root>a & b</root>`,
		},
		{
			// Invalid UTF-8 bytes inside character data. content is held as
			// []byte and only string-cast for the decoder, so the raw bytes
			// reach encoding/xml, which rejects them as illegal UTF-8.
			name:  "invalid utf8 bytes",
			input: "<root>\xff\xfe\xfd</root>",
		},
		{
			// Unterminated CDATA section: the ]]> close marker never
			// appears before EOF.
			name:  "unterminated cdata",
			input: `<root><![CDATA[never closed`,
		},
		{
			// Unterminated comment: the --> close marker never appears.
			name:  "unterminated comment",
			input: `<root><!-- never closed`,
		},
		{
			// Unquoted attribute value — XML requires attribute values to
			// be quoted, so the decoder rejects this.
			name:  "unquoted attribute value",
			input: `<root attr=val>x</root>`,
		},
		{
			// A lone '<' that opens nothing: the decoder hits EOF mid-token.
			name:  "lone less than",
			input: `<`,
		},
		{
			// Garbage angle-bracket soup with no balanced structure.
			name:  "garbage markup",
			input: `<<<>>> @@@ </not-open>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed XML input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the reader tolerates without
// reporting an error: empty or content-free documents that the decoder walks to
// EOF cleanly, simply yielding no translatable blocks. The contract under test
// is robustness — no panic, no hang, no race — not whether a particular degenerate
// input is classified as an error.
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
			// Whitespace only — the decoder walks it to EOF with no
			// elements, producing an empty (but valid) document.
			name:  "whitespace only",
			input: "   \n\t  ",
		},
		{
			// A lone XML declaration with no root element. encoding/xml
			// treats the ProcInst and reaches EOF without complaint.
			name:  "xml declaration only",
			input: `<?xml version="1.0" encoding="UTF-8"?>`,
		},
		{
			// A leading byte-order mark followed by nothing translatable.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// A self-closing empty root carries no text, so no block is
			// emitted, but the document is well-formed.
			name:  "self-closing empty root",
			input: `<root/>`,
		},
		{
			// A standalone comment with no document content.
			name:  "comment only",
			input: `<!-- just a comment -->`,
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

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested, well-formed element
// tree to exercise the recursive element-frame walk. Go grows goroutine stacks
// on demand, so this must complete without a stack-overflow panic and surface no
// error, yielding the single innermost text node.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	input := strings.Repeat(`<a>`, depth) + `deep value` + strings.Repeat(`</a>`, depth)

	var foundError bool
	var blocks int
	require.NotPanics(t, func() {
		foundError, blocks = readAll(t, input)
	})
	// The input is well-formed XML, so it parses without error and yields the
	// single innermost translatable text node.
	assert.False(t, foundError, "well-formed deeply nested XML should not error")
	assert.Equal(t, 1, blocks, "expected the innermost value as the sole block")
}

// TestReadPathologicalDoesNotPanic feeds pathological-but-finite inputs (a very
// wide sibling list and a long unterminated run of open tags) to confirm the
// reader degrades gracefully — it must not panic, hang, or race regardless of
// whether the input is well-formed.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadPathologicalDoesNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Many sibling empty elements: well-formed, exercises the wide
			// (rather than deep) walk without recursion.
			name:  "wide sibling list",
			input: `<root>` + strings.Repeat(`<e/>`, 20000) + `</root>`,
		},
		{
			// A long run of opening tags with no closes — the decoder must
			// reach EOF and report an error without unbounded growth.
			name:  "long unbalanced open tags",
			input: strings.Repeat(`<a>`, 20000),
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

// TestReadNilReader verifies Open rejects a document whose Reader is nil without
// panicking, returning a clean error instead.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
