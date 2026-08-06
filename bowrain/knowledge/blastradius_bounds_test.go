package knowledge

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// The blast radius is the most expensive read in the platform: a walk over every
// stored block in a workspace. It used to have no bound and no way to say it had
// not finished — so on a 17k-block workspace the request ran until something
// upstream gave up, and the report it produced was shaped exactly like a clean
// one. These tests pin the three properties that make it safe: it stops, it says
// when it stopped early, and it does not pay per block for what varies per item.

// TestEvaluateChangeSet_BudgetExhaustedReportsPartial pins the difference
// between a clean radius and one that ran out of time. "0 blocks affected" and
// "0 blocks affected so far" support opposite merge decisions.
func TestEvaluateChangeSet_BudgetExhaustedReportsPartial(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	var blocks []*store.StoredBlock
	for i := range 50 {
		blocks = append(blocks, srcBlock(fmt.Sprintf("b%d", i), "guide.md", "en-US", "Please use foobar here"))
	}
	bs.addBlocks("proj1", "main", blocks...)

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en-US", Text: "foobar", From: model.TermAdmitted, To: model.TermForbidden}),
	}

	// A budget already spent: the walk stops at its first block.
	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{Budget: time.Nanosecond})
	require.NoError(t, err, "an exhausted budget is a partial answer, not a failure")
	require.NotNil(t, imp)
	assert.True(t, imp.Partial, "the report must say it is a floor")
	assert.NotEmpty(t, imp.PartialReason)
	assert.Less(t, imp.TotalBlocks, 50, "the walk stopped early")

	// With no budget the same walk completes and never claims to be partial.
	full, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)
	assert.False(t, full.Partial)
	assert.Equal(t, 50, full.TotalBlocks)
	assert.Equal(t, 50, full.AffectedBlocks)
}

// TestEvaluateChangeSet_CancelledContextIsAnError pins the other half: a caller
// that has gone away stops the walk, and is never told the workspace is clean.
func TestEvaluateChangeSet_CancelledContextIsAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main", srcBlock("b1", "guide.md", "en-US", "Anything at all"))

	e := NewEngine(bs, terms.NewInMemoryStore(), newFakeProfileStore(), nil)
	cancel()

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, nil, EvalOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, imp)
}

// TestEvaluateChangeSet_ResolvesEachCollectionOnce pins the cost of the walk.
// Collections resolve per item, and a project has far more blocks than items, so
// resolving per block meant two database round-trips for every block in the
// workspace — 35,000 sequential queries on the workspace that timed out.
func TestEvaluateChangeSet_ResolvesEachCollectionOnce(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	var blocks []*store.StoredBlock
	for i := range 40 {
		item := "a.md" // twenty blocks each in two items
		if i >= 20 {
			item = "b.md"
		}
		blocks = append(blocks, srcBlock(fmt.Sprintf("b%d", i), item, "en-US", "Some text"))
	}
	bs.addBlocks("proj1", "main", blocks...)

	e := NewEngine(bs, terms.NewInMemoryStore(), newFakeProfileStore(), nil)
	before := bs.itemLookups

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, nil, EvalOptions{})
	require.NoError(t, err)
	assert.Equal(t, 40, imp.TotalBlocks)
	assert.LessOrEqual(t, bs.itemLookups-before, 2,
		"one item lookup per distinct item, not one per block")
}
