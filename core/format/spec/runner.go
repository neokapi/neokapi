package spec

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
)

// NativeRunner runs every Example in a Spec through a neokapi
// DataFormatReader and asserts the results match the spec's
// declarations. It's the always-on counterpart to the parity bridge
// runner: the spec describes WHAT the format does, NativeRunner
// verifies the Go reader honors that.
type NativeRunner struct {
	Spec *Spec

	// NewReader builds the reader for a given variant. The variant is
	// the empty string for monolithic formats. For multi-variant
	// formats (openxml, odf), implementors typically dispatch on the
	// variant id to set the appropriate ParseType / sub-reader.
	NewReader func(variant string) format.DataFormatReader
}

// Run drives every Feature × Example as a subtest. Each example is
// independent: failures don't cascade, the report shows per-example
// pass/fail.
func (r *NativeRunner) Run(t *testing.T) {
	t.Helper()
	if r.Spec == nil {
		t.Fatal("NativeRunner: Spec is nil")
	}
	if r.NewReader == nil {
		t.Fatal("NativeRunner: NewReader is nil")
	}
	for _, feat := range r.Spec.Features {
		feat := feat
		t.Run(feat.ID, func(t *testing.T) {
			for _, ex := range feat.Examples {
				ex := ex
				t.Run(ex.Name, func(t *testing.T) {
					r.runExample(t, feat, ex)
				})
			}
		})
	}
}

func (r *NativeRunner) runExample(t *testing.T, feat Feature, ex Example) {
	t.Helper()
	if ex.BridgeOnly {
		t.Skip("bridge_only example — skipped by native runner")
		return
	}
	input, err := ResolveInput(r.Spec, ex)
	if err != nil {
		// Skip cleanly when an upstream-testdata fixture isn't fetched
		// — the spec itself is fine, the corpus just isn't available.
		if strings.HasPrefix(ex.InputFile, "okapi:") {
			t.Skipf("input not available: %v", err)
			return
		}
		t.Fatalf("resolve input: %v", err)
	}
	reader := r.NewReader(ex.Variant)
	if reader == nil {
		t.Fatalf("NewReader returned nil for variant %q", ex.Variant)
	}

	cfg := MergeConfig(feat.Config, ex.Config)
	if len(cfg) > 0 {
		c := reader.Config()
		if c == nil {
			t.Fatalf("config provided but reader has no Config()")
		}
		if err := c.ApplyMap(cfg); err != nil {
			t.Fatalf("apply config %v: %v", cfg, err)
		}
	}

	parts, err := ReadParts(reader, input)
	if err != nil {
		if ex.ExpectedFail != "" {
			t.Logf("expected_fail (%s): read error %v", ex.ExpectedFail, err)
			return
		}
		t.Fatalf("read: %v", err)
	}
	failed := EvalAssertions(parts, ex.Assertions)
	if ex.ExpectedFail != "" {
		if len(failed) == 0 {
			t.Logf("expected_fail (%s): assertions now pass — remove the expected_fail tag", ex.ExpectedFail)
			return
		}
		for _, msg := range failed {
			t.Logf("expected_fail (%s): %s", ex.ExpectedFail, msg)
		}
		return
	}
	for _, msg := range failed {
		t.Errorf("%s", msg)
	}
}
