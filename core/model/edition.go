package model

import (
	"fmt"
	"slices"
	"strings"
)

// A block holds its content as editions: the edition the document itself is
// written in, and one more for every translation or other variant a reader or
// a tool attached. The accessors in this file address every edition the same
// way, by key, whichever field holds it. Source holds the edition the block was
// read in; Targets holds the others.
//
// New code reaches content through these accessors rather than through Source
// and Targets, so that how the editions are stored can change without touching
// it.

// EditionKey names one edition of a block: its language, and optionally a tone
// and a channel. It is the type the Targets map is keyed by.
type EditionKey = VariantKey

// Status is an edition's lifecycle state. The edition's role picks the ladder
// it climbs: the authoritative edition goes from written to established (the
// values of SourceStatus), a derived edition from draft through translated to
// established (the values of TargetStatus). Empty means no status recorded yet.
type Status string

// Edition is the content of one edition with its lifecycle and provenance.
type Edition struct {
	// Runs is the content. The slice is the block's own: treat it as read-only.
	Runs []Run
	// Status is where the edition stands on its ladder.
	Status Status
	// Origin records how the content was produced.
	Origin Origin
	// Score is the producer's quality score for a derived edition. The edition
	// a document is written in carries none.
	Score float64
}

// AuthorityPolicy says which edition of a block is authoritative: the one
// every other edition is derived from.
type AuthorityPolicy struct {
	// Locale is the language a recipe names as the source of the block's
	// collection. Empty means the edition the block was read in.
	Locale LocaleID
}

// ParseEditionKey reads an edition key in its text form, "fr",
// "fr;tone=formal" or "en;channel=short", and returns it canonical: the
// language normalized to its BCP-47 form. The empty string is the zero key,
// which names the document's own edition.
//
// It is strict where VariantKey.UnmarshalText is lenient: a language that is
// not a locale, a dimension other than tone and channel, a dimension named
// twice, and a dimension with no value are errors.
func ParseEditionKey(s string) (EditionKey, error) {
	if s == "" {
		return EditionKey{}, nil
	}
	parts := strings.Split(s, ";")
	if parts[0] == "" {
		return EditionKey{}, fmt.Errorf("edition %q names no language", s)
	}
	locale, err := CanonicalLocale(parts[0])
	if err != nil {
		return EditionKey{}, fmt.Errorf("edition %q: %w", s, err)
	}
	k := EditionKey{Locale: locale}
	for _, p := range parts[1:] {
		name, val, ok := strings.Cut(p, "=")
		if !ok || val == "" || strings.Contains(val, "=") {
			return EditionKey{}, fmt.Errorf("edition %q: %q is not name=value", s, p)
		}
		switch name {
		case "tone":
			if k.Tone != "" {
				return EditionKey{}, fmt.Errorf("edition %q names its tone twice", s)
			}
			k.Tone = val
		case "channel":
			if k.Channel != "" {
				return EditionKey{}, fmt.Errorf("edition %q names its channel twice", s)
			}
			k.Channel = val
		default:
			return EditionKey{}, fmt.Errorf("edition %q: unknown dimension %q; an edition has a language, a tone and a channel", s, name)
		}
	}
	return k, nil
}

// keyText is the text form of k (MarshalText), for ordering keys.
func keyText(k EditionKey) string {
	b, _ := k.MarshalText()
	return string(b)
}

// sourceLanguageKey is the key that names the block's source language, with
// no tone and no channel.
func (b *Block) sourceLanguageKey() EditionKey {
	return EditionKey{Locale: NormalizeLocale(b.SourceLocale)}
}

// holdsSameLanguageTarget reports whether the block holds a target filed under
// its source language with no tone and no channel: the target of a bilingual
// file whose two languages are one, such as an XLIFF file from en-US to en-US
// or a PO catalogue in its source language.
func (b *Block) holdsSameLanguageTarget() bool {
	return b.SourceLocale != "" && b.Targets[b.sourceLanguageKey()] != nil
}

// sourceKey is the key of the edition Source holds: its language, or the zero
// key when a same-language target holds that key.
func (b *Block) sourceKey() EditionKey {
	if b.holdsSameLanguageTarget() {
		return EditionKey{}
	}
	return b.sourceLanguageKey()
}

// holdsSource reports whether k names the edition Source holds: the zero key,
// which is always the document's own edition, or the block's source language
// with no tone and no channel, unless a same-language target holds that key.
// A same-language edition with a tone or a channel is an edition of its own.
func (b *Block) holdsSource(k EditionKey) bool {
	if k.IsZero() {
		return true
	}
	return k.Tone == "" && k.Channel == "" && b.SourceLocale != "" &&
		NormalizeLocale(k.Locale) == NormalizeLocale(b.SourceLocale) && !b.holdsSameLanguageTarget()
}

// IsSourceEdition reports whether k reaches the edition Source holds. Overlays
// on that edition carry no Variant; overlays on every other edition carry its
// key.
func (b *Block) IsSourceEdition(k EditionKey) bool { return b.holdsSource(k) }

// EditionKeyOf returns the canonical key of the edition k reaches on b: the
// key of the edition Source holds when k names it, k canonical otherwise. Two
// keys reach the same edition exactly when their EditionKeyOf values are
// equal.
func (b *Block) EditionKeyOf(k EditionKey) EditionKey {
	if b.holdsSource(k) {
		return b.sourceKey()
	}
	return k.Canonical()
}

// Edition returns edition k of the block and whether the block holds it. The
// edition the block was read in is always held, empty or not.
func (b *Block) Edition(k EditionKey) (Edition, bool) {
	if b.holdsSource(k) {
		e := Edition{Runs: b.Source, Status: Status(b.SourceStatus)}
		if o, ok := b.SourceOrigin(); ok && o != nil {
			e.Origin = *o
		}
		return e, true
	}
	t := b.Targets[k.Canonical()]
	if t == nil {
		return Edition{}, false
	}
	return Edition{Runs: t.Runs, Status: Status(t.Status), Origin: t.Origin, Score: t.Score}, true
}

// SetEdition stores e as edition k, creating the edition when the block does
// not hold it. Writing new runs to the edition the block was read in is an
// edit (EditSourceRuns), so the block keeps the content its reader produced;
// handing back the runs the edition holds (Edition, then SetEdition with a new
// status) writes its status and origin and edits nothing. Its origin is the
// block's source-origin annotation, removed when e.Origin is zero, and e.Score
// is not kept. An existing derived edition is updated in place, so a *Target a
// caller holds sees the change.
func (b *Block) SetEdition(k EditionKey, e Edition) {
	if b.holdsSource(k) {
		if !sameRuns(b.Source, e.Runs) {
			b.EditSourceRuns(e.Runs)
		}
		b.SourceStatus = SourceStatus(e.Status)
		if e.Origin == (Origin{}) {
			b.DelAnno(AnnoSourceOrigin)
		} else {
			o := e.Origin
			b.SetSourceOrigin(&o)
		}
		return
	}
	key := k.Canonical()
	if b.Targets == nil {
		b.Targets = make(map[VariantKey]*Target)
	}
	if t := b.Targets[key]; t != nil {
		t.Runs, t.Status, t.Origin, t.Score = e.Runs, TargetStatus(e.Status), e.Origin, e.Score
		return
	}
	b.Targets[key] = &Target{Runs: e.Runs, Status: TargetStatus(e.Status), Origin: e.Origin, Score: e.Score}
}

// SetEditionStatus sets the status of edition k and changes nothing else: the
// runs, the origin, the score and the source as read (SourceAsRead) stay as
// they are. It reports whether the block holds k, and a block that does not
// hold it is left unchanged. A status stamp on the edition the block was read
// in goes through here: SetEdition on that edition writes its runs and its
// origin as well.
func (b *Block) SetEditionStatus(k EditionKey, s Status) bool {
	if b.holdsSource(k) {
		b.SourceStatus = SourceStatus(s)
		return true
	}
	t := b.Targets[k.Canonical()]
	if t == nil {
		return false
	}
	t.Status = TargetStatus(s)
	return true
}

// sameRuns reports whether a and b are one slice: both nil, both empty, or the
// same elements of the same backing array.
func sameRuns(a, b []Run) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

// RemoveEdition removes edition k and reports whether the block held it. The
// edition the block was read in cannot be removed.
func (b *Block) RemoveEdition(k EditionKey) bool {
	if b.holdsSource(k) {
		return false
	}
	key := k.Canonical()
	if _, ok := b.Targets[key]; !ok {
		return false
	}
	delete(b.Targets, key)
	return true
}

// Editions returns the keys of every edition the block holds: the edition it
// was read in first, then the others in the order of their text form. When a
// same-language target holds the key of the source language, the edition the
// block was read in is listed by the zero key.
func (b *Block) Editions() []EditionKey {
	src := b.sourceKey()
	out := make([]EditionKey, 0, 1+len(b.Targets))
	out = append(out, src)
	rest := make([]EditionKey, 0, len(b.Targets))
	for k, t := range b.Targets {
		if t == nil || b.holdsSource(k) {
			continue
		}
		rest = append(rest, k.Canonical())
	}
	slices.SortFunc(rest, func(a, c EditionKey) int { return strings.Compare(keyText(a), keyText(c)) })
	return append(out, slices.Compact(rest)...)
}

// EachEdition yields every edition the block holds with its key, for use as a
// range function: the edition it was read in first, under the key EditionKeyOf
// returns for the zero key, then the others in no particular order, each under
// its canonical key. No other edition is yielded under the first one's key.
// It reads the storage once, sorts nothing and allocates nothing, so a loop
// over the editions of many blocks ranges over it rather than over Editions.
//
// A target filed under a key that is not canonical is yielded under its
// canonical form unless a target is filed under that form too, in which case
// the canonical one is the edition and the other is skipped.
func (b *Block) EachEdition(yield func(EditionKey, Edition) bool) {
	src := b.sourceKey()
	e := Edition{Runs: b.Source, Status: Status(b.SourceStatus)}
	if o, ok := b.SourceOrigin(); ok && o != nil {
		e.Origin = *o
	}
	if !yield(src, e) {
		return
	}
	for k, t := range b.Targets {
		if t == nil {
			continue
		}
		ck := k.Canonical()
		// The zero key always names the edition yielded first, and so does the
		// source language while no same-language target holds it.
		if ck.IsZero() || ck == src {
			continue
		}
		if ck != k && b.Targets[ck] != nil {
			continue
		}
		if !yield(ck, Edition{Runs: t.Runs, Status: Status(t.Status), Origin: t.Origin, Score: t.Score}) {
			return
		}
	}
}

// Authoritative returns the key of the block's authoritative edition under p:
// the edition p names when the block holds it as an edition of its own, and
// otherwise the edition the block was read in. A policy that names the source
// language names the edition the block was read in, whatever else holds that
// language.
func (b *Block) Authoritative(p AuthorityPolicy) EditionKey {
	if p.Locale != "" && NormalizeLocale(p.Locale) != NormalizeLocale(b.SourceLocale) {
		if k := Variant(p.Locale); b.Targets[k] != nil {
			return k
		}
	}
	return b.sourceKey()
}
