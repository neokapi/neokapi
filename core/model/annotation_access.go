package model

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
	AnnoTMMatch        = "tm-match"
	AnnoWordCount      = "word-count"
	AnnoCharCount      = "char-count"
	AnnoSegCount       = "seg-count"
	AnnoComparison     = "comparison"
	AnnoScopingReport  = "scoping-report"
	AnnoRepetition     = "repetition"
	AnnoBrandVoice     = "brand-voice"
	AnnoEntityMapping  = "entity-mapping"
	AnnoTermEnforce    = "term-enforcement"
)

// --- Block-scoped annotation access ---

// Anno returns the block annotation stored under key, or (nil, false).
func (b *Block) Anno(key string) (any, bool) {
	v, ok := b.Annotations[key]
	return v, ok
}

// SetAnno stores v as the block annotation under key (upserting).
func (b *Block) SetAnno(key string, v any) {
	if b.Annotations == nil {
		b.Annotations = make(map[string]any)
	}
	b.Annotations[key] = v
}

// DelAnno removes the block annotation stored under key.
func (b *Block) DelAnno(key string) { delete(b.Annotations, key) }

// AnnoMap returns the block's annotation map for ranging and length checks. The
// returned map is the block's own storage; use SetAnno/DelAnno to mutate.
func (b *Block) AnnoMap() map[string]any { return b.Annotations }

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

// AppendAltUnder appends an alt-translation to the AltTranslations collection
// stored under an arbitrary key (e.g. the per-segment TM-match collection),
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
	v, ok := b.Annotations[key]
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// --- Layer-scoped annotation access ---

// Anno returns the layer annotation stored under key, or (nil, false).
func (l *Layer) Anno(key string) (any, bool) {
	v, ok := l.Annotations[key]
	return v, ok
}

// SetAnno stores v as the layer annotation under key (upserting).
func (l *Layer) SetAnno(key string, v any) {
	if l.Annotations == nil {
		l.Annotations = make(map[string]any)
	}
	l.Annotations[key] = v
}

// DelAnno removes the layer annotation stored under key.
func (l *Layer) DelAnno(key string) { delete(l.Annotations, key) }

// AnnoMap returns the layer's annotation map.
func (l *Layer) AnnoMap() map[string]any { return l.Annotations }
