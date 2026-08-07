package model

import (
	"iter"
	"maps"
)

// Annotations are block-scoped typed metadata: a keyed payload attached to the
// whole Block (or Layer) with no run position — the counterpart to positional
// Overlays (see overlay.go). They are the former "annotation map": notes,
// alt-translations, analysis results (word/char/segment counts, comparison,
// repetition, brand-voice), and format round-trip state. Each value is a typed
// payload registered with the payload registry (annotation_registry.go) so the
// wire and store layers can rehydrate it from its type name. The vocabulary is
// open — formats and plugins store their own keys.
const (
	AnnoNote           = "note"
	AnnoAltTranslation = "alt-translation"
	AnnoMemoryMatch    = "tm-match"
	AnnoWordCount      = "word-count"
	AnnoCharCount      = "char-count"
	AnnoSegCount       = "seg-count"
	AnnoComparison     = "comparison"
	AnnoScopingReport  = "scoping-report"
	AnnoRepetition     = "repetition"
	AnnoVoice          = "voice"
	AnnoEntityMapping  = "entity-mapping"
	AnnoTermEnforce    = "term-enforcement"
)

// --- Block-scoped annotation access ---

// Anno returns the block annotation stored under key, or (nil, false).
//
// A promoted key reads its field first and falls through to the map when the
// field is empty, so a payload of an unexpected type stored under that key —
// which SetAnno routes to the map — is still found.
func (b *Block) Anno(key string) (Payload, bool) {
	switch {
	case key == AnnoStructure && b.structure != nil:
		return b.structure, true
	case key == AnnoGeometry && b.geometry != nil:
		return b.geometry, true
	}
	v, ok := b.Annotations[key]
	return v, ok
}

// SetAnno stores v as the block annotation under key (upserting).
//
// A payload under one of the promoted keys goes to its field; anything else,
// including a value of the wrong type under a promoted key, goes to the map, so
// a caller storing something unexpected still gets it back.
func (b *Block) SetAnno(key string, v Payload) {
	switch key {
	case AnnoStructure:
		if s, ok := v.(*StructureAnnotation); ok {
			b.structure = s
			return
		}
	case AnnoGeometry:
		if g, ok := v.(*GeometryAnnotation); ok {
			b.geometry = g
			return
		}
	}
	if b.Annotations == nil {
		b.Annotations = make(map[string]Payload)
	}
	b.Annotations[key] = v
}

// DelAnno removes the block annotation stored under key.
func (b *Block) DelAnno(key string) {
	switch key {
	case AnnoStructure:
		b.structure = nil
	case AnnoGeometry:
		b.geometry = nil
	}
	delete(b.Annotations, key)
}

// AnnoMap returns the block's annotations as one map, for ranging and length
// checks. It is a snapshot, not the block's storage: writing to it does not
// write to the block. Use SetAnno/DelAnno to mutate, and prefer Annos for
// read-only iteration, which allocates nothing.
func (b *Block) AnnoMap() map[string]Payload {
	if b.structure == nil && b.geometry == nil {
		return b.Annotations
	}
	out := make(map[string]Payload, len(b.Annotations)+2)
	maps.Copy(out, b.Annotations)
	if b.structure != nil {
		out[AnnoStructure] = b.structure
	}
	if b.geometry != nil {
		out[AnnoGeometry] = b.geometry
	}
	return out
}

// Annos yields the block's annotations for read-only iteration, without
// handing out or building a map.
func (b *Block) Annos() iter.Seq2[string, Payload] {
	return func(yield func(string, Payload) bool) {
		if b.structure != nil && !yield(AnnoStructure, b.structure) {
			return
		}
		if b.geometry != nil && !yield(AnnoGeometry, b.geometry) {
			return
		}
		for k, v := range b.Annotations {
			if !yield(k, v) {
				return
			}
		}
	}
}

// AltTranslations returns the block's alternative-translation candidates (the
// []*AltTranslation under AnnoAltTranslation), or nil.
func (b *Block) AltTranslations() []*AltTranslation {
	if v, ok := AnnoAs[*AltTranslations](b, AnnoAltTranslation); ok {
		return v.Items
	}
	return nil
}

// AddAltTranslation appends an alt-translation candidate to the block's
// AnnoAltTranslation collection (creating it if absent). Multiplicity lives in
// the collection, never in numbered keys.
func (b *Block) AddAltTranslation(a *AltTranslation) { b.appendAlt(AnnoAltTranslation, a) }

// Notes returns the block's notes (the []*NoteAnnotation under AnnoNote), or nil.
func (b *Block) Notes() []*NoteAnnotation {
	if v, ok := AnnoAs[*Notes](b, AnnoNote); ok {
		return v.Items
	}
	return nil
}

// AddNote appends a note to the block's AnnoNote collection (creating it if
// absent). Multiplicity lives in the collection, never in numbered keys.
func (b *Block) AddNote(n *NoteAnnotation) {
	v, ok := AnnoAs[*Notes](b, AnnoNote)
	if !ok || v == nil {
		v = &Notes{}
	}
	v.Items = append(v.Items, n)
	b.SetAnno(AnnoNote, v)
}

// AppendAltUnder appends an alt-translation to the AltTranslations collection
// stored under an arbitrary key (e.g. the per-segment content memory-match collection),
// creating it if absent. The set is one typed payload; the segment is recorded
// on AltTranslation.SegmentIndex rather than in the key.
func (b *Block) AppendAltUnder(key string, a *AltTranslation) { b.appendAlt(key, a) }

func (b *Block) appendAlt(key string, a *AltTranslation) {
	v, ok := AnnoAs[*AltTranslations](b, key)
	if !ok || v == nil {
		v = &AltTranslations{}
	}
	v.Items = append(v.Items, a)
	b.SetAnno(key, v)
}

// AnnoAs returns the block annotation under key asserted to type T, reporting
// ok only when the annotation exists and has that concrete type.
func AnnoAs[T any](b *Block, key string) (T, bool) {
	var zero T
	v, ok := b.Anno(key)
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// --- Layer-scoped annotation access ---

// Anno returns the layer annotation stored under key, or (nil, false).
func (l *Layer) Anno(key string) (Payload, bool) {
	v, ok := l.Annotations[key]
	return v, ok
}

// SetAnno stores v as the layer annotation under key (upserting).
func (l *Layer) SetAnno(key string, v Payload) {
	if l.Annotations == nil {
		l.Annotations = make(map[string]Payload)
	}
	l.Annotations[key] = v
}

// DelAnno removes the layer annotation stored under key.
func (l *Layer) DelAnno(key string) { delete(l.Annotations, key) }

// AnnoMap returns the layer's annotation map. As with Block.AnnoMap, the
// returned map is the layer's own LIVE storage; use SetAnno/DelAnno to mutate
// and prefer Annos for read-only iteration.
func (l *Layer) AnnoMap() map[string]Payload { return l.Annotations }

// Annos yields the layer's annotations for read-only iteration, without
// handing out the underlying map.
func (l *Layer) Annos() iter.Seq2[string, Payload] {
	return maps.All(l.Annotations)
}
