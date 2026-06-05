//go:build parity

package roundtrip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
)

// Case is one round-trip scenario the harness drives end-to-end.
// The okapi engine is run inline as the comparator: every other
// engine's output is compared against okapi's output for the same
// input + pseudo-spec, on the fly. Goldens are not committed —
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

	// ExpectedSkipped engines are not asserted on. Use this for cases
	// where the engine literally cannot run on this fixture (filter
	// missing, intentionally-malformed input, etc.). Recorded as
	// "skipped" in the parity report.
	ExpectedSkipped []string

	// SkipReason is the human-readable note explaining why
	// ExpectedSkipped engines were skipped. Recorded in the parity
	// report so we can audit the skip list later.
	SkipReason string

	// Normalizer, when non-nil, is run on both engine output and
	// reference before the canonical-tier comparison. nil = byte-only
	// comparison (engine reaches at most TierByteEqual or TierDivergent).
	Normalizer Normalizer

	// MinTier maps engine name → required minimum tier. Engines that
	// reach a stricter (= numerically lower) tier pass; engines that
	// reach a looser tier fail. Engines without an entry default to
	// TierByteEqual (the strictest contract).
	//
	// Use this to grade an engine on a "must reach canonical-equal"
	// contract while still surfacing actual achievement (byte vs
	// canonical) in the parity report.
	MinTier map[string]Tier

	// CanonClass classifies any canonical-equal outcome for this case as
	// "faithful" (native preserves source; okapi re-serializes — expected,
	// don't chase) or "closeable" (native loses source info — real work).
	// Defaults to CanonUnclassified, which never inflates the
	// faithful-parity figure. Declared per-format on the coverage scan.
	CanonClass CanonClass
}

// RunThreeWay runs the okapi engine end-to-end to obtain the live
// reference output, then runs each tested engine and compares its
// output against okapi's via the tier mechanic. The OkapiEngine is
// the comparator, not a tested engine — asserting okapi-against-itself
// would be meaningless.
//
// Engines listed in c.ExpectedSkipped are recorded as t.Skip and as
// "skipped" in the parity report. Engines that run must reach their
// MinTier (default TierByteEqual). The harness reports every
// disagreement at once instead of bailing on the first.
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

	formatID := string(c.FormatID)
	// Annotation is per-(format, fixture), shared across engines.
	// Looked up once per Case so the YAML loader sees one hit per
	// fixture instead of one per engine.
	var ann *Annotation
	if a, ok := LookupAnnotation(formatID, c.Name); ok {
		ann = &a
	}

	var divergences []Divergence
	for _, e := range entries {

		t.Run(e.name, func(t *testing.T) {
			required := requiredTier(c.MinTier, e.name)
			if skipSet[e.name] {
				recordParityResult(parityRecord{
					Format:     formatID,
					Fixture:    c.Name,
					Engine:     e.name,
					Required:   required,
					Skipped:    true,
					SkipMsg:    c.SkipReason,
					Annotation: ann,
				})
				t.Skipf("engine %q intentionally skipped for this case: %s", e.name, c.SkipReason)
			}
			out := e.eng.RoundTrip(t, c.Input, c.Spec)
			result := compareTiered(out, reference, c.IsZip, c.Normalizer)
			// When PARITY_DUMP=<dir> is set, write got + reference to
			// disk for any divergent fixture so a human can run their
			// own diff tools (`diff`, `vimdiff`, hex editors, …).
			// Byte-equal and canonical-equal cases are skipped — there's
			// nothing to inspect.
			if dir := os.Getenv("PARITY_DUMP"); dir != "" && result.Achieved == TierDivergent {
				dumpDivergentArtifacts(dir, formatID, c.Name, e.name, out, reference)
				if c.Normalizer != nil {
					if gn, err := c.Normalizer.Normalize(out); err == nil {
						_ = os.WriteFile(filepath.Join(dir, formatID, c.Name+"."+e.name+".norm.bin"), gn, 0o644)
					}
					if rn, err := c.Normalizer.Normalize(reference); err == nil {
						_ = os.WriteFile(filepath.Join(dir, formatID, c.Name+".reference.norm.bin"), rn, 0o644)
					}
				}
			}
			recordParityResult(parityRecord{
				Format:         formatID,
				Fixture:        c.Name,
				Engine:         e.name,
				Required:       required,
				Achieved:       result.Achieved,
				Reason:         result.Reason,
				GotSize:        result.GotSize,
				RefSize:        result.RefSize,
				RawDiffOffset:  result.RawDiffOffset,
				NormDiffOffset: result.NormDiffOffset,
				Normalizer:     result.Normalizer,
				CanonClass:     c.CanonClass,
				Annotation:     ann,
			})
			if result.Achieved > required {
				reason := result.Reason
				if c.Normalizer != nil && result.NormDiffOffset >= 0 {
					reason = result.Reason + "; normalized diff at offset " + itoa(result.NormDiffOffset)
				}
				divergences = append(divergences, Divergence{
					Engine: e.name,
					Reason: reason,
				})
				t.Errorf("engine %q reached %s, required %s: %s",
					e.name, result.Achieved, required, reason)
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

// requiredTier returns the engine's required minimum tier. Per-Case
// overrides win; otherwise the per-engine default kicks in.
//
// Engine defaults reflect the current contract:
//   - bridge → TierByteEqual (must match okapi byte-for-byte)
//   - native → TierDivergent (observation mode — we collect tier data
//     in the report but don't fail on stylistic gaps until a per-format
//     normalizer or writer fix is in place)
//   - other → TierByteEqual
//
// Tighten a per-format native contract by setting minTier explicitly.
func requiredTier(min map[string]Tier, engine string) Tier {
	if min != nil {
		if t, ok := min[engine]; ok {
			return t
		}
	}
	if engine == "native" {
		return TierDivergent
	}
	return TierByteEqual
}

// dumpDivergentArtifacts writes the engine output and the reference
// for one divergent fixture to <dir>/<format>/<fixture>.<engine>.bin
// and <dir>/<format>/<fixture>.reference.bin. Failures are silent —
// dumping is opt-in diagnostic plumbing, not a contract.
func dumpDivergentArtifacts(dir, format, fixture, engine string, got, ref []byte) {
	target := filepath.Join(dir, format)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return
	}
	safe := strings.ReplaceAll(fixture, string(os.PathSeparator), "_")
	_ = os.WriteFile(filepath.Join(target, safe+"."+engine+".bin"), got, 0o644)
	_ = os.WriteFile(filepath.Join(target, safe+".reference.bin"), ref, 0o644)
}

// itoa is a tiny strconv.Itoa to avoid pulling in the strconv import
// just for diagnostic strings.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
