package host

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// A rejection through kapi's own change service keeps its reasons and note
// in the project's record, as the platform does.
func TestDecide_ARejectionKeepsItsReasonsInTheRecord(t *testing.T) {
	root := writeStalenessProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}
	a.InitRegistries()
	ctx := context.Background()
	recordFlowWrite(t, a, recipe, "locales/en/app.json", "fr", model.Origin{Kind: model.OriginAI})

	edition := model.EditionKey{Locale: "fr"}
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "test", TargetLocale: edition.Locale})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/fr/app.json", Blocks: []string{"greeting"}, Editions: []model.EditionKey{edition}})
	require.NoError(t, err)
	require.NotEmpty(t, page.Blocks)
	b := page.Blocks[0]
	at, rev := b.Ref, b.Rev
	if at.Edition.Canonical() != edition.Canonical() {
		at.Edition = edition
		rev = model.AbsentRevision
		if ed, ok := b.Editions[at.EditionText()]; ok {
			rev = ed.Rev
		}
	}

	reject := func(reasons ...string) change.OpStatus {
		res, err := svc.Apply(ctx, change.Set{Note: "Too formal.", Ops: []change.Op{
			{Kind: change.KindDecide, At: at, IfMatch: rev, Body: &change.Decide{Outcome: change.OutcomeReject, Reasons: reasons}},
		}}, reviewPerson)
		require.NoError(t, err)
		require.Nil(t, res.Ops[0].Error)
		return res.Ops[0].Status
	}
	assert.Equal(t, change.OpApplied, reject("tone", "meaning"))

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	key := state.Key{Scope: a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json")),
		Unit: "greeting", Variant: model.Variant("fr")}
	u, ok := st.Get(ctx, key)
	require.True(t, ok)
	assert.Equal(t, ReviewDecisionRejected, u.Decision.ReviewState)
	assert.Equal(t, []string{"meaning", "tone"}, u.Decision.Reasons)
	assert.Equal(t, "Too formal.", u.Decision.Note)

	assert.Equal(t, change.OpUnchanged, reject("meaning", "tone"), "the same rejection again changes nothing")
	assert.Equal(t, change.OpApplied, reject("grammar"), "other reasons are a decision of their own")
	u, _ = st.Get(ctx, key)
	assert.Equal(t, []string{"grammar"}, u.Decision.Reasons)

	// What the platform's change route refuses, kapi refuses in the same
	// words, before the record holds anything a push would have to carry.
	refused := func(note string, reasons ...string) *change.Error {
		res, err := svc.Apply(ctx, change.Set{Note: note, Ops: []change.Op{
			{Kind: change.KindDecide, At: at, IfMatch: rev, Body: &change.Decide{Outcome: change.OutcomeReject, Reasons: reasons}},
		}}, reviewPerson)
		require.NoError(t, err)
		require.NotNil(t, res.Ops[0].Error)
		return res.Ops[0].Error
	}
	e := refused("Too formal.", "tone", "made_up")
	assert.Equal(t, change.CodeInvalid, e.Code)
	assert.Equal(t, "reasons", e.Field)
	assert.Equal(t, venue.CheckDecisionInput("", []string{"made_up"}).Message, e.Message)
	e = refused(strings.Repeat("x", venue.MaxDecisionNote+1), "tone")
	assert.Equal(t, "note", e.Field)
	u, _ = st.Get(ctx, key)
	assert.Equal(t, []string{"grammar"}, u.Decision.Reasons, "a refused decision changes nothing")
}
