//go:build parity

package roundtrip

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
)

// Case is one round-trip scenario the harness drives end-to-end
// across each engine.
type Case struct {
	// Name labels the case for sub-test names.
	Name string

	// FormatID is the neokapi format registry key. The native engine
	// uses it to look up reader+writer; the comparator uses it to
	// re-extract every engine's output through the native reader.
	FormatID registry.FormatID

	// Input is the source document.
	Input Input

	// Spec is the pseudo-translation transform (defaults to «»
	// when empty).
	Spec PseudoSpec

	// ExpectedSkipped engines are not asserted on (e.g. mark
	// "tikal" when a format isn't known to the upstream Okapi
	// distribution and tikal would error out).
	ExpectedSkipped []string
}

// RunThreeWay drives Native, Bridge, and Tikal engines against the
// case in parallel sub-tests, then compares each engine's output
// against the expected wrapped Block stream. Engines that report
// Available()!=nil run as t.Skip; engines that participate must
// produce the expected stream.
//
// The harness reports one Divergence per engine that disagrees,
// instead of bailing on the first — so a failing test gives a
// complete picture of where the round-trip broke.
func RunThreeWay(t *testing.T, c Case, native *NativeEngine, bridge *BridgeEngine, tikal *TikalEngine) {
	t.Helper()
	if c.FormatID == "" {
		t.Fatal("RunThreeWay: Case.FormatID is required")
	}
	var readerConfig map[string]any
	if native != nil {
		readerConfig = native.ReaderConfig
	}
	expected, err := expectedTargets(c.FormatID, c.Input.Bytes, c.Spec, readerConfig)
	if err != nil {
		t.Fatalf("RunThreeWay: expected targets: %v", err)
	}
	if len(expected) == 0 {
		t.Fatalf("RunThreeWay: input has no translatable blocks; nothing to round-trip")
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
	if tikal != nil {
		entries = append(entries, engineEntry{tikal.Name(), tikal})
	}
	if len(entries) == 0 {
		t.Fatal("RunThreeWay: no engines configured")
	}

	var divergences []Divergence
	var ran []string
	for _, e := range entries {
		e := e
		t.Run(e.name, func(t *testing.T) {
			if skipSet[e.name] {
				t.Skipf("engine %q intentionally skipped for this case", e.name)
			}
			if err := e.eng.Available(); err != nil {
				t.Skipf("engine %q unavailable: %v", e.name, err)
			}
			out := e.eng.RoundTrip(t, c.Input, c.Spec)
			actual, err := extractedBlocks(c.FormatID, out, c.Spec.SrcLocale(), c.Spec.TgtLocale(), readerConfig)
			if err != nil {
				t.Fatalf("engine %q: re-extract output: %v", e.name, err)
			}
			ran = append(ran, e.name)
			if reason := reasonFor(expected, actual); reason != "" {
				divergences = append(divergences, Divergence{
					Engine:   e.name,
					Expected: expected,
					Actual:   actual,
					Reason:   reason,
				})
				t.Errorf("engine %q diverged: %s", e.name, reason)
			}
		})
	}

	if len(ran) == 0 {
		t.Skip("no engines participated")
	}
	if len(divergences) > 0 {
		var sb strings.Builder
		sb.WriteString("round-trip divergences:\n")
		for _, d := range divergences {
			sb.WriteString("  - ")
			sb.WriteString(d.String())
			sb.WriteString("\n")
		}
		t.Log(sb.String())
	}
}
