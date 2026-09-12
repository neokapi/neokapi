// Package canary is the subject every Prose probe run must refuse.
//
// Each test is a way a rung could be awarded without evidence. The probe
// scores this package on every run and requires P0 with exactly the outcomes
// below; any other scoring makes the run invalid. Under an ordinary
// `go test ./...` every test here skips, so the deliberately failing one never
// reaches a build.
package canary

import (
	"os"
	"testing"
)

// underProbe skips outside a probe run, where PROSE_PROBE_CANARY is set.
func underProbe(t *testing.T) {
	t.Helper()
	if os.Getenv("PROSE_PROBE_CANARY") == "" {
		t.Skip("canary: runs only under the Prose probe")
	}
}

// TestProseP0_canary is the canary's presence test. It passes under the probe,
// so the canary is present and its rungs are judged.
func TestProseP0_canary(t *testing.T) {
	underProbe(t)
}

// TestProseP1_canary skips. A skip reports as green to `go test` and must come
// to did-not-run.
func TestProseP1_canary(t *testing.T) {
	t.Skip("canary: a skipped rung test did not run")
}

// TestProseP2_canary passes with no P1 beneath it. The rung is met and the
// level must still be P0.
func TestProseP2_canary(t *testing.T) {
	underProbe(t)
}

// TestProseP3_canary fails and must come to not-met.
func TestProseP3_canary(t *testing.T) {
	underProbe(t)
	t.Error("canary: a failing rung test is not met")
}

// TestProseP4_canary passes only because its one subtest skipped. `go test`
// reports the parent as pass; it must come to did-not-run.
func TestProseP4_canary(t *testing.T) {
	underProbe(t)
	t.Run("only-case", func(t *testing.T) {
		t.Skip("canary: a parent whose every subtest skipped checked nothing")
	})
}
