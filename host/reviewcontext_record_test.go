package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// TestReviewContext_ProvenanceReadsTheBlockHistory pins the fold of an edit
// made through the change service into the review model: the edit is a
// content.edit in the block history rather than a ledger entry, so the
// provenance names its writer when the recorded change produced the content
// in force, and a decision that judged text no longer there is not in force.
func TestReviewContext_ProvenanceReadsTheBlockHistory(t *testing.T) {
	f := newCommitFixture(t)
	f.app.InitRegistries()
	ctx := t.Context()
	const doc = "docs/guide.md"
	source := filepath.Join(f.root, doc)

	svc, err := f.app.ChangeService(ctx, ChangeServiceOptions{Project: f.recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	p := page.Blocks[len(page.Blocks)-1]
	at := p.Ref
	at.Edition = editionKey(t, "fr")

	unit := func(t *testing.T) ReviewContextRequest {
		t.Helper()
		blocks, err := f.app.ReadBlocksForCheck(ctx, source, "", nil, "en")
		require.NoError(t, err)
		target, err := f.app.ReadBlocksForCheck(ctx, filepath.Join(f.root, "docs", "fr", "guide.md"), "", nil, "en")
		require.NoError(t, err)
		OverlayTargets(blocks, target, "fr")
		return ReviewContextRequest{Cmd: f.command(t), Root: f.root, SourcePath: source,
			Locale: "fr", SourceLang: "en", Blocks: blocks, Key: p.Ref.Block}
	}
	edit := func(t *testing.T, actor change.Actor, ifMatch, text string) {
		t.Helper()
		res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, ifMatch, text)}}, actor)
		require.NoError(t, err)
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	}
	aiDraft := func(text string) *state.UnitState {
		return &state.UnitState{Unit: p.Ref.Block, Variant: model.Variant("fr"), Status: model.TargetStatusEstablished,
			Origin:     model.Origin{Kind: model.OriginAI, Engine: "claude", Timestamp: "2026-09-01T10:00:00Z"},
			TargetHash: state.TargetHash(text),
			Decision:   state.Decision{ReviewState: "approved", By: "ada"}}
	}

	edit(t, change.Actor{Kind: change.ActorPerson, Name: "asgeir"}, model.AbsentRevision, "Nous utilisons le gadget chaque jour.")

	tests := []struct {
		name       string
		unit       *state.UnitState
		wantOrigin string
		wantEngine string
		wantState  string
	}{
		{"a person's edit is the origin, and a decision on text that is gone is not in force",
			aiDraft("Le gadget, chaque jour."), model.OriginHuman, "", ""},
		{"a decision that judged the content in force stays in force",
			aiDraft("Nous utilisons le gadget chaque jour."), model.OriginHuman, "", "approved"},
		{"with no ledger record the history still names the writer", nil, model.OriginHuman, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := unit(t)
			req.Unit = tc.unit
			rc := f.app.AssembleReviewContext(ctx, req)
			require.NotNil(t, rc)
			require.NotNil(t, rc.Provenance.Origin)
			assert.Equal(t, tc.wantOrigin, rc.Provenance.Origin.Kind)
			assert.Equal(t, tc.wantEngine, rc.Provenance.Origin.Engine)
			assert.NotEmpty(t, rc.Provenance.Origin.Timestamp)
			assert.Equal(t, tc.wantState, rc.Provenance.ReviewState)
		})
	}

	t.Run("an agent's edit names the agent", func(t *testing.T) {
		cur, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/fr/guide.md"})
		require.NoError(t, err)
		fr := cur.Blocks[len(cur.Blocks)-1]
		edit(t, change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"}, fr.Rev, "Nous utilisons le gadget tous les jours.")
		rc := f.app.AssembleReviewContext(ctx, unit(t))
		require.NotNil(t, rc.Provenance.Origin)
		assert.Equal(t, model.OriginAgent, rc.Provenance.Origin.Kind)
		assert.Equal(t, "claude", rc.Provenance.Origin.Engine)
		assert.Equal(t, "s1", rc.Provenance.Origin.Reference)
	})

	t.Run("a change made outside kapi since the record leaves the ledger's origin", func(t *testing.T) {
		writeFile(t, filepath.Join(f.root, "docs", "fr", "guide.md"), "# Guide\n\nLe gadget, sans cesse.\n")
		req := unit(t)
		req.Unit = aiDraft("Le gadget, sans cesse.")
		rc := f.app.AssembleReviewContext(ctx, req)
		require.NotNil(t, rc.Provenance.Origin)
		assert.Equal(t, model.OriginAI, rc.Provenance.Origin.Kind, "the history's last change did not produce this text")
		assert.Equal(t, "approved", rc.Provenance.ReviewState)
	})
}
