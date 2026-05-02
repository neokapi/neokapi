//go:build parity

package roundtrip

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
)

// Case is one round-trip scenario the harness drives end-to-end.
// The okapi engine is run inline as the comparator: every other
// engine's output is byte-compared against okapi's output for the
// same input + pseudo-spec, on the fly. Goldens are not committed —
// the okapi engine (the bridge launcher's `pseudo` subcommand) is a
// hard requirement.
type Case struct {
	// Name labels the case for sub-test names.
	Name string

	// FormatID is the neokapi format registry key. Used by the native
	// engine to look up reader+writer.
	FormatID registry.FormatID

	// Input is the source document.
	Input Input

	// Spec is the pseudo-translation transform (defaults to «»
	// when empty).
	Spec PseudoSpec

	// IsZip flips the comparator to per-entry zip mode for compound
	// archive formats (idml, openxml, epub, …) where byte-equality
	// across zip metadata is unrealistic.
	IsZip bool

	// ExpectedSkipped engines are not asserted on. Use this for known
	// real divergences (engine bug, semantic config gap) tracked
	// elsewhere — same role as `expected_fail` in spec.yaml.
	ExpectedSkipped []string
}

// RunThreeWay runs the okapi engine end-to-end to obtain the live
// reference output, then runs each tested engine and compares its
// output byte-for-byte (or per-zip-entry) against okapi's. The
// OkapiEngine is the comparator, not a tested engine — asserting
// okapi-against-itself would be meaningless.
//
// Engines listed in c.ExpectedSkipped are recorded as t.Skip; the
// remaining engines must produce output equivalent to okapi's. The
// harness reports every disagreement at once instead of bailing on
// the first.
func RunThreeWay(t *testing.T, c Case, native *NativeEngine, bridge *BridgeEngine, okapi *OkapiEngine) {
	t.Helper()
	if c.FormatID == "" {
		t.Fatal("RunThreeWay: Case.FormatID is required")
	}
	if c.Name == "" {
		t.Fatal("RunThreeWay: Case.Name is required")
	}
	if okapi == nil {
		t.Fatal("RunThreeWay: OkapiEngine is required (it is the comparator). For fixtures with no okapi support, skip the case at the call site.")
	}

	// The okapi engine is the comparator: produce the reference
	// output up front. Failures here mean either upstream Okapi can't
	// process this fixture or the fixture itself is malformed —
	// either way we can't proceed, and OkapiEngine.RoundTrip already
	// calls t.Fatalf with the diagnostic.
	reference := okapi.RoundTrip(t, c.Input, c.Spec)
	if len(reference) == 0 {
		t.Fatal("RunThreeWay: okapi engine produced empty reference output")
	}

	skipSet := map[string]bool{}
	for _, name := range c.ExpectedSkipped {
		skipSet[name] = true
	}

	type engineEntry struct {
		name string
		eng  Engine
	}
	entries := []engineEntry{}
	if native != nil {
		entries = append(entries, engineEntry{native.Name(), native})
	}
	if bridge != nil {
		entries = append(entries, engineEntry{bridge.Name(), bridge})
	}
	if len(entries) == 0 {
		t.Fatal("RunThreeWay: no engines configured")
	}

	var divergences []Divergence
	for _, e := range entries {
		e := e
		t.Run(e.name, func(t *testing.T) {
			if skipSet[e.name] {
				t.Skipf("engine %q intentionally skipped for this case", e.name)
			}
			out := e.eng.RoundTrip(t, c.Input, c.Spec)
			if reason := compareToReference(out, reference, c.IsZip); reason != "" {
				divergences = append(divergences, Divergence{
					Engine: e.name,
					Reason: reason,
				})
				t.Errorf("engine %q diverged from okapi reference: %s", e.name, reason)
			}
		})
	}

	if len(divergences) > 0 {
		var sb strings.Builder
		sb.WriteString("round-trip divergences vs okapi reference:\n")
		for _, d := range divergences {
			sb.WriteString("  - ")
			sb.WriteString(d.String())
			sb.WriteString("\n")
		}
		t.Log(sb.String())
	}
}
