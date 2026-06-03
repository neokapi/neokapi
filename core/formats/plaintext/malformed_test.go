package plaintext_test

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedInput feeds truncated, garbage, binary, and invalid-encoding
// bytes through Open+Read and asserts the reader never panics. Plain text is a
// permissive format: there is no syntax to violate, so byte salad is read as
// content rather than rejected. The contract verified here is therefore the
// floor one — Open+Read survive any byte stream cleanly (graceful, no panic),
// emitting either content or an empty document, never a crash. Run with -race
// to also catch data races in the channel/goroutine path.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "nul bytes",
			input: "\x00\x00\x00\x00",
		},
		{
			name:  "binary garbage",
			input: "\x00\x01\x02\x03\xff\xfe\x80\x81\x7f",
		},
		{
			name:  "truncated utf-8 sequence",
			input: "valid text \xe2\x82", // leading bytes of '€' with the final byte chopped
		},
		{
			name:  "lone continuation bytes",
			input: "\x80\x81\x82",
		},
		{
			name:  "overlong / invalid utf-8 mixed with text",
			input: "head\xc0\xafmiddle\xf5\x80\x80\x80tail",
		},
		{
			name:  "utf-16le bom then odd byte count",
			input: "\xff\xfeA\x00B", // BOM + "A" + a dangling high byte
		},
		{
			name:  "utf-16be bom then odd byte count",
			input: "\xfe\xff\x00A\x00", // BOM + "A" + a dangling low byte
		},
		{
			name:  "control characters",
			input: "line1\x07\x1b[31m\x08\x0cline2\n",
		},
		{
			name:  "bare carriage returns and mixed line endings",
			input: "a\rb\r\nc\nd",
		},
		{
			name:  "only line endings",
			input: "\r\n\r\n\n\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := plaintext.NewReader()

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					// Plain text never produces a parse error from its byte
					// content; surface any unexpected one so it is not swallowed.
					require.NoError(t, result.Error)
				}
			})
		})
	}
}

// errReader is an io.ReadCloser that always fails. It models a source whose
// bytes cannot be read (e.g. a broken pipe or unreadable file) so the test can
// exercise the reader's io.ReadAll error branch.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("simulated read failure") }
func (errReader) Close() error             { return nil }

// TestReadIOErrorSurfaces verifies that a failure while reading the underlying
// stream surfaces as a clean PartResult.Error on the channel rather than
// panicking or being silently dropped.
func TestReadIOErrorSurfaces(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := plaintext.NewReader()
	doc := &model.RawDocument{
		URI:          "test://broken",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       errReader{},
	}

	require.NotPanics(t, func() {
		require.NoError(t, reader.Open(ctx, doc))
	})
	defer reader.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
		}
	})
	assert.True(t, foundError, "expected a clean read error to surface on the channel")
}
