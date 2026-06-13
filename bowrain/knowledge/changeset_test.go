package knowledge

import (
	"encoding/json"
	"testing"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustOp builds a ChangeSetOp whose payload is the JSON encoding of payload.
// A nil payload leaves ChangeSetOp.Payload empty (the empty-payload case).
func mustOp(t *testing.T, seq int64, opType OpType, payload any) ChangeSetOp {
	t.Helper()
	op := ChangeSetOp{Seq: seq, Op: opType}
	if payload != nil {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		op.Payload = b
	}
	return op
}

func TestValidateOp(t *testing.T) {
	tests := []struct {
		name    string
		op      ChangeSetOp
		wantErr bool
	}{
		// concept.create
		{"concept.create valid", mustOp(t, 1, OpConceptCreate, ConceptCreatePayload{
			Concept: termbase.Concept{ID: "c1", Terms: []termbase.Term{{Text: "widget", Locale: "en", Status: model.TermApproved}}},
		}), false},
		{"concept.create missing id", mustOp(t, 1, OpConceptCreate, ConceptCreatePayload{
			Concept: termbase.Concept{Definition: "no id"},
		}), true},
		{"concept.create bad term status", mustOp(t, 1, OpConceptCreate, ConceptCreatePayload{
			Concept: termbase.Concept{ID: "c1", Terms: []termbase.Term{{Text: "x", Locale: "en", Status: "bogus"}}},
		}), true},

		// concept.update
		{"concept.update valid", mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "c1"}), false},
		{"concept.update missing id", mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{}), true},

		// concept.delete
		{"concept.delete valid", mustOp(t, 1, OpConceptDelete, ConceptDeletePayload{ConceptID: "c1"}), false},
		{"concept.delete missing id", mustOp(t, 1, OpConceptDelete, ConceptDeletePayload{}), true},

		// term.add
		{"term.add valid", mustOp(t, 1, OpTermAdd, TermAddPayload{ConceptID: "c1", Term: termbase.Term{Text: "x", Locale: "en"}}), false},
		{"term.add missing concept", mustOp(t, 1, OpTermAdd, TermAddPayload{Term: termbase.Term{Text: "x", Locale: "en"}}), true},
		{"term.add missing text", mustOp(t, 1, OpTermAdd, TermAddPayload{ConceptID: "c1", Term: termbase.Term{Locale: "en"}}), true},
		{"term.add missing locale", mustOp(t, 1, OpTermAdd, TermAddPayload{ConceptID: "c1", Term: termbase.Term{Text: "x"}}), true},
		{"term.add bad status", mustOp(t, 1, OpTermAdd, TermAddPayload{ConceptID: "c1", Term: termbase.Term{Text: "x", Locale: "en", Status: "nope"}}), true},

		// term.update
		{"term.update valid", mustOp(t, 1, OpTermUpdate, TermUpdatePayload{ConceptID: "c1", Locale: "en", Text: "x", Term: termbase.Term{Text: "y", Locale: "en"}}), false},
		{"term.update missing locale", mustOp(t, 1, OpTermUpdate, TermUpdatePayload{ConceptID: "c1", Text: "x"}), true},
		{"term.update missing text", mustOp(t, 1, OpTermUpdate, TermUpdatePayload{ConceptID: "c1", Locale: "en"}), true},

		// term.remove
		{"term.remove valid", mustOp(t, 1, OpTermRemove, TermRemovePayload{ConceptID: "c1", Locale: "en", Text: "x"}), false},
		{"term.remove missing text", mustOp(t, 1, OpTermRemove, TermRemovePayload{ConceptID: "c1", Locale: "en"}), true},

		// term.status
		{"term.status valid governed", mustOp(t, 1, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en", Text: "x", From: model.TermApproved, To: model.TermForbidden}), false},
		{"term.status valid ordinary", mustOp(t, 1, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en", Text: "x", From: model.TermProposed, To: model.TermApproved}), false},
		{"term.status unknown from", mustOp(t, 1, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en", Text: "x", From: "bogus", To: model.TermApproved}), true},
		{"term.status unknown to", mustOp(t, 1, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en", Text: "x", From: model.TermApproved, To: "bogus"}), true},
		{"term.status missing text", mustOp(t, 1, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en", From: model.TermApproved, To: model.TermPreferred}), true},

		// relation.add
		{"relation.add valid", mustOp(t, 1, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelReplacedBy}}), false},
		{"relation.add missing id", mustOp(t, 1, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{SourceID: "c1", TargetID: "c2", RelationType: graph.LabelRelated}}), true},
		{"relation.add unknown type", mustOp(t, 1, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: "WAT"}}), true},

		// relation.remove
		{"relation.remove valid", mustOp(t, 1, OpRelationRemove, RelationRemovePayload{RelationID: "r1"}), false},
		{"relation.remove missing id", mustOp(t, 1, OpRelationRemove, RelationRemovePayload{}), true},

		// voice.rule.add
		{"voice.rule.add valid", mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "synergy"}}), false},
		{"voice.rule.add missing profile", mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "synergy"}}), true},
		{"voice.rule.add bad list", mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: "nope", Rule: corebrand.TermRule{Term: "synergy"}}), true},
		{"voice.rule.add missing term", mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListPreferred}), true},

		// voice.rule.remove
		{"voice.rule.remove valid", mustOp(t, 1, OpVoiceRuleRemove, VoiceRuleRemovePayload{ProfileID: "p1", List: VoiceListForbidden, Term: "synergy"}), false},
		{"voice.rule.remove missing term", mustOp(t, 1, OpVoiceRuleRemove, VoiceRuleRemovePayload{ProfileID: "p1", List: VoiceListForbidden}), true},

		// structural
		{"unknown op type", mustOp(t, 1, OpType("concept.frobnicate"), map[string]string{}), true},
		{"empty payload", ChangeSetOp{Seq: 1, Op: OpConceptUpdate}, true},
		{"malformed payload", ChangeSetOp{Seq: 1, Op: OpConceptUpdate, Payload: json.RawMessage(`{`)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOp(tt.op)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsGovernedOp(t *testing.T) {
	tests := []struct {
		name    string
		op      ChangeSetOp
		want    bool
		wantErr bool
	}{
		{"term.status to forbidden is governed", mustOp(t, 1, OpTermStatus, TermStatusPayload{From: model.TermApproved, To: model.TermForbidden}), true, false},
		{"term.status to preferred is governed", mustOp(t, 1, OpTermStatus, TermStatusPayload{From: model.TermApproved, To: model.TermPreferred}), true, false},
		{"term.status from forbidden is governed", mustOp(t, 1, OpTermStatus, TermStatusPayload{From: model.TermForbidden, To: model.TermDeprecated}), true, false},
		{"term.status ordinary", mustOp(t, 1, OpTermStatus, TermStatusPayload{From: model.TermProposed, To: model.TermApproved}), false, false},
		{"relation.add REPLACED_BY governed", mustOp(t, 1, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{RelationType: graph.LabelReplacedBy}}), true, false},
		{"relation.add RELATED ordinary", mustOp(t, 1, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{RelationType: graph.LabelRelated}}), false, false},
		{"concept.delete governed", mustOp(t, 1, OpConceptDelete, ConceptDeletePayload{ConceptID: "c1"}), true, false},
		{"voice.rule.add governed", mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "x"}}), true, false},
		{"voice.rule.remove governed", mustOp(t, 1, OpVoiceRuleRemove, VoiceRuleRemovePayload{ProfileID: "p1", List: VoiceListForbidden, Term: "x"}), true, false},
		{"concept.create ordinary", mustOp(t, 1, OpConceptCreate, ConceptCreatePayload{Concept: termbase.Concept{ID: "c1"}}), false, false},
		{"concept.update ordinary", mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "c1"}), false, false},
		{"term.add ordinary", mustOp(t, 1, OpTermAdd, TermAddPayload{ConceptID: "c1"}), false, false},
		{"term.update ordinary", mustOp(t, 1, OpTermUpdate, TermUpdatePayload{ConceptID: "c1"}), false, false},
		{"term.remove ordinary", mustOp(t, 1, OpTermRemove, TermRemovePayload{ConceptID: "c1"}), false, false},
		{"relation.remove ordinary", mustOp(t, 1, OpRelationRemove, RelationRemovePayload{RelationID: "r1"}), false, false},
		{"unknown op error", mustOp(t, 1, OpType("nope"), map[string]string{}), false, true},
		{"term.status malformed payload error", ChangeSetOp{Op: OpTermStatus, Payload: json.RawMessage(`{`)}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsGovernedOp(tt.op)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestChangeSetIsGoverned(t *testing.T) {
	ordinary1 := mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "c1"})
	ordinary2 := mustOp(t, 2, OpTermAdd, TermAddPayload{ConceptID: "c1", Term: termbase.Term{Text: "x", Locale: "en"}})
	governed := mustOp(t, 3, OpConceptDelete, ConceptDeletePayload{ConceptID: "c1"})
	broken := ChangeSetOp{Seq: 4, Op: OpType("bogus")}

	t.Run("all ordinary", func(t *testing.T) {
		got, err := ChangeSetIsGoverned([]ChangeSetOp{ordinary1, ordinary2})
		require.NoError(t, err)
		assert.False(t, got)
	})
	t.Run("one governed", func(t *testing.T) {
		got, err := ChangeSetIsGoverned([]ChangeSetOp{ordinary1, governed, ordinary2})
		require.NoError(t, err)
		assert.True(t, got)
	})
	t.Run("empty", func(t *testing.T) {
		got, err := ChangeSetIsGoverned(nil)
		require.NoError(t, err)
		assert.False(t, got)
	})
	t.Run("error propagates", func(t *testing.T) {
		_, err := ChangeSetIsGoverned([]ChangeSetOp{ordinary1, broken})
		assert.Error(t, err)
	})
}

func TestValidateStatusTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    ChangeSetStatus
		to      ChangeSetStatus
		wantErr bool
	}{
		// allowed
		{"draft to in_review", ChangeSetDraft, ChangeSetInReview, false},
		{"draft to abandoned", ChangeSetDraft, ChangeSetAbandoned, false},
		{"in_review to approved", ChangeSetInReview, ChangeSetApproved, false},
		{"in_review to draft (reject)", ChangeSetInReview, ChangeSetDraft, false},
		{"in_review to abandoned", ChangeSetInReview, ChangeSetAbandoned, false},
		{"approved to merged", ChangeSetApproved, ChangeSetMerged, false},
		{"approved to abandoned", ChangeSetApproved, ChangeSetAbandoned, false},
		{"approved to in_review (reopen)", ChangeSetApproved, ChangeSetInReview, false},
		{"draft to merged (ordinary fast-path)", ChangeSetDraft, ChangeSetMerged, false},
		// disallowed
		{"draft to approved", ChangeSetDraft, ChangeSetApproved, true},
		{"in_review to merged", ChangeSetInReview, ChangeSetMerged, true},
		{"merged terminal", ChangeSetMerged, ChangeSetDraft, true},
		{"abandoned terminal", ChangeSetAbandoned, ChangeSetDraft, true},
		{"no-op same status", ChangeSetDraft, ChangeSetDraft, true},
		{"invalid from", ChangeSetStatus("bogus"), ChangeSetDraft, true},
		{"invalid to", ChangeSetDraft, ChangeSetStatus("bogus"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusTransition(tt.from, tt.to)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCanMerge(t *testing.T) {
	const author = "alice"
	const reviewer = "bob"

	approvedCS := ChangeSet{Status: ChangeSetApproved, CreatedBy: author}

	tests := []struct {
		name     string
		cs       ChangeSet
		governed bool
		reviews  []ChangeSetReview
		wantErr  bool
	}{
		{
			name:     "governed approved with foreign approval",
			cs:       approvedCS,
			governed: true,
			reviews:  []ChangeSetReview{{Reviewer: reviewer, Verdict: VerdictApprove}},
			wantErr:  false,
		},
		{
			name:     "governed not approved",
			cs:       ChangeSet{Status: ChangeSetInReview, CreatedBy: author},
			governed: true,
			reviews:  []ChangeSetReview{{Reviewer: reviewer, Verdict: VerdictApprove}},
			wantErr:  true,
		},
		{
			name:     "governed approved but only self-approval",
			cs:       approvedCS,
			governed: true,
			reviews:  []ChangeSetReview{{Reviewer: author, Verdict: VerdictApprove}},
			wantErr:  true,
		},
		{
			name:     "governed approved but only foreign reject",
			cs:       approvedCS,
			governed: true,
			reviews:  []ChangeSetReview{{Reviewer: reviewer, Verdict: VerdictReject}},
			wantErr:  true,
		},
		{
			name:     "governed approved no reviews",
			cs:       approvedCS,
			governed: true,
			reviews:  nil,
			wantErr:  true,
		},
		{
			name:     "ordinary draft merges",
			cs:       ChangeSet{Status: ChangeSetDraft, CreatedBy: author},
			governed: false,
			wantErr:  false,
		},
		{
			name:     "ordinary approved merges",
			cs:       approvedCS,
			governed: false,
			wantErr:  false,
		},
		{
			name:     "ordinary in_review blocked",
			cs:       ChangeSet{Status: ChangeSetInReview, CreatedBy: author},
			governed: false,
			wantErr:  true,
		},
		{
			name:     "ordinary merged blocked",
			cs:       ChangeSet{Status: ChangeSetMerged, CreatedBy: author},
			governed: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanMerge(tt.cs, tt.governed, tt.reviews)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckBaseRev(t *testing.T) {
	t.Run("zero base never conflicts", func(t *testing.T) {
		op := mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "c1"})
		op.BaseRev = 0
		assert.Nil(t, CheckBaseRev(op, 7))
	})
	t.Run("matching base is clean", func(t *testing.T) {
		op := mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "c1"})
		op.BaseRev = 7
		assert.Nil(t, CheckBaseRev(op, 7))
	})
	t.Run("mismatch yields conflict with concept id", func(t *testing.T) {
		op := mustOp(t, 5, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "c1"})
		op.BaseRev = 3
		c := CheckBaseRev(op, 9)
		require.NotNil(t, c)
		assert.Equal(t, int64(5), c.Seq)
		assert.Equal(t, "c1", c.ConceptID)
		assert.NotEmpty(t, c.Reason)
	})

	// concept-id extraction across the op types that name a concept.
	conceptIDCases := []struct {
		name string
		op   ChangeSetOp
		want string
	}{
		{"concept.create", mustOp(t, 1, OpConceptCreate, ConceptCreatePayload{Concept: termbase.Concept{ID: "cc"}}), "cc"},
		{"concept.update", mustOp(t, 1, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "cu"}), "cu"},
		{"concept.delete", mustOp(t, 1, OpConceptDelete, ConceptDeletePayload{ConceptID: "cd"}), "cd"},
		{"term.add", mustOp(t, 1, OpTermAdd, TermAddPayload{ConceptID: "ta"}), "ta"},
		{"term.update", mustOp(t, 1, OpTermUpdate, TermUpdatePayload{ConceptID: "tu"}), "tu"},
		{"term.remove", mustOp(t, 1, OpTermRemove, TermRemovePayload{ConceptID: "tr"}), "tr"},
		{"term.status", mustOp(t, 1, OpTermStatus, TermStatusPayload{ConceptID: "ts"}), "ts"},
		{"relation.add has no concept id", mustOp(t, 1, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{ID: "r1"}}), ""},
		{"voice.rule.add has no concept id", mustOp(t, 1, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1"}), ""},
	}
	for _, tt := range conceptIDCases {
		t.Run("conceptID/"+tt.name, func(t *testing.T) {
			op := tt.op
			op.BaseRev = 1
			c := CheckBaseRev(op, 2) // force a conflict so ConceptID is populated
			require.NotNil(t, c)
			assert.Equal(t, tt.want, c.ConceptID)
		})
	}
}

func TestEnumIsValid(t *testing.T) {
	assert.True(t, ObservationCompetitor.IsValid())
	assert.True(t, ObservationInternal.IsValid())
	assert.False(t, ObservationKind("nope").IsValid())

	assert.True(t, VerdictApprove.IsValid())
	assert.True(t, VerdictReject.IsValid())
	assert.False(t, ReviewVerdict("maybe").IsValid())

	for _, s := range []ChangeSetStatus{ChangeSetDraft, ChangeSetInReview, ChangeSetApproved, ChangeSetMerged, ChangeSetAbandoned} {
		assert.True(t, s.IsValid(), s)
	}
	assert.False(t, ChangeSetStatus("paused").IsValid())

	for _, o := range []OpType{
		OpConceptCreate, OpConceptUpdate, OpConceptDelete,
		OpTermAdd, OpTermUpdate, OpTermRemove, OpTermStatus,
		OpRelationAdd, OpRelationRemove, OpVoiceRuleAdd, OpVoiceRuleRemove,
	} {
		assert.True(t, o.IsValid(), o)
	}
	assert.False(t, OpType("concept.frobnicate").IsValid())
}
