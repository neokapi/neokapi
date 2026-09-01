// Package convergence is the framework-owned model of a project's localization
// convergence: the per-(collection, locale) target coverage and ship-gate
// standing, the source authoring readiness, and the review queue — the derived
// picture `kapi status`, `kapi status --review`, and `kapi check --ship` report.
//
// It owns the report TYPES and the per-block ladder helpers (the meaning of the
// draft→translated→reviewed→signed-off and authored→checked→approved ladders), so
// any surface — the CLI over files, a future server over its store — derives the
// same shape from the same rules (rolled up via core/gate). The IO-bound
// orchestration (resolving content units, reading blocks, loading review state)
// lives with each surface's IO, not here.
package convergence

import (
	"strings"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
)

// Report is the full derived convergence picture for a project.
type Report struct {
	Project string           `json:"project,omitempty"`
	Source  *SourceCoverage  `json:"source,omitempty"`
	Locales []LocaleCoverage `json:"locales"`
	Review  []ReviewItem     `json:"review"`
}

// LocaleCoverage is the ship-gate view for one (collection, locale) scope: the
// state distribution of its translatable units, whether it clears its gate, and
// which thresholds are still pending. Collection is empty for content not in a
// named collection (the common case), where the rows are effectively per-locale.
type LocaleCoverage struct {
	Locale     string           `json:"locale"`
	Collection string           `json:"collection,omitempty"`
	Total      int              `json:"total"`
	Pct        map[string]int   `json:"pct"`               // ladder state → "at least" percent (rounded)
	Gated      bool             `json:"gated"`             // a ship gate applies to this scope
	Shippable  bool             `json:"shippable"`         // ship gate satisfied (or no ship gate)
	Pending    []gate.Shortfall `json:"pending,omitempty"` // unmet ship-gate thresholds
	// ShipProgress is how far the scope has come toward its ship gate, in
	// [0,100] — gate.Progress: the mean fractional attainment of the gate's
	// required thresholds. It is a distance to the bar, not a lifecycle
	// percentage (those are in Pct), so it answers exactly one question: how
	// much of the ship gate is left. 100 for an ungated scope.
	ShipProgress int `json:"shipProgress"`
	// Blocking names the lowest unmet rung of the ship gate — the one gate to
	// clear next. Empty when the scope ships.
	Blocking string `json:"blocking,omitempty"`
	// Verified reports whether the scope clears its verified gate — the second,
	// independent bar meaning a person reviewed or signed off the content. It is
	// evaluated exactly like Shippable but against the recipe's verified gate.
	// With no verified gate configured for the scope, Verified is false (nothing
	// is verified by default): a shippable-but-unverified locale is flagged AI in
	// a language picker, a verified one carries no badge.
	Verified bool `json:"verified"`
	// AIReviewed counts units whose reviewed/signed-off rung was reached by an
	// autonomous AI decision ("ai/…" identity). They read as reviewed in Pct —
	// with an "(ai)" qualifier in displays — but do not satisfy a gate's
	// reviewed/signed-off threshold unless it says `by: any` (core/gate).
	AIReviewed int `json:"aiReviewed,omitempty"`
	// Stale counts units whose decision was recorded against source wording that
	// has since changed (state.UnitState.SourceStale). They tally at `draft` —
	// a target exists, but it is not a translation of the source the project has
	// now — and they hold the scope out of Shippable and Verified however the
	// percentages read: a gate is a bar on quantity, and shipping a translation
	// of a sentence that is gone is not a shortfall of quantity.
	Stale int `json:"stale,omitempty"`
	// FailingChecks counts produced units that fail the project's bound
	// target-side checks (placeholder and tag integrity, terminology). They
	// count at their true rung in Pct — the unit is translated, and a percentage
	// that denied it would be false — and they hold the scope out of Shippable
	// and Verified however the percentages read, whether or not a gate applies.
	//
	// It is populated only when the caller supplies the check findings; a
	// surface that does not run the checks reports 0 and would therefore call a
	// failing scope shippable. Every surface that publishes a verdict runs them.
	FailingChecks int `json:"failingChecks,omitempty"`
	// BasisUnknown counts units holding a decision recorded before its basis
	// was tracked. Such a decision says nothing about the source it blessed, so
	// it keeps its rung and the scope ships — the count is what makes that
	// assumption visible rather than silent.
	BasisUnknown int `json:"basisUnknown,omitempty"`
}

// SourceCoverage is the source-readiness view for the project: how far its source
// content has progressed along the authoring ladder (authored → checked →
// approved) and whether it clears the optional source gate. Source content is
// shared across all target locales, so this rolls up project-wide over the
// distinct source files (deduped), not per-locale.
type SourceCoverage struct {
	Total     int              `json:"total"`
	Pct       map[string]int   `json:"pct"`
	Gated     bool             `json:"gated"`
	Shippable bool             `json:"shippable"`
	Pending   []gate.Shortfall `json:"pending,omitempty"`
	// Unreadable names the formats this machine has no reader for, so the files
	// declaring them contributed nothing to the numbers above. A format can come
	// from a plugin, so a recipe that reads on one machine may not on another;
	// the rollup continues rather than aborting, and says so here. Empty is the
	// normal case. Never let this go unreported — coverage computed over content
	// that was never opened reads as progress.
	Unreadable []string `json:"unreadable,omitempty"`
}

// ReviewItem is one translatable unit awaiting human review (a translated unit not
// yet approved), with short previews for listing.
type ReviewItem struct {
	Locale string `json:"locale"`
	File   string `json:"file"`
	Key    string `json:"key"`
	// Collection is the parent content-collection name (empty for a bare
	// entry), so a review surface can filter the queue to one collection.
	Collection string `json:"collection,omitempty"`
	// Relative is the SOURCE file's project-relative path. File is the target
	// file, which is what a reviewer is looking at but not what a path filter
	// is written against: a filter says `web/**` about the content, and the
	// target of that content lives under a locale directory the filter never
	// mentions. Both halves have to reach a surface for it to scope a queue the
	// way it scopes everything else.
	Relative string `json:"relative,omitempty"`
	// SourceLocale is the project's source language, so a review surface can
	// render the source preview in its own writing direction.
	SourceLocale string `json:"sourceLocale,omitempty"`
	Source       string `json:"source"`           // short source preview
	Target       string `json:"target,omitempty"` // short target preview
	// HasFindings reports whether the unit currently has check findings —
	// enrichment a surface may add when it can compute findings cheaply. Nil
	// means "not computed" (the base derivation never runs checkers).
	HasFindings *bool `json:"hasFindings,omitempty"`
	// AIScore is the unit's AI pre-review score (0–100) when a fresh annotation
	// exists for the current translation; nil when none was recorded (queue
	// listing never calls a provider — the score is read from the state store).
	AIScore *int `json:"aiScore,omitempty"`
	// AIModel names the model that produced AIScore.
	AIModel string `json:"aiModel,omitempty"`
}

// Unit is a resolved content unit to measure: a source file paired with one
// target locale. The IO orchestration on each surface produces these.
type Unit struct {
	SourcePath  string
	TargetPath  string
	Locale      string
	Collection  string // parent content-collection (empty for a bare entry); ship gates resolve against (collection, locale)
	DisplayPath string // path reported in findings/queue (the target file, relative to root when possible)
}

// BlockKey is a block's stable unit identity: its Name when set, else its ID. It
// is the key the document cache, the overlays, and the state store all address a
// unit by.
//
// Not the same ladder as model.Block.ChainUnit, which prefers the structural
// address over the name and refuses to fall through to the ID. Both are right
// for their own question; see core/model/identity_ladders.go.
func BlockKey(b *model.Block) string {
	// A resolved unit wins, because it is the one of the three that is matched
	// rather than named — it survives a sibling being deleted, which is the
	// case the other two cannot answer. Absent, the reader's name is the best
	// available key, and the reader's id the last resort.
	if b.Unit != "" {
		return b.Unit
	}
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// BlockAddress is a block's TRANSLATION-INVARIANT unit identity, or "" when it
// has none. It is the key that pairs a source file with its translation: a
// structural name carries its ancestors' words, so the same paragraph is named
// in each document's own language, while the address writes each of those
// ancestors as its own structural identity instead
// (model.StructureAnnotation.Address).
//
// Empty is the ordinary case, not a defect. A format whose names carry no
// ancestor text — a key path, an element path, a catalog id — is already
// invariant, and BlockKey pairs it.
func BlockAddress(b *model.Block) string { return b.StructuralAddress() }

// TargetState derives a translatable block's target-lifecycle state for a locale.
// A committed Target.Status is authoritative; otherwise a present, non-empty
// target counts as `translated` (the presence baseline) and an absent/empty
// target is untranslated (below every rung).
//
// Presence is model.RunsHaveContent, so a target whose only run is an inline
// code counts as produced. Read through TargetText() it flattened to "" and the
// unit read as untranslated forever: below every rung, dragging its scope's
// coverage down, and so permanently short of the ship gate.
func TargetState(b *model.Block, locale string) string {
	loc := model.LocaleID(locale)
	t := b.Target(loc)
	if t == nil || !model.RunsHaveContent(b.TargetRuns(loc)) {
		return ""
	}
	if t.Status != "" {
		return string(t.Status)
	}
	return string(model.TargetStatusTranslated)
}

// SourceState derives a translatable block's source-authoring state: a committed
// SourceStatus is authoritative, else a present, non-empty source counts as
// `authored` (the presence baseline).
//
// Presence is model.RunsHaveContent — the same run-aware question the source
// readiness gate asks (check.NewSourceReadinessTool), so a placeholder-only unit
// is authored content here too rather than a hole in the source ladder.
func SourceState(b *model.Block) string {
	if !model.RunsHaveContent(b.SourceRuns()) {
		return ""
	}
	if b.SourceStatus != "" {
		return string(b.SourceStatus)
	}
	return string(model.SourceStatusAuthored)
}

// Preview trims a string to a short single-line preview for queue listings.
func Preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 72
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
