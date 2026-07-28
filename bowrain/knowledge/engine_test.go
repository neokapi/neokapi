package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// term is a small constructor for a terms.Term.
func term(text string, locale model.LocaleID, status model.TermStatus) terms.Term {
	return terms.Term{Text: text, Locale: locale, Status: status}
}

func concept(id string, ts ...terms.Term) terms.Concept {
	return terms.Concept{ID: id, Terms: ts}
}

// ---------------------------------------------------------------------------
// ApplyVoiceOpsToProfile
// ---------------------------------------------------------------------------

func TestApplyVoiceOpsToProfile(t *testing.T) {
	baseline := &coreprofile.VoiceProfile{
		ID:   "p1",
		Name: "Acme",
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "synergy"}},
		},
	}

	t.Run("add to each list", func(t *testing.T) {
		ops := []ChangeSetOp{
			mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: coreprofile.TermRule{Term: "leverage", Replacement: "use"}}),
			mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListPreferred, Rule: coreprofile.TermRule{Term: "sign in"}}),
			mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListCompetitor, Rule: coreprofile.TermRule{Term: "Globex"}}),
		}
		cand := ApplyVoiceOpsToProfile(baseline, ops)
		require.NotNil(t, cand)

		assert.Len(t, cand.Vocabulary.ForbiddenTerms, 2)
		assert.Equal(t, "leverage", cand.Vocabulary.ForbiddenTerms[1].Term)
		assert.Equal(t, "use", cand.Vocabulary.ForbiddenTerms[1].Replacement)
		require.Len(t, cand.Vocabulary.PreferredTerms, 1)
		assert.Equal(t, "sign in", cand.Vocabulary.PreferredTerms[0].Term)
		require.Len(t, cand.Vocabulary.CompetitorTerms, 1)
		assert.Equal(t, "Globex", cand.Vocabulary.CompetitorTerms[0].Term)

		// Baseline is never mutated.
		assert.Len(t, baseline.Vocabulary.ForbiddenTerms, 1)
		assert.Empty(t, baseline.Vocabulary.PreferredTerms)
	})

	t.Run("remove from list", func(t *testing.T) {
		ops := []ChangeSetOp{
			mustOp(t, 0, OpVoiceRuleRemove, VoiceRuleRemovePayload{ProfileID: "p1", List: VoiceListForbidden, Term: "SYNERGY"}),
		}
		cand := ApplyVoiceOpsToProfile(baseline, ops)
		require.NotNil(t, cand)
		assert.Empty(t, cand.Vocabulary.ForbiddenTerms)
		// Baseline retains its rule.
		assert.Len(t, baseline.Vocabulary.ForbiddenTerms, 1)
	})

	t.Run("add is idempotent by term", func(t *testing.T) {
		ops := []ChangeSetOp{
			mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: coreprofile.TermRule{Term: "synergy", Replacement: "teamwork"}}),
		}
		cand := ApplyVoiceOpsToProfile(baseline, ops)
		require.Len(t, cand.Vocabulary.ForbiddenTerms, 1)
		assert.Equal(t, "teamwork", cand.Vocabulary.ForbiddenTerms[0].Replacement)
	})

	t.Run("ops for other profiles are ignored", func(t *testing.T) {
		ops := []ChangeSetOp{
			mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "other", List: VoiceListForbidden, Rule: coreprofile.TermRule{Term: "leverage"}}),
		}
		cand := ApplyVoiceOpsToProfile(baseline, ops)
		assert.Len(t, cand.Vocabulary.ForbiddenTerms, 1)
	})

	t.Run("nil baseline yields nil", func(t *testing.T) {
		assert.Nil(t, ApplyVoiceOpsToProfile(nil, nil))
	})
}

// ---------------------------------------------------------------------------
// ApplyOpsToTerms
// ---------------------------------------------------------------------------

func TestApplyOpsToTerms(t *testing.T) {
	ctx := context.Background()

	newBase := func(t *testing.T) *terms.InMemoryStore {
		t.Helper()
		base := terms.NewInMemoryStore()
		require.NoError(t, base.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))
		return base
	}

	t.Run("term.status sets status and leaves base unmutated", func(t *testing.T) {
		base := newBase(t)
		ops := []ChangeSetOp{mustOp(t, 0, OpTermStatus, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "foobar",
			From: model.TermAdmitted, To: model.TermForbidden,
		})}

		after, err := ApplyOpsToTerms(ctx, base, ops)
		require.NoError(t, err)

		ac, ok, _ := after.GetConcept(ctx, "c1")
		require.True(t, ok)
		assert.Equal(t, model.TermForbidden, ac.Terms[0].Status)

		bc, ok, _ := base.GetConcept(ctx, "c1")
		require.True(t, ok)
		assert.Equal(t, model.TermAdmitted, bc.Terms[0].Status, "base must not be mutated")
	})

	t.Run("concept.create / term.add / term.remove", func(t *testing.T) {
		base := newBase(t)
		ops := []ChangeSetOp{
			mustOp(t, 0, OpConceptCreate, ConceptCreatePayload{Concept: concept("c2", term("widget", "en-US", model.TermPreferred))}),
			mustOp(t, 0, OpTermAdd, TermAddPayload{ConceptID: "c1", Term: term("foo-bar", "en-GB", model.TermAdmitted)}),
			mustOp(t, 0, OpTermRemove, TermRemovePayload{ConceptID: "c2", Locale: "en-US", Text: "widget"}),
		}
		after, err := ApplyOpsToTerms(ctx, base, ops)
		require.NoError(t, err)

		c1, ok, _ := after.GetConcept(ctx, "c1")
		require.True(t, ok)
		assert.Len(t, c1.Terms, 2)

		c2, ok, _ := after.GetConcept(ctx, "c2")
		require.True(t, ok)
		assert.Empty(t, c2.Terms)

		// Base still has only c1 with one term.
		bc1, _, _ := base.GetConcept(ctx, "c1")
		assert.Len(t, bc1.Terms, 1)
		_, ok, _ = base.GetConcept(ctx, "c2")
		assert.False(t, ok)
	})

	t.Run("relation.add and relation.remove", func(t *testing.T) {
		base := terms.NewInMemoryStore()
		require.NoError(t, base.AddConcept(ctx, concept("c1", term("old", "en-US", model.TermDeprecated))))
		require.NoError(t, base.AddConcept(ctx, concept("c2", term("new", "en-US", model.TermPreferred))))

		rel := terms.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelUseInstead}
		after, err := ApplyOpsToTerms(ctx, base, []ChangeSetOp{
			mustOp(t, 0, OpRelationAdd, RelationAddPayload{Relation: rel}),
		})
		require.NoError(t, err)
		rels, _ := after.RelationsOf(ctx, "c1", nil)
		require.Len(t, rels, 1)

		// Base has no relations.
		baseRels, _ := base.RelationsOf(ctx, "c1", nil)
		assert.Empty(t, baseRels)

		// relation.remove drops it again.
		removed, err := ApplyOpsToTerms(ctx, after, []ChangeSetOp{
			mustOp(t, 0, OpRelationRemove, RelationRemovePayload{RelationID: "r1"}),
		})
		require.NoError(t, err)
		rels, _ = removed.RelationsOf(ctx, "c1", nil)
		assert.Empty(t, rels)
	})

	t.Run("voice ops are ignored by the terms store builder", func(t *testing.T) {
		base := newBase(t)
		after, err := ApplyOpsToTerms(ctx, base, []ChangeSetOp{
			mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: coreprofile.TermRule{Term: "x"}}),
		})
		require.NoError(t, err)
		n, _ := after.Count(ctx)
		assert.Equal(t, 1, n)
	})

	t.Run("editing a missing concept errors", func(t *testing.T) {
		base := newBase(t)
		_, err := ApplyOpsToTerms(ctx, base, []ChangeSetOp{
			mustOp(t, 0, OpTermStatus, TermStatusPayload{ConceptID: "ghost", Locale: "en-US", Text: "foobar", From: model.TermAdmitted, To: model.TermForbidden}),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ghost")
	})
}
