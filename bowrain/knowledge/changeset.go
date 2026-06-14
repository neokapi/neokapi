package knowledge

import (
	"encoding/json"
	"fmt"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// ---------------------------------------------------------------------------
// Per-op payload structs
//
// Each change-set op carries an op-specific JSON payload (ChangeSetOp.Payload).
// The structs below are the decoded forms; they are self-contained so an op can
// be re-validated and re-applied at merge without external lookups beyond the
// current concept revision.
// ---------------------------------------------------------------------------

// VoiceRuleList names the vocabulary list a voice rule belongs to.
type VoiceRuleList = string

const (
	VoiceListPreferred  VoiceRuleList = "preferred"
	VoiceListForbidden  VoiceRuleList = "forbidden"
	VoiceListCompetitor VoiceRuleList = "competitor"
)

// isVoiceRuleList reports whether list is one of the three vocabulary lists.
func isVoiceRuleList(list string) bool {
	switch list {
	case VoiceListPreferred, VoiceListForbidden, VoiceListCompetitor:
		return true
	default:
		return false
	}
}

// ConceptCreatePayload is the payload for OpConceptCreate: a full concept to
// insert into the workspace graph.
type ConceptCreatePayload struct {
	Concept termbase.Concept `json:"concept"`
}

// ConceptUpdatePayload is the payload for OpConceptUpdate: a partial edit to a
// concept's ordinary metadata. Nil pointers leave a field unchanged.
type ConceptUpdatePayload struct {
	ConceptID  string            `json:"concept_id"`
	Domain     *string           `json:"domain,omitempty"`
	Definition *string           `json:"definition,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// ConceptDeletePayload is the payload for OpConceptDelete (governed).
type ConceptDeletePayload struct {
	ConceptID string `json:"concept_id"`
}

// TermAddPayload is the payload for OpTermAdd: a new term on an existing concept.
type TermAddPayload struct {
	ConceptID string        `json:"concept_id"`
	Term      termbase.Term `json:"term"`
}

// TermUpdatePayload is the payload for OpTermUpdate: locale+text identify the
// existing term; Term carries its new field values.
type TermUpdatePayload struct {
	ConceptID string         `json:"concept_id"`
	Locale    model.LocaleID `json:"locale"`
	Text      string         `json:"text"`
	Term      termbase.Term  `json:"term"`
}

// TermRemovePayload is the payload for OpTermRemove: locale+text identify the
// term to remove.
type TermRemovePayload struct {
	ConceptID string         `json:"concept_id"`
	Locale    model.LocaleID `json:"locale"`
	Text      string         `json:"text"`
}

// TermStatusPayload is the payload for OpTermStatus: a term status transition,
// governed when termbase.IsGovernedTransition(From, To) is true. Validity
// optionally scopes the new status to a market/time window.
type TermStatusPayload struct {
	ConceptID string           `json:"concept_id"`
	Locale    model.LocaleID   `json:"locale"`
	Text      string           `json:"text"`
	From      model.TermStatus `json:"from"`
	To        model.TermStatus `json:"to"`
	Validity  *graph.Validity  `json:"validity,omitempty"`
}

// RelationAddPayload is the payload for OpRelationAdd: a full relation, governed
// when its RelationType is graph.LabelReplacedBy.
type RelationAddPayload struct {
	Relation termbase.ConceptRelation `json:"relation"`
}

// RelationRemovePayload is the payload for OpRelationRemove.
type RelationRemovePayload struct {
	RelationID string `json:"relation_id"`
}

// VoiceRuleAddPayload is the payload for OpVoiceRuleAdd (always governed). Rule
// is a brand vocabulary rule that references the backing concept by ID; List
// selects which of the profile's vocabulary lists it joins.
type VoiceRuleAddPayload struct {
	ProfileID string             `json:"profile_id"`
	List      VoiceRuleList      `json:"list"`
	Rule      corebrand.TermRule `json:"rule"`
}

// VoiceRuleRemovePayload is the payload for OpVoiceRuleRemove (always governed).
type VoiceRuleRemovePayload struct {
	ProfileID string        `json:"profile_id"`
	List      VoiceRuleList `json:"list"`
	Term      string        `json:"term"`
}

// decodePayload unmarshals an op's payload into v, wrapping the op type into
// any error.
func decodePayload(op ChangeSetOp, v any) error {
	if len(op.Payload) == 0 {
		return fmt.Errorf("%s: empty payload", op.Op)
	}
	if err := json.Unmarshal(op.Payload, v); err != nil {
		return fmt.Errorf("%s: decode payload: %w", op.Op, err)
	}
	return nil
}

// requireField returns a "required field" error for an op.
func requireField(op OpType, field string) error {
	return fmt.Errorf("%s: %s is required", op, field)
}

// ---------------------------------------------------------------------------
// Op validation
// ---------------------------------------------------------------------------

// ValidateOp decodes an op's payload according to its OpType and checks that
// the required fields are present and well-formed. An unknown OpType is an
// error. It validates structure only — whether the op is permitted (governed)
// is answered separately by IsGovernedOp.
func ValidateOp(op ChangeSetOp) error {
	switch op.Op {
	case OpConceptCreate:
		var p ConceptCreatePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.Concept.ID == "" {
			return requireField(op.Op, "concept.id")
		}
		for _, t := range p.Concept.Terms {
			if t.Status != "" && !termbase.KnownTermStatus(t.Status) {
				return fmt.Errorf("%s: term %q (%s): unknown status %q", op.Op, t.Text, t.Locale, t.Status)
			}
		}
		return nil

	case OpConceptUpdate:
		var p ConceptUpdatePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ConceptID == "" {
			return requireField(op.Op, "concept_id")
		}
		return nil

	case OpConceptDelete:
		var p ConceptDeletePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ConceptID == "" {
			return requireField(op.Op, "concept_id")
		}
		return nil

	case OpTermAdd:
		var p TermAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ConceptID == "" {
			return requireField(op.Op, "concept_id")
		}
		if p.Term.Text == "" {
			return requireField(op.Op, "term.text")
		}
		if p.Term.Locale == "" {
			return requireField(op.Op, "term.locale")
		}
		if p.Term.Status != "" && !termbase.KnownTermStatus(p.Term.Status) {
			return fmt.Errorf("%s: unknown term status %q", op.Op, p.Term.Status)
		}
		return nil

	case OpTermUpdate:
		var p TermUpdatePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ConceptID == "" {
			return requireField(op.Op, "concept_id")
		}
		if p.Locale == "" {
			return requireField(op.Op, "locale")
		}
		if p.Text == "" {
			return requireField(op.Op, "text")
		}
		if p.Term.Status != "" && !termbase.KnownTermStatus(p.Term.Status) {
			return fmt.Errorf("%s: unknown term status %q", op.Op, p.Term.Status)
		}
		return nil

	case OpTermRemove:
		var p TermRemovePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ConceptID == "" {
			return requireField(op.Op, "concept_id")
		}
		if p.Locale == "" {
			return requireField(op.Op, "locale")
		}
		if p.Text == "" {
			return requireField(op.Op, "text")
		}
		return nil

	case OpTermStatus:
		var p TermStatusPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ConceptID == "" {
			return requireField(op.Op, "concept_id")
		}
		if p.Locale == "" {
			return requireField(op.Op, "locale")
		}
		if p.Text == "" {
			return requireField(op.Op, "text")
		}
		if err := termbase.ValidateTransition(p.From, p.To); err != nil {
			return fmt.Errorf("%s: %w", op.Op, err)
		}
		return nil

	case OpRelationAdd:
		var p RelationAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if err := termbase.ValidateRelation(p.Relation); err != nil {
			return fmt.Errorf("%s: %w", op.Op, err)
		}
		return nil

	case OpRelationRemove:
		var p RelationRemovePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.RelationID == "" {
			return requireField(op.Op, "relation_id")
		}
		return nil

	case OpVoiceRuleAdd:
		var p VoiceRuleAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ProfileID == "" {
			return requireField(op.Op, "profile_id")
		}
		if !isVoiceRuleList(p.List) {
			return fmt.Errorf("%s: unknown list %q (want preferred|forbidden|competitor)", op.Op, p.List)
		}
		if p.Rule.Term == "" {
			return requireField(op.Op, "rule.term")
		}
		return nil

	case OpVoiceRuleRemove:
		var p VoiceRuleRemovePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if p.ProfileID == "" {
			return requireField(op.Op, "profile_id")
		}
		if !isVoiceRuleList(p.List) {
			return fmt.Errorf("%s: unknown list %q (want preferred|forbidden|competitor)", op.Op, p.List)
		}
		if p.Term == "" {
			return requireField(op.Op, "term")
		}
		return nil

	default:
		return fmt.Errorf("unknown op type: %q", op.Op)
	}
}

// ---------------------------------------------------------------------------
// Governed / ordinary classification
// ---------------------------------------------------------------------------

// IsGovernedOp reports whether an op is governed — that is, whether it may only
// reach the live graph through a reviewed change-set (AD-021). Governed ops:
// a term status transition that termbase.IsGovernedTransition flags, a
// REPLACED_BY relation, a concept deletion, and any voice-rule change. Every
// other op is ordinary. An unknown OpType is an error.
func IsGovernedOp(op ChangeSetOp) (bool, error) {
	switch op.Op {
	case OpTermStatus:
		var p TermStatusPayload
		if err := decodePayload(op, &p); err != nil {
			return false, err
		}
		return termbase.IsGovernedTransition(p.From, p.To), nil

	case OpRelationAdd:
		var p RelationAddPayload
		if err := decodePayload(op, &p); err != nil {
			return false, err
		}
		return p.Relation.RelationType == graph.LabelReplacedBy, nil

	case OpConceptDelete, OpVoiceRuleAdd, OpVoiceRuleRemove:
		return true, nil

	case OpConceptCreate, OpConceptUpdate,
		OpTermAdd, OpTermUpdate, OpTermRemove, OpRelationRemove:
		return false, nil

	default:
		return false, fmt.Errorf("unknown op type: %q", op.Op)
	}
}

// ChangeSetIsGoverned reports whether a change-set contains any governed op. A
// governed change-set requires in_review → approved (with an approval from
// someone other than its author) before merge; an ordinary change-set may merge
// directly from draft by its author.
func ChangeSetIsGoverned(ops []ChangeSetOp) (bool, error) {
	for _, op := range ops {
		governed, err := IsGovernedOp(op)
		if err != nil {
			return false, err
		}
		if governed {
			return true, nil
		}
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// Change-set state machine
// ---------------------------------------------------------------------------

// allowedChangeSetTransitions is the change-set lifecycle, as an adjacency map
// of from → set-of-allowed-to. merged and abandoned are terminal (absent as
// keys). draft → merged is the ordinary fast-path: an ungoverned change-set
// merges straight from draft. The separation-of-duties and governed gates are
// enforced by CanMerge, which a merging service must consult before
// SetMergeResult — this state machine only forbids double-merges and exits from
// terminal states, never the gate itself. Reject re-opens in_review → draft;
// approved → in_review re-opens for more review.
var allowedChangeSetTransitions = map[ChangeSetStatus]map[ChangeSetStatus]struct{}{
	ChangeSetDraft: {
		ChangeSetInReview:  {},
		ChangeSetMerged:    {},
		ChangeSetAbandoned: {},
	},
	ChangeSetInReview: {
		ChangeSetApproved:  {},
		ChangeSetDraft:     {},
		ChangeSetAbandoned: {},
	},
	ChangeSetApproved: {
		ChangeSetMerged:    {},
		ChangeSetAbandoned: {},
		ChangeSetInReview:  {},
	},
}

// ValidateStatusTransition checks that a change-set may move from one status to
// another. The only allowed edges are draft→in_review, draft→merged,
// draft→abandoned, in_review→approved, in_review→draft, in_review→abandoned,
// approved→merged, approved→abandoned, and approved→in_review. merged and
// abandoned are terminal. Every other pairing — including a no-op same→same and
// unknown statuses — is rejected with a descriptive error. This guards the
// lifecycle shape only; the merge gate (separation of duties, governed approval)
// lives in CanMerge.
func ValidateStatusTransition(from, to ChangeSetStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid change-set status %q", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid change-set status %q", to)
	}
	if from == to {
		return fmt.Errorf("change-set is already in status %q", from)
	}
	if _, ok := allowedChangeSetTransitions[from][to]; ok {
		return nil
	}
	return fmt.Errorf("invalid change-set transition: %s → %s", from, to)
}

// CanMerge reports whether a change-set may be merged given its governed
// classification and its reviews. A governed change-set must be approved and
// carry at least one approve verdict from a reviewer other than its author
// (separation of duties) — self-approval never satisfies the gate. An ordinary
// change-set may merge while it is still a draft or once approved. A nil return
// means merge is permitted; otherwise the error explains why not.
func CanMerge(cs ChangeSet, governed bool, reviews []ChangeSetReview) error {
	if governed {
		if cs.Status != ChangeSetApproved {
			return fmt.Errorf("governed change-set must be approved before merge (status %q)", cs.Status)
		}
		for _, r := range reviews {
			if r.Verdict == VerdictApprove && r.Reviewer != cs.CreatedBy {
				return nil
			}
		}
		return fmt.Errorf("governed change-set requires an approval from a reviewer other than its author %q (separation of duties)", cs.CreatedBy)
	}
	switch cs.Status {
	case ChangeSetDraft, ChangeSetApproved:
		return nil
	default:
		return fmt.Errorf("ordinary change-set can only merge from draft or approved (status %q)", cs.Status)
	}
}

// ---------------------------------------------------------------------------
// Conflict detection
// ---------------------------------------------------------------------------

// OpConflict describes a stale-draft conflict: an op was authored against a
// concept revision that is no longer current, so applying it would clobber an
// intervening edit. Re-basing is: reopen the op, re-validate, resubmit.
type OpConflict struct {
	Seq       int64  `json:"seq"`
	ConceptID string `json:"concept_id"`
	Reason    string `json:"reason"`
}

// CheckBaseRev compares an op's BaseRev against the concept's current revision.
// A BaseRev of 0 means the op was not pinned to a revision and never conflicts.
// Otherwise a mismatch yields a non-nil OpConflict (with the concept ID
// extracted from the payload on a best-effort basis); a match yields nil.
func CheckBaseRev(op ChangeSetOp, currentRev int64) *OpConflict {
	if op.BaseRev == 0 || op.BaseRev == currentRev {
		return nil
	}
	return &OpConflict{
		Seq:       op.Seq,
		ConceptID: conceptIDOf(op),
		Reason:    fmt.Sprintf("op authored against revision %d but concept is at revision %d", op.BaseRev, currentRev),
	}
}

// conceptIDOf extracts the concept ID an op targets, best-effort: it decodes
// the payload and returns the concept ID, or "" for ops that do not name a
// concept (relation ops, voice-rule ops) or whose payload fails to decode.
func conceptIDOf(op ChangeSetOp) string {
	switch op.Op {
	case OpConceptCreate:
		var p ConceptCreatePayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.Concept.ID
		}
	case OpConceptUpdate:
		var p ConceptUpdatePayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.ConceptID
		}
	case OpConceptDelete:
		var p ConceptDeletePayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.ConceptID
		}
	case OpTermAdd:
		var p TermAddPayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.ConceptID
		}
	case OpTermUpdate:
		var p TermUpdatePayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.ConceptID
		}
	case OpTermRemove:
		var p TermRemovePayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.ConceptID
		}
	case OpTermStatus:
		var p TermStatusPayload
		if json.Unmarshal(op.Payload, &p) == nil {
			return p.ConceptID
		}
	}
	return ""
}
