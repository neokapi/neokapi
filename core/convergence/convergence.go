// Package convergence is the framework-owned model of a project's localization
// convergence: the per-(collection, locale) target coverage and ship-gate
// standing, the source authoring readiness, and the review queue — the derived
// picture `kapi status`, `kapi status --review`, and `kapi verify --ship` report.
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
	Shippable  bool             `json:"shippable"`         // gate satisfied (or no gate)
	Pending    []gate.Shortfall `json:"pending,omitempty"` // unmet gate thresholds
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
}

// ReviewItem is one translatable unit awaiting human review (a translated unit not
// yet approved), with short previews for listing.
type ReviewItem struct {
	Locale string `json:"locale"`
	File   string `json:"file"`
	Key    string `json:"key"`
	Source string `json:"source"`           // short source preview
	Target string `json:"target,omitempty"` // short target preview
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
func BlockKey(b *model.Block) string {
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// TargetState derives a translatable block's target-lifecycle state for a locale.
// A committed Target.Status is authoritative; otherwise a present, non-empty
// target counts as `translated` (the presence baseline) and an absent/empty
// target is untranslated (below every rung).
func TargetState(b *model.Block, locale string) string {
	loc := model.LocaleID(locale)
	t := b.Target(loc)
	if t == nil || strings.TrimSpace(b.TargetText(loc)) == "" {
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
func SourceState(b *model.Block) string {
	if strings.TrimSpace(b.SourceText()) == "" {
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
