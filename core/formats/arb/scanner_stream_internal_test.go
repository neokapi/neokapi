package arb

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamScannerParity(t *testing.T) {
	inputs := []string{
		"{}",
		`  { "a" : "b" , "n": 12.5, "t": true, "z": null }`,
		"\ufeff{\n  \"appTitle\": \"Hello \\\"world\\\"\",\n  \"@appTitle\": { \"description\": \"the title\" }\n}\n",
		`{"emoji":"café 😀 end","arr":[1,"x",false]}`,
		`{"@@locale":"en","msg":"{count, plural, one {# item} other {# items}}"}`,
	}
	for _, in := range inputs {
		want, werr := newScanner([]byte(in)).scan()
		require.NoError(t, werr, in)

		ss := newStreamScanner(bufio.NewReader(strings.NewReader(in)))
		var got []token
		for {
			tok, err := ss.next()
			require.NoError(t, err, in)
			got = append(got, tok)
			if tok.typ == tokEOF {
				break
			}
		}
		require.Equal(t, want, got, "token stream mismatch for %q", in)
	}
}
