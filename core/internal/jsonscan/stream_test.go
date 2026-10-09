package jsonscan_test

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/internal/jsonscan"
)

func collect(t *testing.T, ss *jsonscan.StreamScanner, in string) []jsonscan.Token {
	t.Helper()
	var got []jsonscan.Token
	for {
		tok, err := ss.Next()
		require.NoError(t, err, in)
		got = append(got, tok)
		if tok.Type == jsonscan.EOF {
			return got
		}
	}
}

func TestStreamScannerParity(t *testing.T) {
	inputs := []string{
		"{}",
		`  { "a" : "b" , "n": 12.5, "t": true, "z": null }`,
		"\ufeff{\n  \"appTitle\": \"Hello \\\"world\\\"\",\n  \"@appTitle\": { \"description\": \"the title\" }\n}\n",
		`{"emoji":"café 😀 end","arr":[1,"x",false]}`,
		`{"@@locale":"en","msg":"{count, plural, one {# item} other {# items}}"}`,
		`{"pair":"\ud83d\ude00","lone":"\u00e9"}`,
	}
	for _, in := range inputs {
		want, werr := jsonscan.New([]byte(in), "test scanner").Scan()
		require.NoError(t, werr, in)

		got := collect(t, jsonscan.NewStream(bufio.NewReader(strings.NewReader(in)), "test scanner"), in)
		require.Equal(t, want, got, "token stream mismatch for %q", in)
	}
}

// Tokens re-serialize byte-for-byte: every prefix and raw span concatenated is
// the input, including a BOM and trailing whitespace.
func TestScannerRoundTrip(t *testing.T) {
	in := "\ufeff{\n  \"k\": \"v\\u00e9\", \"n\": -1.5e3, \"b\": [true, false, null]\n}\n\n"
	toks, err := jsonscan.New([]byte(in), "test scanner").Scan()
	require.NoError(t, err)
	var sb strings.Builder
	for _, tok := range toks {
		sb.WriteString(tok.Prefix)
		sb.WriteString(tok.Raw)
	}
	require.Equal(t, in, sb.String())
}

// The error prefix names the calling format, so a message from the arb reader
// reads as the arb reader's and never as this package's.
func TestErrorPrefix(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"unexpected", `{"a": @}`, `arb scanner: unexpected character '@' at position 6`},
		{"unterminated string", `{"a": "b`, `arb scanner: unterminated string at 6`},
		{"bad literal", `{"a": tru}`, `arb scanner: expected "true" at 6`},
		{"bad escape", `{"a": "\uZZZZ"}`, `arb scanner: invalid unicode escape \uZZZZ`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := jsonscan.New([]byte(tc.in), "arb scanner").Scan()
			require.EqualError(t, err, tc.want)
		})
	}

	streamCases := []struct {
		name, in, want string
	}{
		{"unexpected", `{"a": @}`, `xcstrings scanner: unexpected character '@'`},
		{"unterminated string", `{"a": "b`, `xcstrings scanner: unterminated string`},
		{"bad literal", `{"a": tru}`, `xcstrings scanner: expected "true"`},
		{"bad escape", `{"a": "\uZZZZ"}`, `xcstrings scanner: invalid unicode escape \uZZZZ`},
	}
	for _, tc := range streamCases {
		t.Run("stream "+tc.name, func(t *testing.T) {
			ss := jsonscan.NewStream(bufio.NewReader(strings.NewReader(tc.in)), "xcstrings scanner")
			var err error
			for err == nil {
				var tok jsonscan.Token
				tok, err = ss.Next()
				if err == nil && tok.Type == jsonscan.EOF {
					t.Fatalf("expected an error for %q", tc.in)
				}
			}
			require.EqualError(t, err, tc.want)
		})
	}
}
