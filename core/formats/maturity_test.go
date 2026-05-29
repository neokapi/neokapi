package formats_test

// This file is a structural maturity guardrail for format packages. It does not
// test behavior — it enforces the conventions that make a format's maturity
// mechanically checkable (see docs/internals/format-maturity.md). The goal is to
// stop NEW formats from being added below the floor; existing debt is tracked in
// explicit ledgers that should shrink over time, never grow.

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// nonFormats are directories under core/formats/ that are not real document
// formats (stubs / internal helpers) and are exempt from the rubric.
var nonFormats = map[string]bool{
	"exec":       true, // command-exec pseudo reader
	"jsx":        true, // klf-rename alias stub
	"memorytest": true, // in-memory test helper
}

// grandfatheredRoundtrip lists formats whose read->write fidelity coverage lives
// somewhere OTHER than a conventionally-named roundtrip_test.go / skeleton_test.go
// (e.g. inside reader_test.go or invariants_test.go), or — for `mo` — is genuine
// tracked debt. NEW formats MUST NOT be added here: put read->write fidelity
// tests in roundtrip_test.go or skeleton_test.go so the floor stays checkable.
// Removing an entry (by adding a conventionally-named test) is encouraged.
var grandfatheredRoundtrip = map[string]bool{
	"designtokens": true,
	"epub":         true,
	"i18next":      true,
	"idml":         true,
	"json":         true,
	"markdown":     true,
	"mo":           true,
	"odf":          true,
}

// realFormatDirs returns the format ids that have a reader.go and are not
// exempted non-formats.
func realFormatDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read core/formats: %v", err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() || nonFormats[e.Name()] {
			continue
		}
		if fileExists(filepath.Join(e.Name(), "reader.go")) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// TestFormatSpecIsGated enforces that every spec.yaml is exercised by a
// spec_test.go. An ungated spec rots silently (see format-engineering.md §8). This
// is a hard floor — there are currently zero violators, and new formats must keep
// it that way.
func TestFormatSpecIsGated(t *testing.T) {
	for _, id := range realFormatDirs(t) {
		if !fileExists(filepath.Join(id, "spec.yaml")) {
			continue
		}
		if !fileExists(filepath.Join(id, "spec_test.go")) {
			t.Errorf("format %q ships a spec.yaml but no spec_test.go — the spec is "+
				"not gated by any test. Add spec_test.go driving spec.NativeRunner "+
				"(see core/formats/properties/spec_test.go).", id)
		}
	}
}

// TestRoundTripTestNamingConvention enforces that a format with a writer carries
// its read->write fidelity test in a conventionally-named roundtrip_test.go or
// skeleton_test.go, so the maturity floor is mechanically checkable. Existing
// exceptions are grandfathered; new formats must follow the convention.
func TestRoundTripTestNamingConvention(t *testing.T) {
	for _, id := range realFormatDirs(t) {
		if !fileExists(filepath.Join(id, "writer.go")) {
			continue // read-only formats (e.g. pdf) have nothing to round-trip
		}
		conventional := fileExists(filepath.Join(id, "roundtrip_test.go")) ||
			fileExists(filepath.Join(id, "skeleton_test.go"))
		if conventional {
			if grandfatheredRoundtrip[id] {
				t.Logf("format %q now has a conventional round-trip test — remove it "+
					"from grandfatheredRoundtrip in maturity_test.go.", id)
			}
			continue
		}
		if grandfatheredRoundtrip[id] {
			continue // tracked debt / non-conventional coverage
		}
		t.Errorf("format %q has a writer.go but no roundtrip_test.go or "+
			"skeleton_test.go. Add a read->write fidelity test in one of those files "+
			"(see docs/internals/format-maturity.md, L1). Do not add it to the "+
			"grandfathered ledger.", id)
	}
}

// TestRobustnessCoverage is advisory: it reports formats lacking a malformed_test.go.
// Robustness against broken input is an L2 requirement (format-maturity.md), and
// today only a handful of formats have it. This does not fail the build — it
// surfaces the gap so it can be burned down.
func TestRobustnessCoverage(t *testing.T) {
	var missing []string
	for _, id := range realFormatDirs(t) {
		if !fileExists(filepath.Join(id, "malformed_test.go")) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		t.Logf("advisory: %d/%d formats lack a malformed_test.go (L2 robustness gap): %v",
			len(missing), len(realFormatDirs(t)), missing)
	}
}
