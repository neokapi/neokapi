package model

import "slices"

// A derived edition records what it was made from on the edition itself
// (Edition.Derived): the edition it was made from and that edition's revision
// at the time, its basis. Whether the derived edition still renders what it
// was made from is then read from the content: it is current while the edition
// it names still has the basis revision, and stale once that edition has
// moved. Nothing about it is stored beside the block.

// PropReadSourceLocale is the advisory block property a read sets when it
// files a block under a language other than the one the block's reader
// declared for it, as a project read files every block under the project's
// source language: the language the reader declared. A revision of the
// edition the block was read in depends on the key it is taken under, and a
// revision taken by a read that kept the reader's language is taken under this
// one.
const PropReadSourceLocale = AdvisoryPropertyPrefix + "read-source-locale"

// Standing says how a derived edition stands against the edition it was made
// from.
type Standing int

const (
	// StandingUnknown: nothing records what the edition was made from, so
	// nothing can call it stale.
	StandingUnknown Standing = iota
	// StandingCurrent: the edition it was made from still has the basis
	// revision.
	StandingCurrent
	// StandingStale: the edition it was made from has moved since, or is gone.
	StandingStale
)

// String names the standing: unknown, current or stale.
func (s Standing) String() string {
	switch s {
	case StandingCurrent:
		return "current"
	case StandingStale:
		return "stale"
	default:
		return "unknown"
	}
}

// Derivation returns what edition k records it was made from, and whether
// the block holds k with a derivation.
func (b *Block) Derivation(k EditionKey) (Derivation, bool) {
	e, ok := b.Edition(k)
	if !ok || e.Derived == nil {
		return Derivation{}, false
	}
	return *e.Derived, true
}

// SetDerivation records d as what edition k was made from, its basis, or
// clears the record for nil, and changes nothing else about the edition. It
// reports whether the block holds k; a block that does not is left unchanged.
// The block keeps a copy of d.
func (b *Block) SetDerivation(k EditionKey, d *Derivation) bool {
	var held *Derivation
	if d != nil {
		cp := *d
		cp.From = cp.From.Canonical()
		held = &cp
	}
	if b.holdsSource(k) {
		b.writeSource(func(e *Edition) { e.Derived = held })
		return true
	}
	t := b.Editions[k.Canonical()]
	if t == nil {
		return false
	}
	t.Derived = held
	return true
}

// BasisStanding grades derivation d against the block as it stands now:
// current while edition d.From still has revision d.Rev, stale once it has
// moved or is gone, and unknown for a derivation that records no revision. A
// reader holding a derivation from elsewhere (the record of the write that
// made an edition) grades it here, so every reader reads staleness one way.
func (b *Block) BasisStanding(d Derivation) Standing {
	if d.Rev == "" {
		return StandingUnknown
	}
	if EditionRevision(b, d.From) == d.Rev {
		return StandingCurrent
	}
	return StandingStale
}

// SourceRevisions returns the revisions of the edition the block was read in
// under every key a reader may have taken one under: the key the block files
// it by first (EditionRevision), then the zero key, the language source names
// (a project's source language, empty for none), and the language the block's
// reader declared where a read filed the block under another
// (PropReadSourceLocale). Readers of one document disagree on that key: a
// reader that declares no language leaves the block with none, a project read
// files every block under the project's language, and a reader that declares a
// language keeps its own. The content is the same under each, so a revision of
// the content as it stands now, taken by any of them, is among these, and a
// revision of other content is none of them.
func (b *Block) SourceRevisions(source LocaleID) []string {
	runs := b.sourceRuns()
	out := []string{EditionRevision(b, EditionKey{})}
	for _, k := range []EditionKey{{}, Variant(source), Variant(LocaleID(b.Properties[PropReadSourceLocale]))} {
		if rev := RunsRevision(k, runs); !slices.Contains(out, rev) {
			out = append(out, rev)
		}
	}
	return out
}

// DerivationStanding grades the derivation edition k records (BasisStanding),
// unknown when it records none.
func (b *Block) DerivationStanding(k EditionKey) Standing {
	d, ok := b.Derivation(k)
	if !ok {
		return StandingUnknown
	}
	return b.BasisStanding(d)
}
