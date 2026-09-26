package model

// SourceStatus is the lifecycle state of a Block's source content, the
// source-side counterpart of TargetStatus. A translation is draft, then
// translated, then established; a source is written, then established when a
// person has reviewed it. Whether a written source passes its checks is a
// separate fact the settle step reports beside the status (check.SettleSourceStatus),
// never a rung of its own.
type SourceStatus string

const (
	// SourceStatusNew ("") means no committed source status yet. It reads as the
	// written baseline: any present, translatable source is at least written.
	SourceStatusNew SourceStatus = ""
	// SourceStatusWritten: the source content exists.
	SourceStatusWritten SourceStatus = "written"
	// SourceStatusEstablished: a person reviewed the source and let it stand.
	SourceStatusEstablished SourceStatus = "established"
)

// SourceStatusLadder is the source lifecycle order, lowest to highest.
// Membership and order define "at least this status" coverage (used by a source
// gate). New ("") is not listed: it reads as the written baseline.
func SourceStatusLadder() []SourceStatus {
	return []SourceStatus{
		SourceStatusWritten,
		SourceStatusEstablished,
	}
}

// Rank returns the 0-based position of s on the ladder, or -1 for New ("") or an
// unknown status.
func (s SourceStatus) Rank() int {
	for i, t := range SourceStatusLadder() {
		if t == s {
			return i
		}
	}
	return -1
}

// EffectiveRank is Rank with New ("") folded to the written baseline: any
// present source is at least written.
func (s SourceStatus) EffectiveRank() int {
	if s == SourceStatusNew {
		return SourceStatusWritten.Rank()
	}
	return s.Rank()
}

// SourceGateLevel is what a block's source must satisfy before its
// translations may be produced: the runtime counterpart of the recipe's
// `defaults.source_gate` string. The fan-out is held for any block that does
// not satisfy it.
type SourceGateLevel string

const (
	// SourceGateNone disables the gate: every present source translates.
	SourceGateNone SourceGateLevel = "none"
	// SourceGateWritten is the default: a written source translates once it
	// passes its checks. A source with a failing finding is held.
	SourceGateWritten SourceGateLevel = "written"
	// SourceGateEstablished holds a source until a person has established it,
	// for regulated or voice-critical projects. A failing finding still holds it.
	SourceGateEstablished SourceGateLevel = "established"
)

// DefaultSourceGate is the gate applied when a project does not set
// `defaults.source_gate`.
const DefaultSourceGate = SourceGateWritten

// ResolveSourceGate maps a recipe's `defaults.source_gate` string onto a gate
// level, applying the default for an empty value and treating an unrecognized
// value as the default too, so a typo never disables the gate. The second
// result reports whether the input named a recognized level.
func ResolveSourceGate(raw string) (SourceGateLevel, bool) {
	switch SourceGateLevel(raw) {
	case "":
		return DefaultSourceGate, true
	case SourceGateNone, SourceGateWritten, SourceGateEstablished:
		return SourceGateLevel(raw), true
	default:
		return DefaultSourceGate, false
	}
}

// Admits reports whether a source at status s, whose checks fail when failing
// is true, clears this gate, so its source may be translated. A disabled gate
// (SourceGateNone) admits everything; any other gate holds a source the settle
// step has not stamped yet (New), because nothing has checked it.
func (g SourceGateLevel) Admits(s SourceStatus, failing bool) bool {
	switch g {
	case SourceGateNone:
		return true
	case SourceGateEstablished:
		return !failing && s == SourceStatusEstablished
	default:
		return !failing && s.Rank() >= SourceStatusWritten.Rank()
	}
}

// PropSourceHeld is the block property the source-gate leading stage sets on a
// block whose source ranks below the active source gate: the marker a producer
// (recycle, translate) reads to skip translating an un-settled source. It is the
// in-stream, file-read counterpart of the server's per-item gateItemsBySource
// hold — the local converge re-reads source from files each pass, so the hold
// rides on the block rather than on a persisted store row. Value "1" means held;
// absent (or any other value) means producible.
const PropSourceHeld = "__source_held"

// SetSourceHeld marks (held=true) or clears (held=false) a block's source-gate
// hold via its Properties. It is idempotent and never allocates a map to clear a
// marker that was never set.
func (b *Block) SetSourceHeld(held bool) {
	if !held {
		if b.Properties != nil {
			delete(b.Properties, PropSourceHeld)
		}
		return
	}
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[PropSourceHeld] = "1"
}

// SourceHeld reports whether a block carries the source-gate hold marker — its
// source ranks below the active gate, so a producer must not translate it.
func (b *Block) SourceHeld() bool {
	return b.Properties[PropSourceHeld] == "1"
}

// PropSourceFailing is the block property the source settle step
// (check.SettleSourceStatus) sets on a block whose source fails its checks.
// A source gate other than `none` holds such a block. Value "1" means failing;
// absent means the source passes.
const PropSourceFailing = "__source_failing"

// SetSourceFailing marks (failing=true) or clears (failing=false) the failing
// marker. It never allocates a map to clear a marker that was never set.
func (b *Block) SetSourceFailing(failing bool) {
	if !failing {
		if b.Properties != nil {
			delete(b.Properties, PropSourceFailing)
		}
		return
	}
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[PropSourceFailing] = "1"
}

// SourceFailing reports whether the block carries the failing marker.
func (b *Block) SourceFailing() bool {
	return b.Properties[PropSourceFailing] == "1"
}

// AdmitsBlock reports whether a settled block clears this gate.
func (g SourceGateLevel) AdmitsBlock(b *Block) bool {
	return g.Admits(b.SourceStatus, b.SourceFailing())
}
