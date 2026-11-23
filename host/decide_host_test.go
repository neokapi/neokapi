package host

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/core/state"
)

// A person who declared themselves in an agent host's shell rejects a
// translation: the record keeps the note they typed and the agent host as a
// field of its own. A record a released client wrote, with the host folded
// into the note, reads the same way, and is never rewritten.
func TestDecide_TheAgentHostIsRecordedApartFromTheNote(t *testing.T) {
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
	inShell := change.Actor{Kind: change.ActorPerson, AgentHost: "claude-code"}
	res, err := svc.Apply(ctx, change.Set{Note: "Too formal.", Ops: []change.Op{
		{Kind: change.KindDecide, At: at, IfMatch: rev, Body: &change.Decide{Outcome: change.OutcomeReject, Reasons: []string{"tone"}}},
	}}, inShell)
	require.NoError(t, err)
	require.Nil(t, res.Ops[0].Error)

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	key := state.Key{Scope: a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json")),
		Unit: "greeting", Variant: model.Variant("fr")}
	u, ok := st.Get(ctx, key)
	require.True(t, ok)
	assert.Equal(t, "Too formal.", u.Decision.Note, "the note is what the person typed")
	assert.Equal(t, "claude-code", u.Decision.AgentHost)
	p := review.ProvenanceOf(nil, "fr", &u)
	assert.Equal(t, "Too formal.", p.Note)
	assert.Equal(t, "claude-code", p.AgentHost)

	// A record a released client wrote.
	legacy := u
	legacy.Decision.Note = "Too formal. (recorded as a person in a claude-code shell)"
	legacy.Decision.AgentHost = ""
	require.NoError(t, st.Put(ctx, legacy))
	stored, _ := st.Get(ctx, key)
	assert.Equal(t, legacy.Decision.Note, stored.Decision.Note, "the store holds what it was given")
	note, agentHost := stored.Decision.Said()
	assert.Equal(t, "Too formal.", note)
	assert.Equal(t, "claude-code", agentHost)
	p = review.ProvenanceOf(nil, "fr", &stored)
	assert.Equal(t, "Too formal.", p.Note, "a reader shows the note alone")
	assert.Equal(t, "claude-code", p.AgentHost)
	assert.Equal(t, "Too formal.", decisionNote(stored.Decision), "the next draft's feedback carries the note alone")
}
