package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
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

	t.Run("editing a missing concept errors", func(t *testing.T) {
		base := newBase(t)
		_, err := ApplyOpsToTerms(ctx, base, []ChangeSetOp{
			mustOp(t, 0, OpTermStatus, TermStatusPayload{ConceptID: "ghost", Locale: "en-US", Text: "foobar", From: model.TermAdmitted, To: model.TermForbidden}),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ghost")
	})
}
