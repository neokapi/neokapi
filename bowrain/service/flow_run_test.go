package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/flowdef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFlowRunFixture seeds a SQLite content store with one project holding two
// items, and returns a flow service over it with the built-in tools registered.
func newFlowRunFixture(t *testing.T) (*FlowService, store.ContentStore, *recordingTracker) {
	t.Helper()
	ctx := context.Background()

	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    "p1",
		Name:                  "Flow run",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "a.json", []*model.Block{
		translatableBlock("a1", "Hello there."),
		translatableBlock("a2", "Good morning."),
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "b.json", []*model.Block{
		translatableBlock("b1", "Goodbye."),
	}))

	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	fs := NewFlowService(cs, nil, reg)
	rec := &recordingTracker{}
	fs.tracker = rec
	return fs, cs, rec
}

func translatableBlock(id, text string) *model.Block {
	b := model.NewBlock(id, text)
	b.Translatable = true
	return b
}

func builtInFlow(t *testing.T, id string) *flow.FlowDefinition {
	t.Helper()
	for _, def := range flowdef.BuiltInFlows() {
		if def.ID == id {
			d := def
			return &d
		}
	}
	t.Fatalf("built-in flow %q not found", id)
	return nil
}

func itemBlocks(t *testing.T, cs store.ContentStore, item string) []*model.Block {
	t.Helper()
	stored, err := cs.GetBlocks(context.Background(), store.BlockQuery{ProjectID: "p1", Stream: "main", ItemName: item})
	require.NoError(t, err)
	out := make([]*model.Block, 0, len(stored))
	for _, sb := range stored {
		out = append(out, sb.Block)
	}
	return out
}

// TestFlowServiceRunFlow_WritesEveryPassBack proves a run streams each item's
// blocks through the flow once per target locale and persists the result:
// the pseudo-translate flow leaves a marked target for every locale on every
// block of every item.
func TestFlowServiceRunFlow_WritesEveryPassBack(t *testing.T) {
	fs, cs, rec := newFlowRunFixture(t)

	res, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    builtInFlow(t, "pseudo-translate"),
		ProjectID:     "p1",
		TargetLocales: []string{"fr", "de"},
		Source:        "test",
	})
	require.NoError(t, err)
	assert.Equal(t, FlowRunResult{Items: 2, Passes: 4, Blocks: 3}, res)

	for _, item := range []string{"a.json", "b.json"} {
		blocks := itemBlocks(t, cs, item)
		require.NotEmpty(t, blocks, item)
		for _, b := range blocks {
			for _, locale := range []model.LocaleID{"fr", "de"} {
				target := b.TargetText(locale)
				assert.Contains(t, target, "▒", "block %s of %s has no pseudo target for %s", b.ID, item, locale)
			}
		}
	}

	ev := rec.find(analytics.EventFlowRunCompleted)
	require.NotNil(t, ev, "flow_run_completed not captured")
	assert.Equal(t, "pseudo-translate", ev.props["flow"])
	assert.Equal(t, "completed", ev.props["outcome"])
	assert.Equal(t, "test", ev.props["source"])
	assert.Equal(t, 3, ev.props["part_count"])
	assert.Equal(t, "p1", ev.distinctID, "no actor falls back to the project")
}

// TestFlowServiceRunFlow_ScopesToNamedItems proves Items limits the run to
// the items named, leaving the others untouched.
func TestFlowServiceRunFlow_ScopesToNamedItems(t *testing.T) {
	fs, cs, _ := newFlowRunFixture(t)

	res, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    builtInFlow(t, "pseudo-translate"),
		ProjectID:     "p1",
		Items:         []string{"b.json"},
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)
	assert.Equal(t, FlowRunResult{Items: 1, Passes: 1, Blocks: 1}, res)

	for _, b := range itemBlocks(t, cs, "a.json") {
		assert.Empty(t, b.TargetText("fr"), "a.json was outside the run's scope")
	}
	for _, b := range itemBlocks(t, cs, "b.json") {
		assert.Contains(t, b.TargetText("fr"), "▒")
	}
}

// TestFlowServiceRunFlow_CheckFlowReadsStoredTargets proves the content store
// is the source binding for the data-flow gate: a flow that opens on a check
// consuming targets passes because the store carries them.
func TestFlowServiceRunFlow_CheckFlowReadsStoredTargets(t *testing.T) {
	fs, _, _ := newFlowRunFixture(t)
	ctx := context.Background()

	_, err := fs.RunFlow(ctx, FlowRun{
		Definition:    builtInFlow(t, "pseudo-translate"),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)

	res, err := fs.RunFlow(ctx, FlowRun{
		Definition:    builtInFlow(t, "qa"),
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Items)
}

// TestFlowServiceRunFlow_UnknownToolFailsBeforeReading proves a flow naming a
// tool the registry lacks fails the run with that tool named, and writes
// nothing.
func TestFlowServiceRunFlow_UnknownToolFailsBeforeReading(t *testing.T) {
	fs, cs, rec := newFlowRunFixture(t)

	def := &flow.FlowDefinition{
		ID:   "broken",
		Name: "Broken",
		Nodes: []flow.FlowNode{
			{ID: "x", Type: flow.NodeTool, Name: "no-such-tool"},
		},
	}
	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    def,
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-tool")

	for _, b := range itemBlocks(t, cs, "a.json") {
		assert.Empty(t, b.TargetText("fr"))
	}
	ev := rec.find(analytics.EventFlowRunCompleted)
	require.NotNil(t, ev)
	assert.Equal(t, "failed", ev.props["outcome"])
}

// TestFlowServiceRunFlow_RejectsIncompleteRequests covers the inputs a run
// cannot start without.
func TestFlowServiceRunFlow_RejectsIncompleteRequests(t *testing.T) {
	fs, _, _ := newFlowRunFixture(t)
	ctx := context.Background()

	_, err := fs.RunFlow(ctx, FlowRun{ProjectID: "p1"})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "flow definition"), err.Error())

	_, err = fs.RunFlow(ctx, FlowRun{Definition: builtInFlow(t, "qa")})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "project id"), err.Error())

	bare := NewFlowService(nil, nil, nil)
	_, err = bare.RunFlow(ctx, FlowRun{Definition: builtInFlow(t, "qa"), ProjectID: "p1"})
	require.Error(t, err)
}
