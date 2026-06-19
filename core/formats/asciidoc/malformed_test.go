package asciidoc_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/asciidoc"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadNilDocument verifies Open rejects a nil document/reader without
// panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	r := asciidoc.NewReader()
	require.Error(t, r.Open(t.Context(), nil))
}

// TestReadMalformedInput feeds truncated/garbage/degenerate input and asserts
// the reader never panics and always produces balanced layer bookends. AsciiDoc
// has no invalid byte sequence (any text is a valid document), so the contract
// here is robustness: no panic, no error, clean bookends.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"unterminated listing", "[source]\n----\nnever closed\n"},
		{"unterminated table", "|===\n| a | b\n| c"},
		{"unterminated comment block", "////\nhidden\n"},
		{"lone delimiters", "----\n....\n++++\n"},
		{"only markers", "===\n***\n___\n--\n"},
		{"garbage bytes", "\x00\x01\x02 \xff\xfe random *unclosed _bold"},
		{"deep nesting markers", strings.Repeat("* ", 50) + "x\n"},
		{"huge pipes", "|===\n" + strings.Repeat("|", 200) + "\n|===\n"},
		{"only newlines", "\n\n\n\n"},
		{"cr only", "a\rb\rc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := asciidoc.NewReader()
			require.NotPanics(t, func() {
				require.NoError(t, r.Open(t.Context(), testutil.RawDocFromString(tc.input, model.LocaleEnglish)))
			})
			defer r.Close()

			var parts []*model.Part
			require.NotPanics(t, func() {
				for res := range r.Read(t.Context()) {
					require.NoError(t, res.Error)
					parts = append(parts, res.Part)
				}
			})
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			// Group brackets stay balanced even on truncated input.
			depth := 0
			for _, p := range parts {
				switch p.Type {
				case model.PartGroupStart:
					depth++
				case model.PartGroupEnd:
					depth--
				}
			}
			assert.Equal(t, 0, depth, "groups balanced on malformed input")
		})
	}
}

// TestMalformedRoundTripStillByteExact asserts even truncated documents survive
// the skeleton round-trip byte-for-byte (nothing is silently dropped).
func TestMalformedRoundTripStillByteExact(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"[source]\n----\nnever closed\n",
		"|===\n| a | b\n| c",
		"////\nhidden\n",
	} {
		assert.Equal(t, in, skelRoundtrip(t, in, ""))
	}
}
