package contextop_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

func TestAttest_PolicyLetsOnlyAPersonAttest(t *testing.T) {
	tests := []struct {
		name  string
		actor contextop.Actor
		ok    bool
	}{
		{"a person attests", person("asgeir"), true},
		{"an agent does not", agent("claude", "s1"), false},
		{"a tool does not", contextop.Actor{Kind: contextop.ActorTool, Name: "settle"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := contextop.PersonDecides(contextop.Transition{Actor: tt.actor, Kind: contextop.KindAttest})
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, contextop.ErrRefused)
			}
		})
	}
}

func TestAttest_RevisionAndReviews(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)

	observed, err := ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve,
		Subject: termRule("utilise", "use", false),
	})
	require.NoError(t, err)

	read := func() contextop.Record {
		t.Helper()
		r, err := ledger.Get(ctx, observed.ID)
		require.NoError(t, err)
		return r
	}
	assert.Equal(t, observed.ID, read().Revision, "a subject starts at its own revision")

	attest := func(revision string) contextop.Record {
		t.Helper()
		a, err := ledger.Append(ctx, contextop.Record{
			Project: "prj", Actor: person("asgeir"), Kind: contextop.KindAttest, Target: observed.ID,
			Attest: &contextop.Attestation{
				Item:     contextop.ItemRef{Kind: contextop.ItemOp, ID: observed.ID},
				Revision: revision, Verdict: contextop.VerdictKeep, Basis: contextop.BasisAgentAuthored,
			},
		})
		require.NoError(t, err)
		return a
	}
	first := attest(observed.ID)

	all, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	reviews := contextop.Reviews(all)
	key := contextop.ItemRef{Kind: contextop.ItemOp, ID: observed.ID}.Key()
	require.Contains(t, reviews, key)
	assert.Equal(t, first.ID, reviews[key].ID)
	assert.Equal(t, contextop.BasisAgentAuthored, reviews[key].Basis)
	assert.True(t, reviews[key].Current(read().Revision), "the attestation covers the revision it named")
	assert.Equal(t, contextop.StatusSuggested, read().Status, "an attestation changes nothing in force")

	// A plain keep leaves the rule as it reads, so the review still holds.
	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: observed.ID})
	require.NoError(t, err)
	assert.True(t, reviews[key].Current(read().Revision), "keeping without an edit keeps the revision")

	// A keep that edits the rule is a new revision, and the review no longer covers it.
	edit, err := ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: observed.ID,
		Subject: termRule("utilise", "employ", false),
	})
	require.NoError(t, err)
	assert.Equal(t, edit.ID, read().Revision)
	assert.False(t, reviews[key].Current(read().Revision), "an edit after the review makes it changed since review")

	second := attest(edit.ID)
	all, err = ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	reviews = contextop.Reviews(all)
	assert.Equal(t, second.ID, reviews[key].ID, "the latest attestation wins")
	assert.True(t, reviews[key].Current(read().Revision))
}

func TestAttest_ItemInAnotherStore(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	ref := contextop.ItemRef{Kind: contextop.ItemConcept, ID: "c1"}
	_, err := ledger.Append(ctx, contextop.Record{
		Project: "prj", Actor: person("asgeir"), Kind: contextop.KindAttest,
		Attest: &contextop.Attestation{Item: ref, Revision: "3", Verdict: contextop.VerdictKeep, Basis: contextop.BasisSoloOwner},
	})
	require.NoError(t, err)
	all, err := ledger.Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	reviews := contextop.Reviews(all)
	require.Contains(t, reviews, ref.Key())
	assert.True(t, reviews[ref.Key()].Current("3"))
	assert.False(t, reviews[ref.Key()].Current("4"))
}

func TestReviews_CountsOnlyAPersonsAttestation(t *testing.T) {
	item := contextop.ItemRef{Kind: contextop.ItemOp, ID: "op1"}
	tests := []struct {
		name   string
		actor  contextop.Actor
		counts bool
	}{
		{"a person", person("asgeir"), true},
		{"an actor with no kind reads as a person", contextop.Actor{Name: "asgeir"}, true},
		{"an agent", agent("claude", "s1"), false},
		{"a tool issued by a member", contextop.Actor{Kind: contextop.ActorTool, Name: "dogfood-sync", OnBehalfOf: "Asgeir"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviews := contextop.Reviews([]contextop.Record{{
				ID: "a1", Kind: contextop.KindAttest, Actor: tt.actor,
				Attest: &contextop.Attestation{Item: item, Revision: "op1", Verdict: contextop.VerdictKeep, Basis: contextop.BasisPeer},
			}})
			_, ok := reviews[item.Key()]
			assert.Equal(t, tt.counts, ok)
		})
	}
}
