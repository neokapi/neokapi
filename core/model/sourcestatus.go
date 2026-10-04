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

// TranslateAfterLevel is the derivation gate: what an edition must reach
// before an edition is derived from it, so a block's translations are produced
// only once its source has reached the level. It is the runtime counterpart of
// the recipe's `defaults.translate_after` string, and the fan-out is held for
// any block whose source has not reached it (AdmitsDerivation).
type TranslateAfterLevel string

const (
	// TranslateAfterNone holds nothing: every present source translates.
	TranslateAfterNone TranslateAfterLevel = "none"
	// TranslateAfterWritten is the default: a written source translates once it
	// passes its checks. A source with a failing finding is held.
	TranslateAfterWritten TranslateAfterLevel = "written"
	// TranslateAfterEstablished holds a source until a person has established it,
	// for regulated or voice-critical projects. A failing finding still holds it.
	TranslateAfterEstablished TranslateAfterLevel = "established"
)

// DefaultTranslateAfter is the level applied when a project does not set
// `defaults.translate_after`.
const DefaultTranslateAfter = TranslateAfterWritten

// ResolveTranslateAfter maps a recipe's `defaults.translate_after` string onto
// a level, applying the default for an empty value and treating an
// unrecognized value as the default too, so a typo never disables the hold.
// The second result reports whether the input named a recognized level.
func ResolveTranslateAfter(raw string) (TranslateAfterLevel, bool) {
	switch TranslateAfterLevel(raw) {
	case "":
		return DefaultTranslateAfter, true
	case TranslateAfterNone, TranslateAfterWritten, TranslateAfterEstablished:
		return TranslateAfterLevel(raw), true
	default:
		return DefaultTranslateAfter, false
	}
}

// Admits reports whether a source at status s, whose checks fail when failing
// is true, has reached this level, so its source may be translated. Level
// `none` (TranslateAfterNone) admits everything; any other level holds a source
// the settle step has not stamped yet (New), because nothing has checked it.
func (g TranslateAfterLevel) Admits(s SourceStatus, failing bool) bool {
	switch g {
	case TranslateAfterNone:
		return true
	case TranslateAfterEstablished:
		return !failing && s == SourceStatusEstablished
	default:
		return !failing && s.Rank() >= SourceStatusWritten.Rank()
	}
}

// PropSourceHeld is the block property the translate-after leading stage sets
// on a block whose source is below the active level: the marker a producer
// (recycle, translate) reads to skip translating an un-settled source. It is the
// in-stream, file-read counterpart of the server's per-item gateItemsBySource
// hold. The local converge re-reads source from files each pass, so the hold
// rides on the block rather than on a persisted store row. Value "1" means held;
// absent (or any other value) means producible.
const PropSourceHeld = "__source_held"

// SetSourceHeld marks (held=true) or clears (held=false) a block's
// translate-after hold via its Properties. It is idempotent and never allocates
// a map to clear a marker that was never set.
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

// SourceHeld reports whether a block carries the translate-after hold marker:
// its source is below the active level, so a producer must not translate it.
func (b *Block) SourceHeld() bool {
	return b.Properties[PropSourceHeld] == "1"
}

// PropSourceFailing is the block property the source settle step
// (check.SettleSourceStatus) sets on a block whose source fails its checks.
// A translate_after level other than `none` holds such a block. Value "1" means failing;
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

// AdmitsDerivation reports whether an edition may be derived from edition
// from of b at this level. The edition it would be made from climbs the ladder
// of its role: the edition the block was read in goes from written to
// established, and its failing checks (SourceFailing) hold it at any level but
// none; any other edition is written once translated and established once a
// person established it. Level none admits every derivation.
func (g TranslateAfterLevel) AdmitsDerivation(b *Block, from EditionKey) bool {
	if g == TranslateAfterNone {
		return true
	}
	e, ok := b.Edition(from)
	if !ok {
		return false
	}
	if b.holdsSource(from) {
		return g.Admits(SourceStatus(e.Status), b.SourceFailing())
	}
	s := TargetStatus(e.Status)
	if g == TranslateAfterEstablished {
		return s == TargetStatusEstablished
	}
	return s.Rank() >= TargetStatusTranslated.Rank()
}

// AdmitsBlock reports whether a settled block's translations may be produced:
// the derivation gate on the edition the block was read in (AdmitsDerivation).
func (g TranslateAfterLevel) AdmitsBlock(b *Block) bool {
	return g.AdmitsDerivation(b, EditionKey{})
}
