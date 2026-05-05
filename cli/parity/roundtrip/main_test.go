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
// before any test runs: the okapi-bridge launcher must be installed
// in the parity sandbox (via `make parity-test`). The bridge daemon
// is hard-required too — its acquire path inside
// parity.AcquireBridgeDaemon t.Fatals when it can't spawn — so we
// don't need a separate check here.
//
// Failing fast at TestMain means a missing dependency surfaces as a
// single clear error instead of every subtest skipping with the same
// message.
func TestMain(m *testing.M) {
	okapi := &roundtrip.OkapiEngine{FilterClass: "okf_plaintext"}
	if err := okapi.Available(); err != nil {
		fmt.Fprintf(os.Stderr,
			"parity round-trip: okapi-bridge launcher is required and was not found.\n"+
				"  build the parity sandbox with `make parity-test` from the repo root.\n"+
				"  underlying error: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	parity.ShutdownBridgeDaemon()

	// Always emit the parity report to stderr so a normal `go test`
	// run shows the per-engine tier histogram without extra plumbing.
	// When PARITY_REPORT is set, also write a Markdown copy to the
	// given path for committing or further analysis.
	_ = roundtrip.FlushParityReport(os.Stderr)
	if path := os.Getenv("PARITY_REPORT"); path != "" {
		if f, err := os.Create(path); err == nil {
			_ = roundtrip.FlushParityReport(f)
			_ = f.Close()
		} else {
			fmt.Fprintf(os.Stderr, "parity report: could not write %s: %v\n", path, err)
		}
	}
	// PARITY_FIXTURES_JSON dumps per-fixture divergence detail as JSON
	// for the /parity/fixtures Docusaurus dashboard. Same data the
	// Markdown drill-down carries, machine-readable.
	if path := os.Getenv("PARITY_FIXTURES_JSON"); path != "" {
		if f, err := os.Create(path); err == nil {
			_ = roundtrip.FlushParityFixturesJSON(f)
			_ = f.Close()
		} else {
			fmt.Fprintf(os.Stderr, "parity fixtures: could not write %s: %v\n", path, err)
		}
	}

	os.Exit(code)
}
