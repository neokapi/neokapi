package dtd_test

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/formats/dtd"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader is an io.Reader that always fails, modelling an input that cannot
// be read to completion (e.g. a truncated stream or I/O fault).
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }

// drain reads the whole PartResult channel, returning every part and the first
// error surfaced. It is the harness for the panic/error assertions below: a
// well-behaved reader must terminate the stream cleanly rather than panic, even
// on garbage input.
func drain(ch <-chan model.PartResult) ([]*model.Part, error) {
	var parts []*model.Part
	var firstErr error
	for result := range ch {
		if result.Error != nil && firstErr == nil {
			firstErr = result.Error
		}
		if result.Part != nil {
			parts = append(parts, result.Part)
		}
	}
	return parts, firstErr
}

// TestReadMalformedNeverPanics feeds truncated, unterminated, and garbage DTD
// fragments and asserts the reader degrades gracefully: Open succeeds, Read
// never panics, and the stream terminates with a clean LayerEnd (no error and
// no half-emitted part) even when a declaration cannot be parsed. The DTD
// reader is deliberately lenient — an unterminated declaration is treated as
// the end of parseable content rather than a hard failure — so these cases
// exercise the early-exit branches (indexCloseAngleQuoted == -1, the unclosed
// <!-- comment path, and the truncated <! / <? paths) without surfacing a
// channel error.
func TestReadMalformedNeverPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// indexCloseAngleQuoted == -1: an open quote with no closing
			// quote means the value (and the declaration) never terminate.
			name:  "truncated entity unclosed quote",
			input: `<!ENTITY greeting "Hello world`,
		},
		{
			// indexCloseAngleQuoted == -1: a quoted value never closes, so
			// no '>' is found outside the quote either.
			name:  "truncated entity no closing angle",
			input: `<!ENTITY greeting "Hello`,
		},
		{
			// A bare <!ENTITY with no name, value, or terminator.
			name:  "lone entity keyword",
			input: `<!ENTITY`,
		},
		{
			name:  "entity name only no value",
			input: `<!ENTITY greeting >`,
		},
		{
			// findEntityValuePos == -1: the declaration parses (it is a
			// non-entity once the value can't be found) but the skeleton
			// value-position lookup must not panic on the partial quote.
			name:  "entity value single open quote",
			input: `<!ENTITY greeting "`,
		},
		{
			// Unclosed <!-- comment: rest of the input is consumed as data.
			name:  "unclosed comment",
			input: `<!-- a comment that never closes`,
		},
		{
			name:  "comment then truncated entity",
			input: "<!--note-->\n<!ENTITY greeting \"Hello",
		},
		{
			// Truncated generic <! declaration (ELEMENT/ATTLIST/etc.).
			name:  "truncated element declaration",
			input: `<!ELEMENT note (to,from`,
		},
		{
			// Truncated processing instruction (no ?> terminator).
			name:  "truncated processing instruction",
			input: `<?xml version="1.0"`,
		},
		{
			name:  "lone less than",
			input: `<`,
		},
		{
			name:  "lone ampersand",
			input: `&`,
		},
		{
			name:  "lone percent",
			input: `%`,
		},
		{
			name:  "stray markup characters",
			input: `< & % > ; "`,
		},
		{
			name:  "garbage bytes",
			input: "\x00\x01\x02\xff\xfe random \x7f bytes",
		},
		{
			name:  "empty input",
			input: ``,
		},
		{
			name:  "whitespace only",
			input: "   \n\t\r ",
		},
		{
			// buildEntityValueRuns: a value whose '&' / '%' refs have no
			// terminating ';' must fall through to literal text, not panic.
			name:  "entity value unterminated refs",
			input: `<!ENTITY x "tail & and % no semicolons">`,
		},
		{
			// buildEntityValueRuns: a numeric character reference whose
			// digits don't parse (#zz / #xZZ) must be preserved as a Ph,
			// not crash on strconv.
			name:  "entity value bad numeric refs",
			input: `<!ENTITY x "&#zz; and &#xZZ;">`,
		},
		{
			// buildEntityValueRuns: a multibyte UTF-8 value drives the
			// utf8.DecodeRuneInString default branch.
			name:  "entity value multibyte utf8",
			input: `<!ENTITY x "héllo — 日本語 🎉">`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := dtd.NewReader()

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			var parts []*model.Part
			var streamErr error
			require.NotPanics(t, func() {
				parts, streamErr = drain(reader.Read(ctx))
			})

			// Lenient parsing: malformed declarations are absorbed rather
			// than surfaced as channel errors. The contract under test is
			// that the reader never panics and always emits a well-formed
			// LayerStart … LayerEnd envelope.
			require.NoError(t, streamErr, "lenient DTD parse should not surface a channel error")
			require.NotEmpty(t, parts, "reader should always emit at least the layer envelope")
			assert.Equal(t, model.PartLayerStart, parts[0].Type, "stream must open with LayerStart")
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "stream must close with LayerEnd")
		})
	}
}

// TestReadIOErrorSurfaces verifies the one genuine error-surfacing path: when
// the underlying reader fails mid-read, io.ReadAll's error is reported on the
// PartResult channel (reader.go) rather than swallowed or turned into a panic.
func TestReadIOErrorSurfaces(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := dtd.NewReader()

	err := reader.Open(ctx, testutil.RawDocFromReader(errReader{}, "test://invalid", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	var streamErr error
	require.NotPanics(t, func() {
		_, streamErr = drain(reader.Read(ctx))
	})
	require.Error(t, streamErr, "an I/O read failure must surface on the result channel")
	assert.Contains(t, streamErr.Error(), "dtd:", "error should be wrapped with the format prefix")
}

// TestOpenNilDocument verifies Open rejects a nil document without panicking.
func TestOpenNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := dtd.NewReader()

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
	reader := dtd.NewReader()

	doc := &model.RawDocument{URI: "test://input", SourceLocale: model.LocaleEnglish}
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, doc)
	})
	require.Error(t, err)
}
