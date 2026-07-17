package model

// SourceStatus is the authoring lifecycle state of a Block's source content —
// the source-side counterpart of TargetStatus. Where a Target progresses
// draft → translated → reviewed → signed-off, a source progresses
// authored → checked → approved: written, then cleared of brand/terminology
// findings, then signed off by a human. It is what keeps the author "in check"
// — the source equivalent of translation review.
type SourceStatus string

const (
	// SourceStatusNew ("") means no committed source status yet. It reads as the
	// authored baseline: any present, translatable source is at least authored.
	SourceStatusNew SourceStatus = ""
	// SourceStatusAuthored — source content exists (the presence baseline).
	SourceStatusAuthored SourceStatus = "authored"
	// SourceStatusChecked — source cleared its brand/terminology checks.
	SourceStatusChecked SourceStatus = "checked"
	// SourceStatusApproved — source signed off by a human/agent.
	SourceStatusApproved SourceStatus = "approved"
)

// SourceStatusLadder is the authoring lifecycle order, lowest to highest.
// Membership and order define "at least this status" coverage (used by a source
// gate). New ("") is not listed — it means "no committed status yet" and reads
// as the authored baseline, the lowest rung.
func SourceStatusLadder() []SourceStatus {
	return []SourceStatus{
		SourceStatusAuthored,
		SourceStatusChecked,
		SourceStatusApproved,
	}
}

// Rank returns the 0-based position of s on the ladder, or -1 for New ("") or an
// unknown status. A higher rank is a more advanced authoring state.
func (s SourceStatus) Rank() int {
	for i, t := range SourceStatusLadder() {
		if t == s {
			return i
		}
	}
	return -1
}

// EffectiveRank is Rank with New ("") folded to the authored baseline: any
// present source is at least authored, so an uncommitted status ranks as
// authored rather than "below the ladder". This is the reading a source gate
// uses — an unstamped-but-present block is authored, not un-authored.
func (s SourceStatus) EffectiveRank() int {
	if s == SourceStatusNew {
		return SourceStatusAuthored.Rank()
	}
	return s.Rank()
}

// SourceGateLevel is the SourceStatus a block's source must reach before its
// translations may be produced — the level-based source-first convergence gate
// (strategy 2026-07-dogfood doc 07 / roadmap epic 019). It is the runtime
// counterpart of the recipe's `defaults.source_gate` string: the fan-out is
// held for any block whose source ranks below the gate.
type SourceGateLevel string

const (
	// SourceGateNone disables the gate — the deliberate "raw MT, no gate"
	// opt-out. Every present source translates regardless of its status,
	// exactly as convergence behaved before source-first.
	SourceGateNone SourceGateLevel = "none"
	// SourceGateAuthored gates on the presence baseline: any non-empty source
	// passes. (Effectively a no-op gate, provided for completeness/symmetry.)
	SourceGateAuthored SourceGateLevel = "authored"
	// SourceGateChecked is the DEFAULT: the source must clear its automated
	// terminology + brand + source-QA checks (no human bottleneck).
	SourceGateChecked SourceGateLevel = "checked"
	// SourceGateApproved requires a human/agent source sign-off — for
	// brand-critical or regulated projects.
	SourceGateApproved SourceGateLevel = "approved"
)

// DefaultSourceGate is the gate applied when a project does not set
// `defaults.source_gate`: settle-then-translate is on by default (owner
// decision, 2026-07-17).
const DefaultSourceGate = SourceGateChecked

// ResolveSourceGate maps a recipe's `defaults.source_gate` string onto a gate
// level, applying the default (`checked`) for an empty/unset value and treating
// an unrecognized value as the default too (a typo must not silently disable the
// gate). The second result reports whether the input named a recognized level.
func ResolveSourceGate(raw string) (SourceGateLevel, bool) {
	switch SourceGateLevel(raw) {
	case "":
		return DefaultSourceGate, true // unset → default
	case SourceGateNone:
		return SourceGateNone, true
	case SourceGateAuthored:
		return SourceGateAuthored, true
	case SourceGateChecked:
		return SourceGateChecked, true
	case SourceGateApproved:
		return SourceGateApproved, true
	default:
		return DefaultSourceGate, false // unknown → default, and flag it
	}
}

// RequiredRank is the ladder rank a source must reach to clear this gate, or -1
// when the gate is disabled (SourceGateNone) — nothing is held.
func (g SourceGateLevel) RequiredRank() int {
	switch g {
	case SourceGateNone:
		return -1
	case SourceGateAuthored:
		return SourceStatusAuthored.Rank()
	case SourceGateApproved:
		return SourceStatusApproved.Rank()
	default: // SourceGateChecked and any unresolved value gate at checked
		return SourceStatusChecked.Rank()
	}
}

// Admits reports whether a source at status s clears this gate — i.e. its
// source may be translated. A disabled gate (SourceGateNone) admits everything.
func (g SourceGateLevel) Admits(s SourceStatus) bool {
	req := g.RequiredRank()
	if req < 0 {
		return true // gate disabled
	}
	return s.EffectiveRank() >= req
}
