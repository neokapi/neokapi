package tmx_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/tmx"
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
	reader := tmx.NewReader()

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

// TestReadMalformedSurfacesError feeds inputs the streaming XML decoder
// genuinely cannot tokenize and asserts the parse error surfaces cleanly on the
// result channel (PartResult.Error) rather than panicking or hanging. The TMX
// reader sets decoder.Strict = false (to tolerate DTDs and HTML entities), so
// these are the cases where even the lenient decoder must give up: truncated
// markup, mismatched tags, an unterminated construct, or bytes that are not
// valid text.
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
			// Document cut off mid start-tag: the decoder reaches EOF while
			// still reading the <seg> element name/attributes.
			name:  "truncated mid start tag",
			input: `<?xml version="1.0"?><tmx version="1.4"><body><tu><tuv xml:lang="en"><seg`,
		},
		{
			// <seg> content is opened but neither </seg> nor any ancestor end
			// tag ever arrives.
			name:  "unterminated seg at eof",
			input: `<?xml version="1.0"?><tmx><body><tu><tuv xml:lang="en"><seg>Hello`,
		},
		{
			// End tags interleave in the wrong order (</tu> appears before the
			// still-open <seg>/<tuv>), which violates XML well-formedness even
			// in non-strict mode.
			name:  "mismatched nesting",
			input: `<?xml version="1.0"?><tmx><body><tu><tuv><seg>Hi</tu></seg></tuv></body></tmx>`,
		},
		{
			// A comment is opened with <!-- but never closed, so the decoder
			// consumes the rest of the input looking for -->.
			name:  "unclosed comment",
			input: `<?xml version="1.0"?><tmx><!-- unclosed <body><tu></tu></body></tmx>`,
		},
		{
			// Bare angle brackets that never form a valid element name.
			name:  "stray opening brackets",
			input: `<<<<<<`,
		},
		{
			// Several elements opened, none closed before EOF.
			name:  "nested unterminated elements",
			input: `<tmx><body><tu><tuv><seg>`,
		},
		{
			// Bytes that are not valid UTF-8 (and not a recognized legacy
			// encoding with a BOM), so transcoding/XML tokenization rejects
			// them rather than silently corrupting content.
			name:  "invalid byte sequence in seg",
			input: "<?xml version=\"1.0\"?><tmx><body><tu><tuv xml:lang=\"en\"><seg>\xff\xfe\xfa\xfb</seg></tuv></tu></body></tmx>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAll(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed TMX input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the decoder deliberately
// tolerates. With decoder.Strict = false and HTML entity/auto-close handling
// enabled, the TMX reader recovers from things a validating parser would reject
// (unknown entities, missing TMX structure, raw ampersands, non-TMX XML). These
// inputs must not panic and not race; whether they surface an error or parse
// leniently to zero/partial blocks is an implementation detail we do not
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
			// Whitespace only — the decoder reaches EOF having emitted no
			// elements.
			name:  "whitespace only",
			input: "   \n\t  \n",
		},
		{
			// A lone UTF-8 BOM with no following content.
			name:  "lone bom",
			input: "\uFEFF",
		},
		{
			// Well-formed XML that is not TMX at all: no <tu>/<tuv>/<seg>, so
			// the reader walks it and emits no translatable blocks.
			name:  "non-tmx xml document",
			input: `<?xml version="1.0"?><html><body><p>hi</p></body></html>`,
		},
		{
			// Plain garbage that is not XML; the decoder rejects or skips it,
			// but must not crash.
			name:  "garbage bytes",
			input: "@@@ %%% not xml at all",
		},
		{
			// Unknown entity reference. Strict=false + HTMLEntity means the
			// decoder does not necessarily fail here.
			name:  "unknown entity reference",
			input: `<?xml version="1.0"?><tmx><body><tu><tuv xml:lang="en"><seg>A &nonsense; B</seg></tuv></tu></body></tmx>`,
		},
		{
			// Raw, unescaped ampersand inside seg content — invalid XML that
			// the lenient decoder tolerates.
			name:  "raw ampersand in seg",
			input: `<?xml version="1.0"?><tmx><body><tu><tuv xml:lang="en"><seg>A & B</seg></tuv></tu></body></tmx>`,
		},
		{
			// A <tu> with no <tuv>/<seg> children at all: structurally valid
			// XML, semantically empty TU.
			name:  "tu missing tuv and seg",
			input: `<?xml version="1.0"?><tmx><body><tu tuid="x"></tu></body></tmx>`,
		},
		{
			// <tuv> present but no <seg> inside it.
			name:  "tuv missing seg",
			input: `<?xml version="1.0"?><tmx><body><tu><tuv xml:lang="en"></tuv></tu></body></tmx>`,
		},
		{
			// <seg> appearing without an enclosing <tuv> — the reader's guards
			// (currentTUV != nil) must keep it from dereferencing nil state.
			name:  "seg outside tuv",
			input: `<?xml version="1.0"?><tmx><body><seg>orphan</seg></body></tmx>`,
		},
		{
			// Inline codes (<bpt>/<ept>/<ph>) appearing outside any <seg>,
			// exercising the inSeg/segBuilder nil guards.
			name:  "inline codes outside seg",
			input: `<?xml version="1.0"?><tmx><body><bpt i="1"/><ept i="1"/><ph/></body></tmx>`,
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

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested run of elements to
// exercise the streaming decoder's depth handling. Go grows goroutine stacks on
// demand, so this must complete without a stack-overflow panic or a hang. The
// nesting is intentionally not closed, so the decoder surfaces an EOF/parse
// error rather than yielding any block — the contract here is purely "no panic,
// no hang".
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	input := `<?xml version="1.0"?>` + strings.Repeat(`<x>`, depth)

	require.NotPanics(t, func() {
		readAll(t, input)
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := tmx.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}

// TestReadNilDocument verifies Open rejects a nil document without panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := tmx.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, nil)
		require.Error(t, err)
	})
}
