//go:build parity

package tools

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/core/model"
)

// TestParityTools iterates every entry in toolSpecs.
//
// Default behavior (Skip == "") feeds a single Block carrying source
// text "Hello world." through the bridge's ProcessStep RPC and asserts
// the daemon completes without error. This is a stability gate, not a
// correctness gate — most Okapi steps either pass parts through
// unchanged (counters, validators) or transform them in place
// (cleanup, code-simplifier).
//
// The test reports one row per step ID into the parity report so the
// dashboard's per-step status reflects the latest run.
func TestParityTools(t *testing.T) {
	for _, spec := range toolSpecs {
		spec := spec
		t.Run(strings.ReplaceAll(spec.ID, "-", "_"), func(t *testing.T) {
			runToolSpec(t, spec)
		})
	}
}

func runToolSpec(t *testing.T, spec ToolSpec) {
	t.Helper()
	defer parity.Report(t, parity.Outcome{
		Kind:   "step",
		ID:     spec.ID,
		Name:   t.Name(),
		Mode:   "bridge-only",
		Detail: spec.Skip,
	})

	if spec.Skip != "" {
		t.Skip(spec.Skip)
		return
	}

	input := []*model.Part{newSampleBlock("b1", "Hello world.")}
	out := parity.RunBridgeStep(t, parity.StepRequest{
		StepClass:  spec.ID,
		StepParams: spec.StepParams,
		Parts:      input,
	})
	// Bridge-only baseline: the daemon must complete without erroring.
	// Many counter/validator steps emit zero downstream parts (they
	// accumulate state internally), so the stream-length assertion
	// stays loose; RunBridgeStep already fails the test on RPC errors.
	_ = out
}

// newSampleBlock constructs a minimal Block carrying English source
// text — the universal fixture used by Phase C's bridge-only stability
// runs.
func newSampleBlock(id, text string) *model.Part {
	return &model.Part{
		Type: model.PartBlock,
		Resource: &model.Block{
			ID:           id,
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
		},
	}
}
