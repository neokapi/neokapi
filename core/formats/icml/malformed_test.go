package icml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/icml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedICML feeds truncated, garbage, and unterminated ICML input and
// asserts that Read surfaces a clean error on its result channel rather than
// panicking or silently swallowing the parse failure. The reader walks the
// document with encoding/xml; a decoder error other than io.EOF must be reported
// via PartResult.Error rather than quietly breaking the token loop.
func TestReadMalformedICML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// Not XML at all — the decoder rejects the first token.
			name:  "garbage bytes",
			input: "\x00\x01\x02 definitely not xml <<<{[",
		},
		{
			// Well-formed prologue but the document is cut off mid-element,
			// leaving an unclosed Story/ParagraphStyleRange/Content.
			name:  "truncated document",
			input: `<?xml version="1.0"?><Document><Story><ParagraphStyleRange><CharacterStyleRange><Content>First`,
		},
		{
			// A Content element whose text is never terminated by a closing tag.
			name:  "unterminated content",
			input: `<?xml version="1.0"?><Document><Story><ParagraphStyleRange><CharacterStyleRange><Content>Hello world`,
		},
		{
			// Mismatched closing tag — valid prologue, then a structural error.
			name:  "mismatched close tag",
			input: `<?xml version="1.0"?><Document><Story></Document>`,
		},
		{
			// Unquoted attribute value — a lexical XML error mid-stream.
			name:  "bad attribute",
			input: `<?xml version="1.0"?><Document><Story foo=bar></Story></Document>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := icml.NewReader()
			// Open only validates the document/reader; the parse error surfaces
			// during Read.
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
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
			assert.True(t, foundError, "expected a clean error for malformed ICML input")
		})
	}
}

// TestReadNilReader verifies Open rejects a document with a nil reader without
// panicking — the reader must guard against a missing byte source.
// (Open rejecting a nil document is covered by TestReadNilDocument in
// reader_test.go.)
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := icml.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{URI: "broken.icml", Reader: nil})
		require.Error(t, err)
	})
}
