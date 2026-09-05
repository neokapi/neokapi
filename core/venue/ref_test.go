package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/terms"
)

func decision(unit, variant, status string) UnitDecision {
	return UnitDecision{
		ItemName: "docs/intro.md", Unit: unit, Variant: variant,
		Status: status, ReviewState: "approved", DecidedBy: "ana", Updated: "2026-08-01T00:00:00Z",
	}
}

func TestDecisionsComponentIsOrderIndependent(t *testing.T) {
	a := DecisionsComponent([]UnitDecision{
		decision("u1", "nb", "reviewed"),
		decision("u2", "fr", "translated"),
	})
	b := DecisionsComponent([]UnitDecision{
		decision("u2", "fr", "translated"),
		decision("u1", "nb", "reviewed"),
	})
	assert.Equal(t, a, b,
		"the committed record arrives in shard order and the ledger in SQL order; they must still agree")
	assert.NotEmpty(t, a)
}

func TestDecisionsComponentMovesOnlyOnADecision(t *testing.T) {
	base := []UnitDecision{decision("u1", "nb", "reviewed")}
	component := DecisionsComponent(base)

	tests := []struct {
		name   string
		mutate func(UnitDecision) UnitDecision
		moves  bool
	}{
		{name: "an unchanged replay", mutate: func(d UnitDecision) UnitDecision { return d }},
		{
			name:   "a bumped record time alone",
			mutate: func(d UnitDecision) UnitDecision { d.Updated = "2099-01-01T00:00:00Z"; return d },
		},
		{
			name:   "a changed rung",
			mutate: func(d UnitDecision) UnitDecision { d.Status = "signed-off"; return d },
			moves:  true,
		},
		{
			name:   "a changed review verdict",
			mutate: func(d UnitDecision) UnitDecision { d.ReviewState = "rejected"; return d },
			moves:  true,
		},
		{
			name:   "a different decider",
			mutate: func(d UnitDecision) UnitDecision { d.DecidedBy = "ben"; return d },
			moves:  true,
		},
		{
			name:   "a parked unit",
			mutate: func(d UnitDecision) UnitDecision { d.Parked = true; return d },
			moves:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := []UnitDecision{tt.mutate(base[0])}
			got := DecisionsComponent(next)
			if tt.moves {
				assert.NotEqual(t, component, got)
				assert.False(t, SameDecision(base[0], next[0]))
				return
			}
			assert.Equal(t, component, got)
			assert.True(t, SameDecision(base[0], next[0]),
				"a record the store would skip as unchanged must not move the component")
		})
	}
}

// TestDecisionIdentityAgreesWithSameDecision is the invariant the store depends
// on: what counts as a change and what moves the component are one definition.
func TestDecisionIdentityAgreesWithSameDecision(t *testing.T) {
	a := decision("u1", "nb", "reviewed")
	b := a
	b.Updated = "2099-01-01T00:00:00Z"
	c := a
	c.Note = "reworded"

	assert.True(t, SameDecision(a, b))
	assert.Equal(t, DecisionIdentity(a), DecisionIdentity(b))
	assert.False(t, SameDecision(a, c))
	assert.NotEqual(t, DecisionIdentity(a), DecisionIdentity(c))
}

func TestDecisionsComponentIgnoresUnkeyedRecords(t *testing.T) {
	assert.Empty(t, DecisionsComponent(nil))
	assert.Empty(t, DecisionsComponent([]UnitDecision{{ItemName: "x"}}),
		"a record with no unit or variant keys nothing and must not fabricate a component")
}

func concept(id, definition string, termTexts ...string) terms.Concept {
	c := terms.Concept{ID: id, Domain: "product", Definition: definition}
	for _, text := range termTexts {
		c.Terms = append(c.Terms, terms.Term{
			Text: text, Locale: model.LocaleEnglish, Status: model.TermPreferred,
		})
	}
	return c
}

func TestTermsComponent(t *testing.T) {
	base := []terms.Concept{concept("c1", "The loop.", "convergence"), concept("c2", "A voice.", "voice")}
	relations := []terms.ConceptRelation{{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: "RELATED"}}
	component := TermsComponent(base, relations)

	tests := []struct {
		name      string
		concepts  []terms.Concept
		relations []terms.ConceptRelation
		moves     bool
	}{
		{name: "the same set again", concepts: base, relations: relations},
		{
			name:      "the same set in another order",
			concepts:  []terms.Concept{base[1], base[0]},
			relations: relations,
		},
		{
			name:      "a reworded definition",
			concepts:  []terms.Concept{concept("c1", "The kapi loop.", "convergence"), base[1]},
			relations: relations,
			moves:     true,
		},
		{
			name:      "an added term",
			concepts:  []terms.Concept{concept("c1", "The loop.", "convergence", "converge"), base[1]},
			relations: relations,
			moves:     true,
		},
		{
			name:      "a deleted concept",
			concepts:  []terms.Concept{base[0]},
			relations: relations,
			moves:     true,
		},
		{
			name:      "a removed relation",
			concepts:  base,
			relations: nil,
			moves:     true,
		},
		{
			name:      "a relabelled relation",
			concepts:  base,
			relations: []terms.ConceptRelation{{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: "BROADER"}},
			moves:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TermsComponent(tt.concepts, tt.relations)
			if tt.moves {
				assert.NotEqual(t, component, got)
				return
			}
			assert.Equal(t, component, got)
		})
	}
}

func TestTermsComponentIsEmptyForAnEmptyVocabulary(t *testing.T) {
	assert.Empty(t, TermsComponent(nil, nil),
		"a workspace with no terminology makes no claim, which compares as unknown rather than as a difference")
}

// TestConceptTermOrderIsNotIdentity pins the one non-obvious property: a store
// returns a concept's terms in insertion order, and re-reading the same concept
// must hash the same however that order came out.
func TestConceptTermOrderIsNotIdentity(t *testing.T) {
	forward := concept("c1", "The loop.", "convergence", "converge")
	reversed := concept("c1", "The loop.")
	reversed.Terms = []terms.Term{forward.Terms[1], forward.Terms[0]}
	assert.Equal(t,
		TermsComponent([]terms.Concept{forward}, nil),
		TermsComponent([]terms.Concept{reversed}, nil))
}

// TestDecisionIdentity_GoverningFingerprint: a record that gains a governing
// fingerprint is a change a store must write and the component must reflect,
// while a record carrying none keeps the identity it had before the field
// existed, so an upgrade moves no project's decisions component.
func TestDecisionIdentity_GoverningFingerprint(t *testing.T) {
	legacy := decision("u1", "nb", "reviewed")
	stamped := legacy
	stamped.GoverningFingerprint = "fp-governing"
	restamped := legacy
	restamped.GoverningFingerprint = "fp-moved"

	assert.False(t, SameDecision(legacy, stamped), "gaining a fingerprint is a change")
	assert.NotEqual(t, DecisionIdentity(legacy), DecisionIdentity(stamped))
	assert.False(t, SameDecision(stamped, restamped), "a decision re-made under a moved context is a change")
	assert.NotEqual(t, DecisionsComponent([]UnitDecision{stamped}), DecisionsComponent([]UnitDecision{restamped}))

	// A record without a fingerprint folds over exactly the fields it always
	// had: the new field leaves no trace on it.
	assert.Equal(t, ref.Identity(legacy.Status, legacy.TargetHash, legacy.ContentHash, legacy.ReviewState,
		legacy.DecidedBy, legacy.DecidedAt, legacy.Note, "false", legacy.Assignee), DecisionIdentity(legacy))
}

// TestAsBasis_DropsTheGoverningFingerprint: the fingerprint records the
// context a verdict was made under, so a refused verdict kept as a bare basis
// carries none.
func TestAsBasis_DropsTheGoverningFingerprint(t *testing.T) {
	d := decision("u1", "nb", "reviewed")
	d.GoverningFingerprint = "fp-governing"
	basis := d.AsBasis(model.TargetStatusTranslated)
	assert.Empty(t, basis.GoverningFingerprint)
	assert.Empty(t, basis.ReviewState)
}
