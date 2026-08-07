package xmlesc_test

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/stretchr/testify/require"
)

// TestValidCharMatchesTheParser checks the Char production against the parser
// that enforces it: for every character below U+0100 plus the boundary cases,
// ValidChar must agree with whether encoding/xml accepts a document containing
// it — as a literal and as a numeric character reference.
//
// The reference matters because the tempting shortcut is wrong: a numeric
// character reference does not rescue a character outside the production, so
// `&#7;` is as invalid as a raw U+0007 and there is nothing to escape it with.
func TestValidCharMatchesTheParser(t *testing.T) {
	var probes []rune
	for r := range rune(0x100) {
		probes = append(probes, r)
	}
	probes = append(probes, 0xD7FF, 0xE000, 0xFFFD, 0xFFFE, 0xFFFF, 0x10000, 0x10FFFF)

	for _, r := range probes {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			want := xmlesc.ValidChar(r)
			require.Equal(t, want, parserAccepts(t, xmlesc.Text(string(r))),
				"U+%04X through Text", r)
			require.Equal(t, want, parserAccepts(t, fmt.Sprintf("&#x%X;", r)),
				"character reference for U+%04X", r)
		})
	}
}

// parserAccepts reports whether encoding/xml parses a document whose text
// content is the given (already-escaped) body.
func parserAccepts(t *testing.T, body string) bool {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader("<r>" + body + "</r>"))
	for {
		_, err := dec.Token()
		if err != nil {
			return err.Error() == "EOF"
		}
	}
}

func TestCheckText(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantRune  rune
		wantIndex int
		wantErr   bool
	}{
		{name: "plain", in: "hello"},
		{name: "markup_delimiters", in: `a < b & c > "d"`},
		{name: "allowed_whitespace", in: "a\tb\nc\rd"},
		{name: "astral", in: "\U0001F600"},
		{name: "bell", in: "ab\x07cd", wantRune: 0x07, wantIndex: 2, wantErr: true},
		{name: "null_at_start", in: "\x00x", wantRune: 0x00, wantIndex: 0, wantErr: true},
		{name: "first_offender_wins", in: "a\x0Bb\x0Cc", wantRune: 0x0B, wantIndex: 1, wantErr: true},
		{name: "index_is_a_byte_offset", in: "é\x01", wantRune: 0x01, wantIndex: 2, wantErr: true},
		// Undecodable bytes pass through: the escapers copy them untouched
		// rather than rewriting them to U+FFFD, and rejecting them here would
		// make a byte-preserving path lossy.
		{name: "invalid_utf8", in: "a\xffb"},
		{name: "real_replacement_char", in: "a�b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := xmlesc.CheckText(tc.in)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			var charErr *xmlesc.UnrepresentableCharError
			require.ErrorAs(t, err, &charErr)
			require.Equal(t, tc.wantRune, charErr.Rune)
			require.Equal(t, tc.wantIndex, charErr.Index)
			require.Contains(t, err.Error(), fmt.Sprintf("U+%04X", tc.wantRune))
		})
	}
}
