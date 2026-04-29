//go:build parity

package formats

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
)

// TestParityFormats iterates every entry in formatSpecs.
//
// For each filter:
//   - Spec.Skip set       → record skip row, mode=bridge-only.
//   - Spec.NewReader nil  → bridge-only stability run; assert the bridge
//                            produces a non-empty stream and record pass.
//   - Spec.NewReader set  → head-to-head: run both, CompareBlockText.
//
// The test reports one row per filter ID into the parity report so the
// dashboard's per-filter status reflects the latest run.
func TestParityFormats(t *testing.T) {
	for _, spec := range formatSpecs {
		spec := spec
		t.Run(strings.TrimPrefix(spec.ID, "okf_"), func(t *testing.T) {
			runFormatSpec(t, spec)
		})
	}
}

func runFormatSpec(t *testing.T, spec FormatSpec) {
	t.Helper()
	mode := "head-to-head"
	if spec.NewReader == nil {
		mode = "bridge-only"
	}
	defer parity.Report(t, parity.Outcome{
		Kind:   "format",
		ID:     spec.ID,
		Name:   t.Name(),
		Mode:   mode,
		Detail: spec.Skip,
	})

	if spec.Skip != "" {
		t.Skip(spec.Skip)
		return
	}
	if len(spec.Inputs) == 0 {
		t.Skip("no sample inputs declared for " + spec.ID)
		return
	}

	for _, in := range spec.Inputs {
		in := in
		t.Run(in.Name, func(t *testing.T) {
			bridge := parity.RunBridge(t, parity.BridgeRequest{
				FilterClass:  spec.ID,
				InputBytes:   in.Content,
				MimeType:     spec.MimeType,
				FilterParams: spec.FilterArgs,
			})
			if len(bridge) == 0 {
				t.Fatalf("bridge returned no parts for %s/%s", spec.ID, in.Name)
			}

			if spec.NewReader == nil {
				// Bridge-only: just verify the daemon produced a stable
				// non-empty stream.
				return
			}
			native := parity.RunNative(t, parity.NativeRequest{
				NewReader:  spec.NewReader,
				InputBytes: in.Content,
				MimeType:   spec.MimeType,
				URI:        "test." + spec.ID,
			})
			parity.CompareBlockText(t, native, bridge)
		})
	}

	// Round-trip pass: only when both implementations have a writer
	// declared. Reports under Kind="format-roundtrip" so the
	// contract-audit dashboard can show read parity and round-trip
	// parity side-by-side without one masking the other.
	if spec.NewWriter != nil && spec.NewReader != nil {
		runRoundTripSpec(t, spec)
	}
}

// runRoundTripSpec drives a read → write pass on both sides for every
// input and compares the resulting output bytes. A divergence here
// indicates a writer-side regression invisible to read-only parity.
func runRoundTripSpec(t *testing.T, spec FormatSpec) {
	t.Helper()
	t.Run("roundtrip", func(t *testing.T) {
		defer parity.Report(t, parity.Outcome{
			Kind:   "format-roundtrip",
			ID:     spec.ID,
			Name:   t.Name(),
			Mode:   "round-trip",
			Detail: spec.SkipRoundTrip,
		})
		if spec.SkipRoundTrip != "" {
			t.Skip(spec.SkipRoundTrip)
			return
		}
		for _, in := range spec.Inputs {
			in := in
			t.Run(in.Name, func(t *testing.T) {
				native := parity.RunNativeRoundTrip(t, parity.NativeRoundTripRequest{
					NewReader:  spec.NewReader,
					NewWriter:  spec.NewWriter,
					InputBytes: in.Content,
					MimeType:   spec.MimeType,
					URI:        "test." + spec.ID,
				})
				bridge := parity.RunBridgeRoundTrip(t, parity.BridgeRequest{
					FilterClass:  spec.ID,
					InputBytes:   in.Content,
					MimeType:     spec.MimeType,
					FilterParams: spec.FilterArgs,
				})
				parity.CompareBytes(t, bridge.Output, native.Output)
			})
		}
	})
}
