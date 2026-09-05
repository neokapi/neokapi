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

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// TargetHash is the content hash a decision blesses: the hash of the trimmed
// target text. One definition, used by every party that records or verifies a
// decision — the host when it records, a server when it checks freshness at
// ingest. A second implementation of this composition is how two ends of a
// protocol drift.
func TargetHash(targetText string) string {
	return project.HashBytes([]byte(strings.TrimSpace(targetText)))
}

// SourceHash is the BASIS a decision blesses: the hash of the source wording a
// reviewer had in front of them. It is model.ComputeContentHash — the same
// normalization core/reconcile matches identity on — so a unit's basis and its
// identity signal are one number, and a decision recorded on one path is
// comparable with a source read on another.
//
// Target hash and basis are the two halves of one pairing: an approval is about
// a specific translation OF a specific source. Editing the translation
// invalidates the decision through TargetHash; editing the source invalidates it
// through this one.
func SourceHash(sourceText string) string {
	return model.ComputeContentHash(sourceText)
}

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
	// ContentHash is therefore also the decision's BASIS: the source wording the
	// decision blessed (state.SourceHash). A record whose basis no longer matches
	// the unit's current source is stale — see SourceStale — which is what stops
	// an approval outliving the sentence it approved.
	//
	// Scope is the document's resolved key, never its path, so renaming a file
	// does not disturb the units inside it. It is also half of the record's
	// identity — see Key.
	Scope       string `json:"scope,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
	ContextHash string `json:"contextHash,omitempty"`

	// GoverningFingerprint is the governing context this record's answer stands
	// under: tool.ContextFingerprint over the voice guidance and the term rules
	// in force at the unit's point for its locale, the same value every
	// translation producer stamps on model.Origin.ContextFingerprint. A decision
	// records the context the decider approved the translation under; a basis
	// the loop wrote for its own output records the producer's stamp. Empty when
	// that context was ungoverned (no voice, no terms) or when the record was
	// written before the field existed.
	//
	// It is a different quantity from ContextHash, and neither is derivable
	// from the other. ContextHash identifies WHICH block this is
	// (model.ComputeContextHash over the block's name, type and properties) and
	// moves when the block's surroundings move; GoverningFingerprint identifies
	// WHAT GOVERNED the answer and moves when the voice or the terminology
	// moves. Either changes with the other holding still.
	//
	// The record is the durable carrier of this value. A target file in a
	// format with a slot for provenance carries the same stamp in flight, but
	// most delivered formats (a JSON catalog, a .properties file) hold strings
	// and nothing else, so a reader pairing such a file with its source finds
	// what governed the answer here or nowhere.
	GoverningFingerprint string `json:"governingFingerprint,omitempty"`
}

// GoverningContext is the governing context the record's answer stands under:
// GoverningFingerprint where the record carries one, and otherwise the
// producer's own stamp on Origin, which is all a record written before the
// field existed has to say about it. Empty reads as ungoverned.
func (s UnitState) GoverningContext() string {
	if s.GoverningFingerprint != "" {
		return s.GoverningFingerprint
	}
	return s.Origin.ContextFingerprint
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
//
// Scope is part of the identity, not a payload beside it. A unit id is unique
// inside its document and nowhere wider — a reader names blocks by what the
// format gives it, so every markdown page in a collection carries an `h`, a `p`
// and an `fm_title`. Keyed on (unit, variant) alone, the second page's decision
// overwrote the first's: a reviewer's approvals were accepted, reported applied,
// and all but one document's silently discarded.
type Key struct {
	Scope   string
	Unit    string
	Variant model.VariantKey
}

// Key returns the unit's identity key.
func (s UnitState) Key() Key { return Key{Scope: s.Scope, Unit: s.Unit, Variant: s.Variant} }

// Stale reports whether this state was recorded against a different translation
// than targetHash — i.e. the translation changed since the decision, so the
// decision (an approval/sign-off) no longer applies and the unit drops back down
// the ladder. An unset TargetHash on either side is treated as "not stale" (no
// content to compare).
func (s UnitState) Stale(targetHash string) bool {
	return s.TargetHash != "" && targetHash != "" && s.TargetHash != targetHash
}

// SourceStale reports whether this state was recorded against different SOURCE
// wording than contentHash — the basis it blessed is gone, so the translation
// under it renders a sentence the project no longer has and the decision no
// longer applies.
//
// An unset basis on either side is NOT stale but UNKNOWN: a record written
// before the basis was tracked says nothing about the source it blessed, and
// reading silence as drift would demote every decision a project already holds.
// Such a record keeps its rung and is counted as unknown where that count is
// reported, until the next decision on the unit supplies a basis.
func (s UnitState) SourceStale(contentHash string) bool {
	return s.ContentHash != "" && contentHash != "" && s.ContentHash != contentHash
}

// Fresh reports whether the decision still applies to the pairing in front of
// the reader: the translation it blessed and the source it blessed it for.
func (s UnitState) Fresh(targetHash, contentHash string) bool {
	return !s.Stale(targetHash) && !s.SourceStale(contentHash)
}

// Reviewed reports whether the unit is at or above the reviewed rung for a fresh
// translation (its decision blesses the given target content, not a stale one).
func (s UnitState) Reviewed(targetHash string) bool {
	if s.Stale(targetHash) {
		return false
	}
	return s.Status == model.TargetStatusReviewed || s.Status == model.TargetStatusSignedOff
}
