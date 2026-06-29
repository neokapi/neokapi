// Package state is the project's authoritative workflow-state model — the record
// of decisions that are NOT derivable from the source/target content: the review
// ladder (draft→translated→reviewed→signed-off), who approved a unit and when,
// parking, and the content hash of the specific translation a decision blesses.
//
// This is distinct from the translation memory (a recycle/leverage corpus, keyed
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
	// Updated is when this record last changed (RFC 3339).
	Updated string `json:"updated,omitempty"`
}

// Decision is the authored workflow decision recorded for a unit.
type Decision struct {
	ReviewState string `json:"reviewState,omitempty"` // approved | rejected | …
	By          string `json:"by,omitempty"`
	At          string `json:"at,omitempty"` // RFC 3339
	Note        string `json:"note,omitempty"`
	Parked      bool   `json:"parked,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
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
