package vignette_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAllMalformed drains the reader's result channel, returning whether any
// clean PartResult.Error surfaced and the number of translatable Blocks
// emitted. Both Open and the channel drain run inside require.NotPanics so a
// panic fails the test with a clear message instead of crashing the run.
//
// withSkeleton attaches a SkeletonStore before reading, exercising the
// byte-offset skeleton-writer path (writeSkeleton / findReverse) that is only
// reached when parsing succeeds far enough to emit regions.
func readAllMalformed(t *testing.T, input string, withSkeleton bool) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := vignette.NewReader()

	if withSkeleton {
		store, err := format.NewSkeletonStore()
		require.NoError(t, err)
		reader.SetSkeletonStore(store)
	}

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
			// Count only translatable Blocks: the reader surfaces skipped,
			// non-source-locale instances as Translatable:false content
			// blocks by default, which these robustness cases don't care
			// about (the contract is "no translatable extraction").
			if result.Part != nil && result.Part.Type == model.PartBlock {
				if blk, ok := result.Part.Resource.(*model.Block); ok && blk.Translatable {
					blocks++
				}
			}
		}
	})
	return foundError, blocks
}

// TestReadMalformedSurfacesError feeds inputs the underlying XML decoder
// genuinely cannot tokenize and asserts the parse error surfaces cleanly on
// the result channel (PartResult.Error) rather than panicking, hanging, or
// being silently swallowed. parseInstances propagates the decoder error and
// readContent forwards it as a single PartResult.Error.
//
// Run with -race to catch any data race in the reader goroutine that drives
// the channel.
func TestReadMalformedSurfacesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Truncated structure: the document opens an extractable value
			// element but EOF arrives before the closing tag, so the decoder
			// reports an unexpected EOF.
			name: "truncated mid value element",
			input: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
				`<importContentInstance><contentInstance>` +
				`<attribute name="SMCCONTENT-BODY"><valueCLOB>hello world`,
		},
		{
			// Broken markup: a half-written start tag with stray angle
			// brackets the decoder cannot tokenize.
			name:  "broken start tag",
			input: `<packageBody><importContentInstance <<<< name=>`,
		},
		{
			// Pure garbage / non-markup bytes that never form a valid token.
			name:  "garbage bytes",
			input: "@@@ %%% ^^^ <not real xml",
		},
		{
			// Random binary bytes including NULs and high bytes — the decoder
			// rejects the illegal characters.
			name:  "binary garbage",
			input: "\x00\x01\x02\xff\xfe<importContentInstance>\x00\x01",
		},
		{
			// Invalid UTF-8 inside character data: the decoder reports the
			// illegal byte sequence.
			name:  "invalid utf8 in chardata",
			input: "<importContentInstance>\xff\xfe\xfd</importContentInstance>",
		},
		{
			// A lone unclosed start tag at EOF.
			name:  "lone open tag",
			input: `<importContentInstance`,
		},
		{
			// A valid extractable block followed by a truncated second block:
			// the first instance closes cleanly, then EOF hits mid-value. The
			// decoder error must still surface (no partial skeleton emission).
			name: "valid block then truncation",
			input: `<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">` +
				`<importContentInstance><contentInstance>` +
				`<attribute name="SMCCONTENT-BODY"><valueString>x</valueString></attribute>` +
				`</contentInstance></importContentInstance>` +
				`<importContentInstance><contentInstance>` +
				`<attribute name="SMCCONTENT-BODY"><valueString>broken`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Exercise both the plain path and the skeleton-writer path; both
			// must surface the error without panicking.
			foundError, blocks := readAllMalformed(t, tt.input, false)
			assert.True(t, foundError, "expected a clean error for malformed XML input")
			assert.Zero(t, blocks, "a malformed document should emit no translatable Blocks")

			foundErrorSkel, _ := readAllMalformed(t, tt.input, true)
			assert.True(t, foundErrorSkel, "expected a clean error with a SkeletonStore attached")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the reader deliberately
// tolerates. The XML decoder runs in non-strict mode (dec.Strict = false), so
// unknown/undefined entities and other minor lenience are accepted rather than
// rejected. Empty input yields an empty token stream. These inputs must not
// panic and not race; whether they surface an error or parse leniently is an
// implementation detail we do not over-assert. The single contract is
// robustness: no panic, no hang, zero Blocks.
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
			// Whitespace-only input reduces to an empty token stream.
			name:  "whitespace only",
			input: "   \n\t  ",
		},
		{
			// Undefined entities: in non-strict mode the decoder tolerates
			// these rather than failing.
			name: "undefined entities",
			input: `<importContentInstance><contentInstance>` +
				`<attribute name="SMCCONTENT-BODY"><valueString>&notanentity; &amp; text</valueString></attribute>` +
				`</contentInstance></importContentInstance>`,
		},
		{
			// A well-formed but content-free document: no importContentInstance
			// elements, so nothing is extracted.
			name: "well-formed empty package",
			input: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport"><importProject/></packageBody>`,
		},
		{
			// A lone byte-order mark with no following content. The decoder
			// treats U+FEFF as leading whitespace, so this reduces to empty.
			name:  "lone bom",
			input: "\uFEFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				_, blocks := readAllMalformed(t, tt.input, false)
				assert.Zero(t, blocks, "lenient input should emit no translatable Blocks")
			})
			require.NotPanics(t, func() {
				readAllMalformed(t, tt.input, true)
			})
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested element tree to
// exercise the decoder's token loop and the readValueElementContent depth
// counter without a stack-overflow panic. Go's encoding/xml decoder is
// iterative (not recursive) over nesting, and our value-element reader uses an
// explicit depth counter, so deep nesting must complete cleanly.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	var b []byte
	b = append(b, []byte(`<packageBody xmlns="http://www.vignette.com/xmlschemas/importexport">`)...)
	for range depth {
		b = append(b, []byte("<wrap>")...)
	}
	for range depth {
		b = append(b, []byte("</wrap>")...)
	}
	b = append(b, []byte("</packageBody>")...)

	require.NotPanics(t, func() {
		_, blocks := readAllMalformed(t, string(b), false)
		// No importContentInstance anywhere → nothing extracted, but the deep
		// tree must be walked without crashing.
		assert.Zero(t, blocks, "deeply nested non-instance tree extracts nothing")
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader field is nil
// without panicking. (The nil-*document* case is covered by TestReadNilDocument
// in reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	require.NotPanics(t, func() {
		reader := vignette.NewReader()
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
