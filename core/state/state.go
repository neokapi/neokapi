// Package state is the project's authoritative workflow-state model — the record
// of decisions that are NOT derivable from the source/target content: the review
// ladder (draft→translated→reviewed→signed-off), who approved a unit and when,
// parking, and the content hash of the specific translation a decision blesses.
//
// This is distinct from the content memory (a recycle/leverage corpus, keyed
// by content) and from the document cache (a derived, rebuildable optimization).
// State is authored decision data: its durable home is a committed, diff-friendly
// serialization (the source of truth); a working store is a derived index over
// it. See strategy/content-cache/project-state-model.md.
package state

import "github.com/neokapi/neokapi/core/model"

// UnitState is the workflow state of one translatable unit in one locale variant.
type UnitState struct {
	// Unit is the unit identity — the block's content hash / stable id, the same
	// key the document cache and overlays address it by.
	Unit string `json:"unit"`
	// Variant is the locale (and optional tone/channel) this state applies to.
	Variant model.VariantKey `json:"variant"`
	// Status is the target ladder position (draft→translated→reviewed→signed-off).
	Status model.TargetStatus `json:"status,omitempty"`
	// SourceStatus is the source ladder position (authored→checked→approved).
	SourceStatus model.SourceStatus `json:"sourceStatus,omitempty"`
	// Origin is the provenance of the current target (engine/tool/reference).
	Origin model.Origin `json:"origin,omitzero"`
	// TargetHash is the content hash of the translation this state blesses, so an
	// edit to the translation invalidates a stale decision (e.g. an approval). It
	// does NOT duplicate the translation text — that lives in the deliverable.
	TargetHash string `json:"targetHash,omitempty"`
	// Decision is the human/agent workflow decision recorded for the unit.
	Decision Decision `json:"decision,omitzero"`
	// AIReview is the last AI pre-review annotation for the unit — advisory
	// only, never a decision. It informs the review queue (score + findings);
	// an auto-approval it justified is recorded separately in Decision, with
	// By carrying the "ai/<model>" identity.
	AIReview *AIReview `json:"aiReview,omitempty"`
	// Updated is when this record last changed (RFC 3339).
	Updated string `json:"updated,omitempty"`

	// Scope, ContentHash and ContextHash are the identity signals core/reconcile
	// matches a unit by when a source file is re-read.
	//
	// They live here rather than in a separate ledger because there is nothing to
	// separate: the thing a decision is recorded against and the thing identity
	// is matched on are the same unit. Keeping them together means a decision and
	// the evidence for which block it belongs to cannot drift apart, and it is
	// what lets a block removed in one revision and restored in a later one come
	// back to its own history instead of being re-translated.
	//
	// Scope is the document's resolved key, never its path, so renaming a file
	// does not disturb the units inside it.
	Scope       string `json:"scope,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
	ContextHash string `json:"contextHash,omitempty"`
}

// Decision is the authored workflow decision recorded for a unit.
type Decision struct {
	ReviewState string `json:"reviewState,omitempty"` // approved | rejected | …
	// By is the decider's identity. Empty for a plain human decision (the
	// single-player default), "ai/<model>" for an autonomous AI approval
	// (e.g. pre-review auto-approve), "agent/<client>" for an MCP agent
	// acting on a person's behalf.
	By       string `json:"by,omitempty"`
	At       string `json:"at,omitempty"` // RFC 3339
	Note     string `json:"note,omitempty"`
	Parked   bool   `json:"parked,omitempty"`
	Assignee string `json:"assignee,omitempty"`
}

// AIIdentityPrefix marks a decision made autonomously by an AI reviewer.
// Decisions with this prefix count toward reviewed/signed-off gate thresholds
// only when the gate's approver class is "any" (core/gate).
const AIIdentityPrefix = "ai/"

// IsAIDecision reports whether an identity string names an autonomous AI
// decider (prefixed "ai/"). Agent identities ("agent/…") are NOT AI decisions:
// they act on a person's behalf.
func IsAIDecision(by string) bool {
	return len(by) >= len(AIIdentityPrefix) && by[:len(AIIdentityPrefix)] == AIIdentityPrefix
}

// AIReview is an advisory AI pre-review annotation: the structured output of
// the ai review tool ({score 0-100, findings}), bound to the translation it
// judged so an edit invalidates it. It never moves the unit on the ladder.
type AIReview struct {
	Score int `json:"score"`
	// Model identifies the reviewer model (the "<model-id>" the "ai/<model-id>"
	// decision identity would carry).
	Model    string            `json:"model,omitempty"`
	Findings []AIReviewFinding `json:"findings,omitempty"`
	// TargetHash is the content hash of the translation this review judged.
	TargetHash string `json:"targetHash,omitempty"`
	At         string `json:"at,omitempty"` // RFC 3339
}

// AIReviewFinding is one issue an AI review reported (mirrors the review tool's
// output contract).
type AIReviewFinding struct {
	Severity   string `json:"severity,omitempty"` // critical | major | minor | info
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Fresh reports whether the review still judges the given translation content.
// An unset hash on either side is treated as fresh (no content to compare).
func (r *AIReview) Fresh(targetHash string) bool {
	if r == nil {
		return false
	}
	return r.TargetHash == "" || targetHash == "" || r.TargetHash == targetHash
}

// Key uniquely identifies a UnitState within a project.
type Key struct {
	Unit    string
	Variant model.VariantKey
}

// Key returns the unit's identity key.
func (s UnitState) Key() Key { return Key{Unit: s.Unit, Variant: s.Variant} }

// Stale reports whether this state was recorded against a different translation
// than targetHash — i.e. the translation changed since the decision, so the
// decision (an approval/sign-off) no longer applies and the unit drops back down
// the ladder. An unset TargetHash on either side is treated as "not stale" (no
// content to compare).
func (s UnitState) Stale(targetHash string) bool {
	return s.TargetHash != "" && targetHash != "" && s.TargetHash != targetHash
}

// Reviewed reports whether the unit is at or above the reviewed rung for a fresh
// translation (its decision blesses the given target content, not a stale one).
func (s UnitState) Reviewed(targetHash string) bool {
	if s.Stale(targetHash) {
		return false
	}
	return s.Status == model.TargetStatusReviewed || s.Status == model.TargetStatusSignedOff
}
