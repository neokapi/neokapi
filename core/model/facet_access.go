package model

// Block-scoped facet access (the former Block.Annotations / Layer.Annotations
// map). Block-scoped facets are stored in the single Overlays facet carrier as
// a facet whose Type is the lookup key (a non-positional type) carrying one
// span with a Value and a zero range. These accessors give the keyed,
// map-like ergonomics the former annotation map offered, over the one carrier.

// annoFacets is the shared implementation over a facet slice.
func annoGet(facets []Facet, key string) (any, bool) {
	for i := range facets {
		f := &facets[i]
		if string(f.Type) == key && !f.Type.IsPositional() && len(f.Spans) > 0 {
			return f.Spans[0].Value, true
		}
	}
	return nil, false
}

func annoSet(facets []Facet, key string, v any) []Facet {
	for i := range facets {
		if string(facets[i].Type) == key && !facets[i].Type.IsPositional() {
			facets[i].Spans = []Span{{Value: v}}
			return facets
		}
	}
	return append(facets, Facet{Type: FacetType(key), Spans: []Span{{Value: v}}})
}

func annoDel(facets []Facet, key string) []Facet {
	out := facets[:0]
	for _, f := range facets {
		if string(f.Type) == key && !f.Type.IsPositional() {
			continue
		}
		out = append(out, f)
	}
	return out
}

func annoMap(facets []Facet) map[string]any {
	var m map[string]any
	for i := range facets {
		f := &facets[i]
		if f.Type.IsPositional() || len(f.Spans) == 0 {
			continue
		}
		if m == nil {
			m = make(map[string]any)
		}
		m[string(f.Type)] = f.Spans[0].Value
	}
	return m
}

// Anno returns the block-scoped facet value stored under key, or (nil, false).
// It is the facet-carrier replacement for the former b.Annotations[key].
func (b *Block) Anno(key string) (any, bool) { return annoGet(b.Overlays, key) }

// SetAnno stores v as the block-scoped facet under key (upserting), replacing
// the former b.Annotations[key] = v.
func (b *Block) SetAnno(key string, v any) { b.Overlays = annoSet(b.Overlays, key, v) }

// DelAnno removes the block-scoped facet stored under key (former
// delete(b.Annotations, key)).
func (b *Block) DelAnno(key string) { b.Overlays = annoDel(b.Overlays, key) }

// AnnoMap returns a snapshot map of all block-scoped facets keyed by type,
// for ranging and length checks (former range/len over b.Annotations). Writes
// to the returned map do not affect the block; use SetAnno/DelAnno.
func (b *Block) AnnoMap() map[string]any { return annoMap(b.Overlays) }

// Positional facet span access. Run-anchored facets — term, entity,
// term-candidate, … — store one span per occurrence: the span's Range is the
// position and its ID the stable identity. These helpers manage those spans on
// the single facet carrier, replacing the former Position-bearing block-scoped
// annotations keyed "entity:N".

// FacetOf returns a pointer to the source-side facet of type t (Variant nil),
// or nil if the block has none.
func (b *Block) FacetOf(t FacetType) *Facet {
	for i := range b.Overlays {
		if b.Overlays[i].Type == t && b.Overlays[i].Variant == nil {
			return &b.Overlays[i]
		}
	}
	return nil
}

// AddFacetSpan appends span s to the source-side facet of type t, creating the
// facet if absent. Use for positional facets where s.Range is the position and
// s.ID the identity.
func (b *Block) AddFacetSpan(t FacetType, s Span) {
	if f := b.FacetOf(t); f != nil {
		f.Spans = append(f.Spans, s)
		return
	}
	b.Overlays = append(b.Overlays, Facet{Type: t, Spans: []Span{s}})
}

// FacetSpan returns a pointer to the span with the given ID in the source-side
// facet of type t, or nil. The pointer is into the block's storage, so callers
// may mutate the span in place.
func (b *Block) FacetSpan(t FacetType, id string) *Span {
	f := b.FacetOf(t)
	if f == nil {
		return nil
	}
	for i := range f.Spans {
		if f.Spans[i].ID == id {
			return &f.Spans[i]
		}
	}
	return nil
}

// RemoveFacet drops the source-side facet of type t entirely (all its spans).
// Used when an operation invalidates a facet's run-anchored ranges — e.g. a
// source-transform tool that consumes the entity facet and then rewrites the
// source must drop it, since the spans no longer anchor to the new runs.
func (b *Block) RemoveFacet(t FacetType) {
	out := b.Overlays[:0]
	for _, f := range b.Overlays {
		if f.Type == t && f.Variant == nil {
			continue
		}
		out = append(out, f)
	}
	b.Overlays = out
}

// RemoveFacetSpan removes the span with the given ID from the source-side facet
// of type t, reporting whether it was found.
func (b *Block) RemoveFacetSpan(t FacetType, id string) bool {
	f := b.FacetOf(t)
	if f == nil {
		return false
	}
	for i := range f.Spans {
		if f.Spans[i].ID == id {
			f.Spans = append(f.Spans[:i], f.Spans[i+1:]...)
			return true
		}
	}
	return false
}

// AnnoAs returns the block-scoped facet payload under key asserted to type T,
// reporting ok only when the facet exists and has that concrete type. It is the
// generic replacement for `v, ok := b.Annotations[key].(T)`.
func AnnoAs[T any](b *Block, key string) (T, bool) {
	var zero T
	v, ok := b.Anno(key)
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// Anno returns the layer-scoped facet value stored under key, or (nil, false).
func (l *Layer) Anno(key string) (any, bool) { return annoGet(l.Overlays, key) }

// SetAnno stores v as the layer-scoped facet under key (upserting).
func (l *Layer) SetAnno(key string, v any) { l.Overlays = annoSet(l.Overlays, key, v) }

// DelAnno removes the layer-scoped facet stored under key.
func (l *Layer) DelAnno(key string) { l.Overlays = annoDel(l.Overlays, key) }

// AnnoMap returns a snapshot map of all layer-scoped facets keyed by type.
func (l *Layer) AnnoMap() map[string]any { return annoMap(l.Overlays) }
