//go:build parity

package roundtrip

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
)

// Case is one round-trip scenario the harness drives end-to-end.
// Tikal is run inline as the comparator: every other engine's
// output is byte-compared against tikal's output for the same
// input + pseudo-spec, on the fly. Goldens are not committed —
// tikal is a hard requirement.
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

// RunThreeWay runs tikal end-to-end to obtain the live reference
// output, then runs each tested engine and compares its output
// byte-for-byte (or per-zip-entry) against tikal's. The TikalEngine
// is the comparator, not a tested engine — asserting tikal-against-
// itself would be meaningless.
//
// Engines listed in c.ExpectedSkipped are recorded as t.Skip; the
// remaining engines must produce output equivalent to tikal's. The
// harness reports every disagreement at once instead of bailing on
// the first.
func RunThreeWay(t *testing.T, c Case, native *NativeEngine, bridge *BridgeEngine, tikal *TikalEngine) {
	t.Helper()
	if c.FormatID == "" {
		t.Fatal("RunThreeWay: Case.FormatID is required")
	}
	if c.Name == "" {
		t.Fatal("RunThreeWay: Case.Name is required")
	}
	if tikal == nil {
		t.Fatal("RunThreeWay: TikalEngine is required (tikal is the comparator). For fixtures with no tikal support, skip the case at the call site.")
	}

	// Tikal is the comparator: produce the reference output up front.
	// Failures here mean either tikal is broken on this fixture or the
	// fixture itself is malformed — either way we can't proceed, and
	// tikal.RoundTrip already calls t.Fatalf with the diagnostic.
	reference := tikal.RoundTrip(t, c.Input, c.Spec)
	if len(reference) == 0 {
		t.Fatal("RunThreeWay: tikal produced empty reference output")
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
				t.Errorf("engine %q diverged from tikal: %s", e.name, reason)
			}
		})
	}

	if len(divergences) > 0 {
		var sb strings.Builder
		sb.WriteString("round-trip divergences vs tikal reference:\n")
		for _, d := range divergences {
			sb.WriteString("  - ")
			sb.WriteString(d.String())
			sb.WriteString("\n")
		}
		t.Log(sb.String())
	}
}
