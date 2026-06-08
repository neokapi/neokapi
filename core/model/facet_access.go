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
