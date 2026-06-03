package ttx_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAll drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It is
// wrapped in require.NotPanics by callers so a panic fails the test with a clear
// message instead of crashing the run.
//
// The reader does NOT get a skeleton store: the malformed inputs here exercise
// the streaming parse path (encoding detect → encoding/xml token loop), not the
// byte-offset skeleton build, so we keep the contract narrow — robustness of the
// parser, not round-trip fidelity (skeleton fidelity is covered by
// skeleton_test.go on well-formed input).
func readAll(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := ttx.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; parse and decode errors
		// surface later, during Read.
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

// TestReadMalformedSurfacesError feeds XML the encoding/xml decoder genuinely
// cannot tokenize and asserts the parse error surfaces cleanly on the result
// channel (PartResult.Error) rather than panicking or being silently swallowed.
//
// The reader's main token loop runs with decoder.Strict = false (lenient), so it
// tolerates a lot of structural sloppiness; these inputs are the cases where the
// decoder still returns a hard tokenization error (bad entity, control bytes,
// malformed declaration) — those must reach the channel as an Error.
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
			// Broken entity: '&' begins an entity reference that is never
			// terminated with ';' before EOF, which the XML tokenizer rejects
			// as an invalid character entity.
			name: "broken entity",
			input: `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag Version="2.0"><Body><Raw><Tu><Tuv Lang="EN-US">A &amp B`,
		},
		{
			// Raw NUL / control bytes inside element content. These are not
			// legal XML characters and the tokenizer rejects them.
			name:  "invalid control bytes",
			input: "<TRADOStag><Body><Raw><Tu><Tuv Lang=\"EN-US\">\x00\x01\x02</Tuv></Tu></Raw></Body></TRADOStag>",
		},
		{
			// Garbage in attribute position: a stray '<' opens a tag whose
			// name is illegal, so the tokenizer fails.
			name:  "illegal tag name",
			input: `<TRADOStag><Body><Raw><Tu><< /></Tu></Raw></Body></TRADOStag>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean PartResult.Error for malformed TTX input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the lenient decoder tolerates or
// that simply have no extractable content. The reader runs encoding/xml with
// Strict = false, so it recovers from unbalanced / mismatched tags, missing TTX
// structure, and truncation rather than rejecting them. These inputs must not
// panic and must not race; whether they surface an error or parse to zero blocks
// is an implementation detail we do not over-assert. The single contract is
// robustness: no panic, no hang.
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
			// Lone byte-order mark with no content; encoding.ToUTF8 strips it
			// and the parse reduces to empty.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// Truncated mid start tag: the root element is opened but the tag
			// is never closed before EOF.
			name:  "truncated start tag",
			input: `<?xml version="1.0"?><TRADOStag Version="2.0`,
		},
		{
			// Truncated mid element: structure opens but EOF arrives inside a
			// <Tuv> before its content/close.
			name: "truncated mid tuv",
			input: `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag><Body><Raw><Tu MatchPercent="0"><Tuv Lang="EN-US">Hello`,
		},
		{
			// Mismatched tags: </Body> closes while <Tu>/<Tuv> are still open.
			// Lenient mode does not treat this as a hard error.
			name: "mismatched close tag",
			input: `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag><Body><Raw><Tu><Tuv Lang="EN-US">Hi</Body></Raw></TRADOStag>`,
		},
		{
			// Missing TTX structure entirely: well-formed XML, but no
			// <TRADOStag>/<Tu>/<Tuv>. Yields zero translatable blocks.
			name:  "missing ttx structure",
			input: `<root><child>not a ttx file</child></root>`,
		},
		{
			// Plain text, not XML at all. The lenient tokenizer treats it as a
			// single CharData run outside any <Raw>, so nothing is extracted.
			name:  "plain text not xml",
			input: "just some bytes, no markup here",
		},
		{
			// Only a stray closing tag, nothing opened.
			name:  "lone closing tag",
			input: `</TRADOStag>`,
		},
		{
			// Unknown named entity inside a <Tuv>: encoding/xml only resolves
			// the five predefined XML entities and numeric refs, so this fails
			// to tokenize — but it fails inside parseTransUnitWithSkeleton,
			// which returns nil (drops the unit) rather than surfacing the
			// error. A valid graceful-empty path: zero blocks, no panic.
			name: "unknown named entity in tuv",
			input: `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag><Body><Raw><Tu><Tuv Lang="EN-US">&bogus;</Tuv></Tu></Raw></Body></TRADOStag>`,
		},
		{
			// Invalid numeric character reference (out-of-range code point)
			// inside a <Tuv>: same graceful-empty path as above.
			name: "invalid numeric entity in tuv",
			input: `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag><Body><Raw><Tu><Tuv Lang="EN-US">&#xFFFFFFFF;</Tuv></Tu></Raw></Body></TRADOStag>`,
		},
		{
			// A <Tu> whose body is truncated before any <Tuv> closes — drives
			// parseTransUnitWithSkeleton to EOF, which must return nil (no
			// block) without panicking.
			name: "truncated inside tu",
			input: `<?xml version="1.0" encoding="utf-8"?>
<TRADOStag><Body><Raw><Tu MatchPercent="0"><Tuv Lang="EN-US">`,
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

// TestReadDeeplyNestedDoesNotPanic feeds deeply nested elements to exercise the
// decoder's recursive descent and the reader's nesting bookkeeping (extDepth /
// depth counters). Go grows goroutine stacks on demand, so this must complete
// without a stack-overflow panic or a hang.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	input := `<TRADOStag><Body><Raw>` +
		strings.Repeat(`<ut>`, depth) +
		`deep` +
		strings.Repeat(`</ut>`, depth) +
		`</Raw></Body></TRADOStag>`

	require.NotPanics(t, func() {
		readAll(t, input)
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil without
// panicking. (The nil-*document* case is covered by TestReadNilDocument in
// reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := ttx.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
