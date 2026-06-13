// Package knowledge implements the governance and collaboration layer of the
// brand knowledge graph (AD-021): markets, observations, comments, concept
// revisions, change-sets with reviews and pilots, and the change-set state
// machine. The framework (Apache) owns the concept/term/relation model and the
// status-transition policy; this package (AGPL) owns the workspace-scoped
// governance that wraps it. It is server-side and PostgreSQL-backed; the
// change-set state machine, op validation, governed/ordinary classification,
// the separation-of-duties merge gate, and conflict detection are pure
// functions in changeset.go, unit-tested without a database.
package knowledge

import (
	"encoding/json"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// ObservationKind classifies the external evidence an observation records.
type ObservationKind string

const (
	ObservationCompetitor ObservationKind = "competitor"  // a competitor's phrasing
	ObservationCustomer   ObservationKind = "customer"    // customer language (support, reviews)
	ObservationStyleGuide ObservationKind = "style_guide" // a style-guide citation
	ObservationRegulatory ObservationKind = "regulatory"  // a regulatory requirement
	ObservationWeb        ObservationKind = "web"         // a public-web usage
	ObservationInternal   ObservationKind = "internal"    // an internal note or decision
)

// IsValid reports whether k is one of the known observation kinds.
func (k ObservationKind) IsValid() bool {
	switch k {
	case ObservationCompetitor, ObservationCustomer, ObservationStyleGuide,
		ObservationRegulatory, ObservationWeb, ObservationInternal:
		return true
	default:
		return false
	}
}

// ReviewVerdict is the outcome a reviewer records on a change-set.
type ReviewVerdict string

const (
	VerdictApprove ReviewVerdict = "approve"
	VerdictReject  ReviewVerdict = "reject"
)

// IsValid reports whether v is one of the known review verdicts.
func (v ReviewVerdict) IsValid() bool {
	return v == VerdictApprove || v == VerdictReject
}

// ChangeSetStatus is the lifecycle state of a change-set. merged and abandoned
// are terminal. See ValidateStatusTransition for the allowed edges.
type ChangeSetStatus string

const (
	ChangeSetDraft     ChangeSetStatus = "draft"
	ChangeSetInReview  ChangeSetStatus = "in_review"
	ChangeSetApproved  ChangeSetStatus = "approved"
	ChangeSetMerged    ChangeSetStatus = "merged"
	ChangeSetAbandoned ChangeSetStatus = "abandoned"
)

// IsValid reports whether s is one of the known change-set statuses.
func (s ChangeSetStatus) IsValid() bool {
	switch s {
	case ChangeSetDraft, ChangeSetInReview, ChangeSetApproved,
		ChangeSetMerged, ChangeSetAbandoned:
		return true
	default:
		return false
	}
}

// OpType identifies one of the eleven change-set operations (AD-021). Each op
// is self-contained: it carries an op-specific JSON payload and is re-validated
// at merge against the concept's current revision.
type OpType string

const (
	OpConceptCreate   OpType = "concept.create"
	OpConceptUpdate   OpType = "concept.update"
	OpConceptDelete   OpType = "concept.delete"
	OpTermAdd         OpType = "term.add"
	OpTermUpdate      OpType = "term.update"
	OpTermRemove      OpType = "term.remove"
	OpTermStatus      OpType = "term.status"
	OpRelationAdd     OpType = "relation.add"
	OpRelationRemove  OpType = "relation.remove"
	OpVoiceRuleAdd    OpType = "voice.rule.add"
	OpVoiceRuleRemove OpType = "voice.rule.remove"
)

// IsValid reports whether o is one of the eleven known op types.
func (o OpType) IsValid() bool {
	switch o {
	case OpConceptCreate, OpConceptUpdate, OpConceptDelete,
		OpTermAdd, OpTermUpdate, OpTermRemove, OpTermStatus,
		OpRelationAdd, OpRelationRemove,
		OpVoiceRuleAdd, OpVoiceRuleRemove:
		return true
	default:
		return false
	}
}

// Market is a workspace-defined scope — a name plus the locales it covers (for
// example "dach" covering de-DE, de-AT, de-CH). Markets give the free validity
// tags on terms and relations a stable vocabulary.
type Market struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Locales     []model.LocaleID `json:"locales"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// Observation attaches external evidence to a concept: a competitor's phrasing,
// customer language, a style-guide citation, a regulatory requirement. It is
// evidence, not a rule — it informs proposals and appears in the concept story
// but enforces nothing.
type Observation struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	ConceptID   string          `json:"concept_id"`
	Kind        ObservationKind `json:"kind"`
	Quote       string          `json:"quote"`
	Source      string          `json:"source"`
	URL         string          `json:"url,omitempty"`
	Locale      model.LocaleID  `json:"locale,omitempty"`
	Market      string          `json:"market,omitempty"`
	Note        string          `json:"note,omitempty"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Comment is a threaded discussion entry attached to a concept or, when
// ChangesetID is set, to a change-set under review. Resolved threads remain
// part of the concept story.
type Comment struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	ConceptID   string    `json:"concept_id"`
	ParentID    string    `json:"parent_id,omitempty"`    // threaded; empty = top-level
	ChangesetID string    `json:"changeset_id,omitempty"` // set when the thread belongs to a change-set
	Body        string    `json:"body"`
	Author      string    `json:"author"`
	CreatedAt   time.Time `json:"created_at"`
	Resolved    bool      `json:"resolved"`
}

// ConceptRevision is an immutable snapshot produced every time a concept is
// edited. Revisions form the spine of the concept story and the base against
// which a change-set op's BaseRev is validated at merge.
type ConceptRevision struct {
	WorkspaceID string          `json:"workspace_id"`
	ConceptID   string          `json:"concept_id"`
	Rev         int64           `json:"rev"`
	Snapshot    json.RawMessage `json:"snapshot"` // termbase.Concept + relations delta
	Summary     string          `json:"summary,omitempty"`
	Actor       string          `json:"actor"`
	ChangesetID string          `json:"changeset_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ChangeSet is a named, reviewable draft of edits to the graph and to brand
// voice vocabulary. Ops accumulate in the draft; nothing touches the live
// graph until merge. It moves through draft → in_review → approved → merged,
// or is abandoned.
type ChangeSet struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Status      ChangeSetStatus `json:"status"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	SubmittedAt *time.Time      `json:"submitted_at,omitempty"`
	MergedAt    *time.Time      `json:"merged_at,omitempty"`
	MergedBy    string          `json:"merged_by,omitempty"`
}

// ChangeSetOp is a single ordered operation within a change-set. Seq orders ops
// within a change-set; Payload carries the op-specific JSON (see the payload
// structs in changeset.go); BaseRev records the concept revision the op was
// authored against, for stale-draft conflict detection at merge.
type ChangeSetOp struct {
	WorkspaceID string          `json:"workspace_id"`
	ChangesetID string          `json:"changeset_id"`
	Seq         int64           `json:"seq"`
	Op          OpType          `json:"op"`
	Payload     json.RawMessage `json:"payload"`
	BaseRev     int64           `json:"base_rev"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ChangeSetReview records a reviewer's verdict on a change-set. The merge gate
// requires at least one approve from a reviewer other than the author
// (separation of duties) for governed change-sets.
type ChangeSetReview struct {
	WorkspaceID string        `json:"workspace_id"`
	ChangesetID string        `json:"changeset_id"`
	Reviewer    string        `json:"reviewer"`
	Verdict     ReviewVerdict `json:"verdict"`
	Comment     string        `json:"comment,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

// Pilot binds a change-set to a content stream so real content and real checks
// resolve through the draft (a stream-scoped shadow over the workspace graph)
// before it merges. Merging or abandoning the change-set retires the shadow.
type Pilot struct {
	WorkspaceID string    `json:"workspace_id"`
	ChangesetID string    `json:"changeset_id"`
	ProjectID   string    `json:"project_id"`
	Stream      string    `json:"stream"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}
