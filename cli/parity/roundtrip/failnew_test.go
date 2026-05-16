//go:build parity

package roundtrip_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
)

// TestParityFailNew is the CI gate that refuses merges introducing
// new parity divergences without documentation. Every divergent
// (format, fixture) seen by TestRoundTrip_Coverage must have a
// matching entry in core/formats/<format>/parity-annotations.yaml.
//
// Grandfathering is intentional: existing divergences pass because
// they ARE annotated (with severity bug, cosmetic, native-more-correct,
// or fixture-bug). Clearing one means deleting the annotation entry
// after the underlying bug is fixed and the test goes green.
//
// The test runs after TestRoundTrip_Coverage (alphabetic file order
// puts failnew_test.go after coverage_test.go). When parity records
// are empty (e.g. only this test was selected via -run), the gate
// skips — its signal is meaningful only in combination with a real
// coverage run.
//
// Set PARITY_FAIL_NEW=0 to downgrade the failure to a warning. This
// exists for the bootstrap moment when the annotation set is being
// extended; in CI, leave it unset (failures are the point).
func TestParityFailNew(t *testing.T) {
	div := roundtrip.UnannotatedDivergences()
	if len(div) == 0 {
		return
	}
	var sb strings.Builder
	noun := "fixtures"
	if len(div) == 1 {
		noun = "fixture"
	}
	fmt.Fprintf(&sb,
		"parity fail-new gate: %d %s reached divergent tier without an annotation. "+
			"Add an entry to core/formats/<format>/parity-annotations.yaml documenting "+
			"WHY it diverges and what would clear it.\n\n",
		len(div), noun)
	for _, d := range div {
		fmt.Fprintf(&sb, "  - %s/%s (engine=%s)\n", d.Format, d.Fixture, d.Engine)
	}
	sb.WriteString("\nSeverity guide: bug = real correctness defect; cosmetic = renders identically; native-more-correct = native beats Okapi per spec; fixture-bug = upstream fixture itself unusable.")

	if os.Getenv("PARITY_FAIL_NEW") == "0" {
		t.Log("WARNING: " + sb.String())
		return
	}
	t.Fatal(sb.String())
}
