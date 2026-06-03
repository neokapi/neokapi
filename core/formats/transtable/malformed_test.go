package transtable_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/transtable"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAllMalformed drains the reader's result channel for the given input,
// returning whether any clean PartResult.Error surfaced and the number of
// translatable blocks emitted. Both Open and the channel drain run inside
// require.NotPanics so a panic fails the test with a clear message instead of
// crashing the run.
//
// Run the package with -race to surface any data race in the reader goroutine
// that drives the channel.
func readAllMalformed(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := transtable.NewReader()

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

// TestReadMalformedSurfacesError feeds inputs the TransTable reader genuinely
// cannot parse and asserts the parse error surfaces cleanly on the result
// channel (PartResult.Error) rather than panicking or being silently swallowed.
//
// The header is validated first (signature must be "TransTableV1"); a bad
// header short-circuits with headerErr. A well-formed header followed by a row
// whose first cell is not an `okpCtx:tu=<id>` crumb fails during parseRows.
func TestReadMalformedSurfacesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// First non-blank line is not the TransTableV1 signature, so
			// readHeader rejects it before any rows are considered.
			name:  "wrong signature",
			input: "NotATransTable\ten\tfr\n\"okpCtx:tu=1\"\t\"hello\"\n",
		},
		{
			// Pure garbage on the first line: still treated as the header,
			// fails the signature check.
			name:  "garbage bytes as header",
			input: "@@@ %%% ^^^\n",
		},
		{
			// Raw NUL / control bytes on the first line — exercises the
			// header path with binary noise; must error, never panic.
			name:  "binary noise as header",
			input: "\x00\x01\x02\x7f\xff\tjunk\n",
		},
		{
			// Invalid UTF-8 byte sequence on the header line. The reader does
			// byte/string work only, so this must surface a signature error
			// rather than panicking on the malformed rune.
			name:  "invalid utf8 header",
			input: "\xff\xfe\xfd\n",
		},
		{
			// Valid header, but the data row has no tab at all and is not a
			// crumb, so the whole line is read as the first cell and fails
			// parseCrumb.
			name:  "valid header missing delimiter row",
			input: "TransTableV1\ten\tfr\nthis is not a crumb\n",
		},
		{
			// Valid header, but the first cell is a non-crumb token (no
			// okpCtx:tu= prefix). parseRow returns an invalid-crumb error.
			name:  "valid header non-crumb first cell",
			input: "TransTableV1\ten\tfr\n\"random\"\t\"hello\"\n",
		},
		{
			// crumb prefix present but the id is empty (okpCtx:tu= followed by
			// nothing) — parseCrumb rejects the empty rest.
			name:  "empty crumb id",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=\"\t\"hello\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAllMalformed(t, tt.input)
			assert.True(t, foundError, "expected a clean error for malformed TransTable input")
		})
	}
}

// TestReadLenientInputsDoNotPanic feeds inputs the reader deliberately
// tolerates: empty documents, header-only documents, whitespace noise, ragged
// rows with extra columns, and rows that carry a valid crumb but omit the
// source/target cells. None of these are hard errors — the reader best-effort
// parses them. The single contract here is robustness: no panic, no hang, no
// race. Whether a given input yields an error or parses leniently beyond the
// asserted invariants is an implementation detail we do not over-assert.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadLenientInputsDoNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// No input at all → LayerStart/End only, no error, no blocks.
			name:  "empty input",
			input: "",
		},
		{
			// Only whitespace-only lines → header path treats them as
			// skippable and reaches EOF as an empty document.
			name:  "whitespace only",
			input: "   \n\t\n  \r\n",
		},
		{
			// A well-formed header with no data rows.
			name:  "header only",
			input: "TransTableV1\ten\tfr\n",
		},
		{
			// Header with no trailing newline at all.
			name:  "header no trailing newline",
			input: "TransTableV1\ten\tfr",
		},
		{
			// Header with no locale cells — readHeader leaves src/trg empty
			// and falls back to the document/default locales.
			name:  "header without locales",
			input: "TransTableV1\n\"okpCtx:tu=1\"\t\"hello\"\n",
		},
		{
			// Valid crumb but only one cell (no source) — source defaults to
			// empty, no error.
			name:  "crumb only no source cell",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=1\"\n",
		},
		{
			// Ragged row: more than three tab-separated cells. SplitN caps at
			// three, folding the extra tabs into the target cell.
			name:  "ragged row extra columns",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=1\"\t\"src\"\t\"trg\"\textra\tmore\n",
		},
		{
			// Truncated final row: no trailing newline, ends mid-cell.
			name:  "truncated final row",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=1\"\t\"unterminated",
		},
		{
			// Dangling backslash at EOF inside a cell — unescape must not read
			// past the end of the string.
			name:  "dangling escape at eof",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=1\"\t\"trailing\\",
		},
		{
			// Cell with a lone closing quote (unbalanced) — stripQuotes only
			// strips a matched pair, leaving the content intact.
			name:  "unbalanced quote in cell",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=1\"\t\"open only\n",
		},
		{
			// Carriage-return-only line endings throughout.
			name:  "cr only line endings",
			input: "TransTableV1\ten\tfr\r\"okpCtx:tu=1\"\t\"hello\"\r",
		},
		{
			// Embedded invalid UTF-8 in an otherwise valid source cell. The
			// reader copies bytes through; it must not panic on the bad rune.
			name:  "invalid utf8 in source cell",
			input: "TransTableV1\ten\tfr\n\"okpCtx:tu=1\"\t\"bad\xff\xfebytes\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAllMalformed(t, tt.input)
			})
		})
	}
}

// TestReadEmptyYieldsNoBlocks pins the empty-document contract: no input means
// no error and zero blocks (just the LayerStart/LayerEnd pair).
func TestReadEmptyYieldsNoBlocks(t *testing.T) {
	t.Parallel()
	foundError, blocks := readAllMalformed(t, "")
	assert.False(t, foundError, "empty input should not surface an error")
	assert.Zero(t, blocks, "empty input should yield no blocks")
}

// TestReadHeaderOnlyYieldsNoBlocks pins the header-only contract: a valid
// header with no data rows parses cleanly and yields zero blocks.
func TestReadHeaderOnlyYieldsNoBlocks(t *testing.T) {
	t.Parallel()
	foundError, blocks := readAllMalformed(t, "TransTableV1\ten\tfr\n")
	assert.False(t, foundError, "header-only input should not surface an error")
	assert.Zero(t, blocks, "header-only input should yield no blocks")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := transtable.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
