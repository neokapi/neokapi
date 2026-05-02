//go:build parity

package roundtrip_test

import (
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
)

// TestRoundTrip_Plaintext drives the simplest possible round-trip:
// a plain-text document with one line per block. All three engines
// should round-trip it identically — any divergence is a real bug
// in that engine's reader/writer pair.
func TestRoundTrip_Plaintext(t *testing.T) {
	input := []byte("Hello world\nAnother line\nThird paragraph\n")
	roundtrip.RunThreeWay(t, roundtrip.Case{
		Name:     "three_lines",
		FormatID: "plaintext",
		Input: roundtrip.Input{
			Bytes:    input,
			Filename: "doc.txt",
		},
	},
		&roundtrip.NativeEngine{FormatID: "plaintext"},
		&roundtrip.BridgeEngine{FilterClass: "okf_plaintext"},
		&roundtrip.TikalEngine{},
	)
}
