package flow_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mutatingAnnotator returns an annotate tool that (illegally) edits the block
// source in place through the live run slice its read-only view hands back —
// exactly what the immutability hash backstop exists to catch.
func mutatingAnnotator() *tool.BaseTool {
	bt := &tool.BaseTool{ToolName: "sneaky-annotator"}
	bt.Annotate = func(v tool.BlockView) error {
		v.SourceRuns()[0].Text.Text = "changed"
		return nil
	}
	return bt
}

// runOneBlock pushes a single block through the executor's pipeline and
// returns the wait() error.
func runOneBlock(t *testing.T, e *flow.DefaultExecutor) error {
	t.Helper()
	f, err := flow.NewFlow("immutability").AddTool(mutatingAnnotator()).Build()
	require.NoError(t, err)

	in, out, wait := e.ExecuteWithChannels(t.Context(), f)
	go func() {
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello world")}
		close(in)
	}()
	for range out { //nolint:revive // drain the pipeline output
	}
	return wait()
}

func TestExecutorImmutabilityCheckOption(t *testing.T) {
	t.Run("enabled catches in-place source mutation", func(t *testing.T) {
		err := runOneBlock(t, flow.NewExecutor(flow.WithImmutabilityCheck(true)))
		require.ErrorContains(t, err, "immutability")
	})

	t.Run("disabled skips the hash backstop", func(t *testing.T) {
		err := runOneBlock(t, flow.NewExecutor(flow.WithImmutabilityCheck(false)))
		require.NoError(t, err)
	})

	t.Run("default seeds from the EnforceImmutability global at construction", func(t *testing.T) {
		//nolint:staticcheck // deliberate: this test pins the deprecated global's seeding behavior
		old := tool.EnforceImmutability
		//nolint:staticcheck // deliberate: this test pins the deprecated global's seeding behavior
		defer func() { tool.EnforceImmutability = old }()

		tool.EnforceImmutability = false //nolint:staticcheck // deliberate: production default (off)
		require.NoError(t, runOneBlock(t, flow.NewExecutor()),
			"with the global off, a default executor must not hash blocks")

		tool.EnforceImmutability = true //nolint:staticcheck // deliberate: compatibility knob still honoured
		err := runOneBlock(t, flow.NewExecutor())
		assert.ErrorContains(t, err, "immutability",
			"an embedding that sets the deprecated global keeps the backstop")
	})
}
