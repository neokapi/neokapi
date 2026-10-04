package model

import (
	"fmt"
	"slices"
	"strings"
)

// A block holds its content as peer editions in Block.Editions: the edition
// the document itself is written in, under the zero key, and one more for
// every translation or other edition a reader or a tool attached, each under
// its canonical key. The accessors in this file address every edition the same
// way, by key, so code that reads or writes content never depends on how the
// editions are stored.
//
// The zero key always reaches the edition the block was read in, and so does
// a key in the block's source language with no tone and no channel, unless the
// block holds a same-language edition of its own under that key, as a
// bilingual file from en-US to en-US does.

// Status is an edition's lifecycle state. The edition's role picks the ladder
// it climbs: the authoritative edition goes from written to established (the
// values of SourceStatus), a derived edition from draft through translated to
// established (the values of TargetStatus). Empty means no status recorded yet.
type Status string

// Edition is the content of one edition with its lifecycle, provenance and
// derivation. Its JSON form spells each field as Go names it and leaves
// Derived out while it is nil.
type Edition struct {
	// Runs is the content. The slice is the block's own: treat it as read-only.
	Runs []Run
	// Status is where the edition stands on its ladder.
	Status Status
	// Origin records how the content was produced. The edition a block was
	// read in keeps its origin in the block's source-origin annotation
	// (SourceOrigin), which every wire and store carries: Edition and
	// EachEdition fill it from there, and SetEdition writes it there.
	Origin Origin
	// Score is the producer's quality score for a derived edition. The edition
	// a document is written in carries none.
	Score float64
	// Derived names the edition this one was made from and that edition's
	// revision when it was made: the basis staleness is read against. Nil for
	// an authored edition, and for an edition nothing recorded a basis for.
	Derived *Derivation `json:",omitempty"`
}

// Derivation records where a derived edition came from: the edition it was
// made from and that edition's revision (EditionRevision) at the time.
type Derivation struct {
	From EditionKey
	Rev  string
}

// AuthorityPolicy says which edition of a block is authoritative: the one
// every other edition is derived from.
type AuthorityPolicy struct {
	// Locale is the language a recipe names as the source of the block's
	// collection. Empty means the engine's rule: the first native edition, the
	// edition the block was read in.
	Locale LocaleID
}

// ParseEditionKey reads an edition key in its text form, "fr",
// "fr;tone=formal" or "en;channel=short", and returns it canonical: the
// language normalized to its BCP-47 form. The empty string is the zero key,
// which names the document's own edition.
//
// It is strict where EditionKey.UnmarshalText is lenient: a language that is
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

// source returns the entry of the edition the block was read in, or nil while
// the block holds nothing there.
func (b *Block) source() *Edition { return b.Editions[EditionKey{}] }

// sourceRuns returns the runs of the edition the block was read in.
func (b *Block) sourceRuns() []Run {
	if s := b.source(); s != nil {
		return s.Runs
	}
	return nil
}

// sourceStatus returns the status of the edition the block was read in.
func (b *Block) sourceStatus() Status {
	if s := b.source(); s != nil {
		return s.Status
	}
	return ""
}

// writeSource applies f to the entry of the edition the block was read in,
// creating it when the block holds none. An entry f leaves with no runs, no
// status and no derivation is removed again, so a block whose source was never
// written and one whose source was written empty hold the same storage.
func (b *Block) writeSource(f func(*Edition)) {
	s := b.source()
	if s == nil {
		var e Edition
		f(&e)
		if e.Runs == nil && e.Status == "" && e.Derived == nil {
			return
		}
		if b.Editions == nil {
			b.Editions = make(map[EditionKey]*Edition)
		}
		b.Editions[EditionKey{}] = &e
		return
	}
	f(s)
	if s.Runs == nil && s.Status == "" && s.Derived == nil {
		delete(b.Editions, EditionKey{})
	}
}

// target returns the entry the target accessors read under key, which is
// canonical: the translation filed under no language for the zero key, the
// edition filed under key for any other.
func (b *Block) target(key EditionKey) *Edition {
	if key.IsZero() {
		return b.unlabelled
	}
	return b.Editions[key]
}

// putTarget files e where target reads key.
func (b *Block) putTarget(key EditionKey, e *Edition) {
	if key.IsZero() {
		b.unlabelled = e
		return
	}
	if b.Editions == nil {
		b.Editions = make(map[EditionKey]*Edition)
	}
	b.Editions[key] = e
}

// sourceLanguageKey is the key that names the block's source language, with
// no tone and no channel.
func (b *Block) sourceLanguageKey() EditionKey {
	return EditionKey{Locale: NormalizeLocale(b.SourceLocale)}
}

// holdsSameLanguageTarget reports whether the block holds an edition filed
// under its source language with no tone and no channel: the target of a
// bilingual file whose two languages are one, such as an XLIFF file from en-US
// to en-US or a PO catalogue in its source language.
func (b *Block) holdsSameLanguageTarget() bool {
	return b.SourceLocale != "" && b.Editions[b.sourceLanguageKey()] != nil
}

// sourceKey is the key the edition the block was read in is known by: its
// language, or the zero key when a same-language edition holds that key or
// the block names no source language.
func (b *Block) sourceKey() EditionKey {
	if b.holdsSameLanguageTarget() {
		return EditionKey{}
	}
	return b.sourceLanguageKey()
}

// holdsSource reports whether k names the edition the block was read in: the
// zero key, or the block's source language with no tone and no channel,
// unless a same-language edition holds that key. A same-language edition with
// a tone or a channel is an edition of its own.
func (b *Block) holdsSource(k EditionKey) bool {
	if k.IsZero() {
		return true
	}
	return k.Tone == "" && k.Channel == "" && b.SourceLocale != "" &&
		NormalizeLocale(k.Locale) == NormalizeLocale(b.SourceLocale) && !b.holdsSameLanguageTarget()
}

// IsSourceEdition reports whether k reaches the edition the block was read in.
// Overlays on that edition name the zero key; overlays on every other edition
// name its key.
func (b *Block) IsSourceEdition(k EditionKey) bool { return b.holdsSource(k) }

// EditionKeyOf returns the canonical key of the edition k reaches on b: the
// key the edition the block was read in is known by when k names it, k
// canonical otherwise. Two keys reach the same edition exactly when their
// EditionKeyOf values are equal.
func (b *Block) EditionKeyOf(k EditionKey) EditionKey {
	if b.holdsSource(k) {
		return b.sourceKey()
	}
	return k.Canonical()
}

// sourceEdition returns the edition the block was read in, with its origin
// from the source-origin annotation and no score.
func (b *Block) sourceEdition() Edition {
	var e Edition
	if s := b.source(); s != nil {
		e.Runs, e.Status, e.Derived = s.Runs, s.Status, s.Derived
	}
	if o, ok := b.SourceOrigin(); ok && o != nil {
		e.Origin = *o
	}
	return e
}

// Edition returns edition k of the block and whether the block holds it. The
// edition the block was read in is always held, empty or not.
func (b *Block) Edition(k EditionKey) (Edition, bool) {
	if b.holdsSource(k) {
		return b.sourceEdition(), true
	}
	t := b.Editions[k.Canonical()]
	if t == nil {
		return Edition{}, false
	}
	return *t, true
}

// SetEdition stores e as edition k, creating the edition when the block does
// not hold it. Writing the edition the block was read in is an edit
// (EditSourceRuns), so the block keeps the content its reader produced; its
// origin is the block's source-origin annotation, removed when e.Origin is
// zero, and e.Score is not kept. Any other edition is stored whole and in
// place, so a later read sees exactly e.
func (b *Block) SetEdition(k EditionKey, e Edition) {
	if b.holdsSource(k) {
		b.EditSourceRuns(e.Runs)
		b.writeSource(func(s *Edition) { s.Status, s.Derived = e.Status, e.Derived })
		if e.Origin == (Origin{}) {
			b.DelAnno(AnnoSourceOrigin)
		} else {
			o := e.Origin
			b.SetSourceOrigin(&o)
		}
		return
	}
	b.SetTargetEdition(k, e)
}

// SetEditionStatus sets the status of edition k and changes nothing else: the
// runs, the origin, the score and the source as read (SourceAsRead) stay as
// they are. It reports whether the block holds k, and a block that does not
// hold it is left unchanged. A status stamp on the edition the block was read
// in goes through here: SetEdition on that edition writes its runs and its
// origin as well.
func (b *Block) SetEditionStatus(k EditionKey, s Status) bool {
	if b.holdsSource(k) {
		b.writeSource(func(e *Edition) { e.Status = s })
		return true
	}
	t := b.Editions[k.Canonical()]
	if t == nil {
		return false
	}
	t.Status = s
	return true
}

// RemoveEdition removes edition k and reports whether the block held it. The
// edition the block was read in cannot be removed. A removed edition is no
// longer native either.
func (b *Block) RemoveEdition(k EditionKey) bool {
	if b.holdsSource(k) {
		return false
	}
	key := k.Canonical()
	if _, ok := b.Editions[key]; !ok {
		return false
	}
	delete(b.Editions, key)
	b.Native = slices.DeleteFunc(b.Native, func(n EditionKey) bool { return !n.IsZero() && n.Canonical() == key })
	return true
}

// EditionKeys returns the keys of every edition the block holds: the edition it
// was read in first, then the others in the order of their text form. When a
// same-language edition holds the key of the source language, the edition the
// block was read in is listed by the zero key.
func (b *Block) EditionKeys() []EditionKey {
	src := b.sourceKey()
	out := make([]EditionKey, 0, len(b.Editions))
	out = append(out, src)
	rest := make([]EditionKey, 0, len(b.Editions))
	for k, t := range b.Editions {
		if t == nil || k.IsZero() || b.holdsSource(k) {
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
// over the editions of many blocks ranges over it rather than over EditionKeys.
//
// An edition filed under a key that is not canonical is yielded under its
// canonical form unless an edition is filed under that form too, in which case
// the canonical one is the edition and the other is skipped.
func (b *Block) EachEdition(yield func(EditionKey, Edition) bool) {
	src := b.sourceKey()
	if !yield(src, b.sourceEdition()) {
		return
	}
	for k, t := range b.Editions {
		if t == nil || k.IsZero() {
			continue
		}
		ck := k.Canonical()
		// The source language names the edition yielded first while no
		// same-language edition holds it, however a stray key spelled it.
		if ck == src {
			continue
		}
		if ck != k && b.Editions[ck] != nil {
			continue
		}
		if !yield(ck, *t) {
			return
		}
	}
}

// Authoritative returns the key of the block's authoritative edition under p.
// A policy that names a language (a project's source language) names the
// edition the block holds as an edition of its own in that language. Otherwise,
// and for a policy that names the block's source language, it is the first
// native edition: the edition the block was read in.
func (b *Block) Authoritative(p AuthorityPolicy) EditionKey {
	if p.Locale != "" && NormalizeLocale(p.Locale) != NormalizeLocale(b.SourceLocale) {
		if k := Variant(p.Locale); b.Editions[k] != nil {
			return k
		}
	}
	return b.sourceKey()
}

// NativeEditions returns the keys of the editions the document's bytes hold,
// as EditionKeys lists them: the edition the block was read in first, then
// every other edition Native names that the block still holds, in the order
// Native names them.
func (b *Block) NativeEditions() []EditionKey {
	out := []EditionKey{b.sourceKey()}
	for _, k := range b.Native {
		if k.IsZero() || b.holdsSource(k) {
			continue
		}
		ck := k.Canonical()
		if b.Editions[ck] == nil || slices.Contains(out, ck) {
			continue
		}
		out = append(out, ck)
	}
	return out
}

// MarkNative records that the document's bytes hold edition k beside the
// edition the block was read in, which is always native. A reader of a format
// that writes more than one edition into a document marks each edition it
// files from the document.
func (b *Block) MarkNative(k EditionKey) {
	if b.holdsSource(k) {
		return
	}
	key := k.Canonical()
	if len(b.Native) == 0 {
		b.Native = []EditionKey{{}}
	}
	if !slices.Contains(b.Native, key) {
		b.Native = append(b.Native, key)
	}
}
