package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// A trial answers "what changes HERE". Folding main into the walk would report
// the workspace's diff under a branch's name.
func TestTrialFindings_WalksOnlyTheBoundStream(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addProject(&store.Project{ID: "proj2", Name: "Site", WorkspaceID: "ws"})
	bs.addStream("proj1", "trial/ban")
	bs.addBlocks("proj1", "main", srcBlock("m1", "g.md", "en-US", "Please use foobar on main"))
	bs.addBlocks("proj1", "trial/ban", srcBlock("t1", "g.md", "en-US", "Please use foobar on the branch"))
	bs.addBlocks("proj2", "main", srcBlock("o1", "g.md", "en-US", "foobar elsewhere entirely"))

	e := NewEngine(bs, tb, nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}

	rep, err := e.TrialFindings(ctx, "ws", ChangeSet{ID: "cs1"}, ops, "proj1", "trial/ban", EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, rep.TotalBlocks, "only the bound stream of the named project is scanned")
	assert.Equal(t, 1, rep.ChangedBlocks)
	require.Len(t, rep.Raised, 1)
	assert.Equal(t, "t1", rep.Raised[0].BlockID)
	assert.Equal(t, "trial/ban", rep.Stream)
	assert.Empty(t, rep.Cleared)
}

// The diff names the rule, not just a count — that is the whole difference
// between a trial and a blast radius.
func TestTrialFindings_NamesTheRuleOnBothSides(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("banned", term("kaputt", "en-US", model.TermForbidden))))
	require.NoError(t, tb.AddConcept(ctx, concept("fine", term("widget", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main", srcBlock("b1", "g.md", "en-US", "the kaputt widget"))

	e := NewEngine(bs, tb, nil)
	ops := []ChangeSetOp{
		// One term stops being forbidden…
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "banned", Locale: "en-US", Text: "kaputt",
			From: model.TermForbidden, To: model.TermAdmitted,
		}),
		// …and another starts.
		mustOp(t, 1, OpTermStatus, TermStatusPayload{
			ConceptID: "fine", Locale: "en-US", Text: "widget",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}

	rep, err := e.TrialFindings(ctx, "ws", ChangeSet{ID: "cs1"}, ops, "proj1", "main", EvalOptions{})
	require.NoError(t, err)

	require.Len(t, rep.Raised, 1)
	assert.Equal(t, "term", rep.Raised[0].Kind)
	assert.Equal(t, "widget", rep.Raised[0].Rule)
	assert.Equal(t, "fine", rep.Raised[0].ConceptID)
	assert.Equal(t, "g.md", rep.Raised[0].ItemName)

	require.Len(t, rep.Cleared, 1)
	assert.Equal(t, "kaputt", rep.Cleared[0].Rule)
	assert.Equal(t, "banned", rep.Cleared[0].ConceptID)

	assert.True(t, rep.TermsComputed, "the terms half is applied here, not resolved on the branch")
}

// A draft that changes nothing on this stream says so, rather than reporting an
// empty list that reads like a failure.
func TestTrialFindings_QuietStreamIsAnAnswer(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main", srcBlock("b1", "g.md", "en-US", "nothing to see here"))

	e := NewEngine(bs, tb, nil)
	rep, err := e.TrialFindings(ctx, "ws", ChangeSet{ID: "cs1"}, []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}, "proj1", "main", EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, rep.TotalBlocks)
	assert.Equal(t, 0, rep.ChangedBlocks)
	assert.NotNil(t, rep.Raised, "an empty diff marshals as [], never null")
	assert.NotNil(t, rep.Cleared)
}

// The lists are capped but the totals are not, so a truncated trial still says
// how much of the diff it is showing.
func TestTrialFindings_CapsTheListsAndKeepsTheTotals(t *testing.T) {
	ctx := context.Background()

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	for i := range 8 {
		bs.addBlocks("proj1", "main", srcBlock(string(rune('a'+i)), "g.md", "en-US", "a foobar here"))
	}

	e := NewEngine(bs, tb, nil)
	rep, err := e.TrialFindings(ctx, "ws", ChangeSet{ID: "cs1"}, []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		}),
	}, "proj1", "main", EvalOptions{MaxSamples: 3})
	require.NoError(t, err)

	assert.Len(t, rep.Raised, 3)
	assert.Equal(t, 8, rep.RaisedTotal)
	assert.Equal(t, 8, rep.ChangedBlocks)
}

func TestTrialFindings_RefusesAnUnboundTrial(t *testing.T) {
	e := NewEngine(newFakeBlockSource(), terms.NewInMemoryStore(), nil)
	_, err := e.TrialFindings(context.Background(), "ws", ChangeSet{ID: "cs1"}, nil, "proj1", "", EvalOptions{})
	require.Error(t, err)
}
