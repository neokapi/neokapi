package spec

import (
	"errors"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// oracle.go exposes the differential-oracle hook the case-gen ritual needs
// (format-spec-cases.md §10 step 4 — "Adjudicate"). It is the native side of
// the §4.3 shim contract: given a candidate case, drive the native reader and
// return the observed result as a canonical block-event dump. The parity
// ParityRunner (cli/parity/spec, build tag "parity") runs the SAME
// Spec/Example through the okapi-bridge daemon and calls DumpBlockEvents on
// the bridge parts, so the two CaseResult dumps compare structurally —
// both-agree → candidate-pass, disagree → divergence triage, both-reject →
// promote to class: invalid. No model prediction enters the loop: assertion
// values are *observed* by these runners, never authored.
//
// The external-port / WASM shim (§4.3) wraps the same two calls: read the
// native payload, ReadParts via the registry-resolved reader, then
// DumpBlockEvents → stdout (exit 1 + stderr category on rejection). A thin
// `kapi spec-dump` CLI command is the intended packaging of that shim; it is
// deliberately left out of this change (it needs registry/config plumbing and
// the in-repo kapi isolation contract) — DumpBlockEvents + RunNativeCase are
// the load-bearing pieces it would call.

// CaseResult is the observed outcome of running one case through an engine
// (native here; the bridge side mirrors it). It is the differential-oracle
// comparison unit.
type CaseResult struct {
	// BlockEvents is the canonical block-event dump (§4) of the read
	// parts. Empty when Err is set.
	BlockEvents []byte
	// Extracted is the source-text view (BlockTexts) — the cheap
	// first-pass comparison before the structural dump.
	Extracted []string
	// Parts is the raw stream, for callers that want to run additional
	// views (roundtrip, overlay inspection) without re-reading.
	Parts []*model.Part
	// Err is non-nil when the engine rejected the input (the §3
	// invalid-class signal).
	Err error
}

// RunNativeCase drives the native reader for one case and returns the observed
// CaseResult. cfg is the already-merged config (typically
// MergeConfig(feature.Config, example.Config)); pass nil for defaults.
//
// This is a pure, test-free entrypoint (no testing dependency) so the
// case-gen ritual and any tooling can call it directly, exactly as the
// NativeRunner does internally.
func RunNativeCase(s *Spec, ex Example, cfg map[string]any, newReader func(variant string) format.DataFormatReader) CaseResult {
	input, err := ResolveInput(s, ex)
	if err != nil {
		return CaseResult{Err: err}
	}
	reader := newReader(ex.Variant)
	if reader == nil {
		return CaseResult{Err: errors.New("spec: newReader returned nil")}
	}
	if len(cfg) > 0 {
		if c := reader.Config(); c != nil {
			if err := c.ApplyMap(cfg); err != nil {
				return CaseResult{Err: err}
			}
		}
	}
	parts, err := ReadParts(reader, input)
	if err != nil {
		return CaseResult{Err: err}
	}
	dump, derr := DumpBlockEvents(parts)
	if derr != nil {
		return CaseResult{Parts: parts, Extracted: BlockTexts(parts), Err: derr}
	}
	return CaseResult{
		BlockEvents: dump,
		Extracted:   BlockTexts(parts),
		Parts:       parts,
	}
}
