// Package spectest provides test helpers for driving a format Spec
// through a neokapi DataFormatReader. It is a sibling to core/format/spec
// and lives in a separate package so that importing testing does not taint
// the core/format/spec package's normal (non-test) build graph.
//
// Callers import this package only from *_test.go files:
//
//	import "github.com/neokapi/neokapi/core/format/spectest"
package spectest

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
)

// NativeRunner runs every Example in a Spec through a neokapi
// DataFormatReader and asserts the results match the spec's
// declarations. It's the always-on counterpart to the parity bridge
// runner: the spec describes WHAT the format does, NativeRunner
// verifies the Go reader honors that.
type NativeRunner struct {
	Spec *spec.Spec

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
		t.Run(feat.ID, func(t *testing.T) {
			for _, ex := range feat.Examples {
				t.Run(ex.Name, func(t *testing.T) {
					r.runExample(t, feat, ex)
				})
			}
		})
	}
}

func (r *NativeRunner) runExample(t *testing.T, feat spec.Feature, ex spec.Example) {
	t.Helper()
	if ex.BridgeOnly {
		t.Skip("bridge_only example — skipped by native runner")
		return
	}
	input, err := spec.ResolveInput(r.Spec, ex)
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

	cfg := spec.MergeConfig(feat.Config, ex.Config)
	if len(cfg) > 0 {
		c := reader.Config()
		if c == nil {
			t.Fatalf("config provided but reader has no Config()")
		}
		if err := c.ApplyMap(cfg); err != nil {
			t.Fatalf("apply config %v: %v", cfg, err)
		}
	}

	parts, err := spec.ReadParts(reader, input)
	if err != nil {
		if ex.ExpectedFail != "" {
			t.Logf("expected_fail (%s): read error %v", ex.ExpectedFail, err)
			return
		}
		t.Fatalf("read: %v", err)
	}
	failed := spec.EvalAssertions(parts, ex.Assertions)
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
