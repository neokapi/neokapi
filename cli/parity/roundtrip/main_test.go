//go:build parity

package roundtrip_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/cli/parity/roundtrip"
)

// TestMain enforces the parity round-trip suite's hard requirements
// before any test runs: tikal must be locatable on PATH, in
// $OKAPI_HOME, or at $OKAPI_TIKAL. The bridge daemon is hard-required
// too — its acquire path inside parity.AcquireBridgeDaemon t.Fatals
// when it can't spawn — so we don't need a separate check here.
//
// Failing fast at TestMain means a missing dependency surfaces as a
// single clear error instead of every subtest skipping with the same
// message.
func TestMain(m *testing.M) {
	tikal := &roundtrip.TikalEngine{}
	if err := tikal.Available(); err != nil {
		fmt.Fprintf(os.Stderr,
			"parity round-trip: tikal is required and was not found.\n"+
				"  set OKAPI_TIKAL=/path/to/tikal.sh, OKAPI_HOME=/path/to/okapi,\n"+
				"  or place tikal on PATH. underlying error: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	parity.ShutdownBridgeDaemon()
	os.Exit(code)
}
