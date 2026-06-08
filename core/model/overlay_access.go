package model

// Positional overlay access. Run-anchored overlays — segmentation, term,
// entity, term-candidate, qa, alignment — store one span per occurrence: the
// span's Range is the position and its ID the stable identity. These helpers
// manage those spans on Block.Overlays. Block-scoped metadata uses the
// annotation map instead (see annotation_access.go).

// OverlayOf returns a pointer to the source-side overlay of type t (Variant
// nil), or nil if the block has none.
func (b *Block) OverlayOf(t OverlayType) *Overlay {
	for i := range b.Overlays {
		if b.Overlays[i].Type == t && b.Overlays[i].Variant == nil {
			return &b.Overlays[i]
		}
	}
	return nil
}

// AddOverlaySpan appends span s to the source-side overlay of type t, creating
// the overlay if absent. s.Range is the position and s.ID the identity.
func (b *Block) AddOverlaySpan(t OverlayType, s Span) {
	if o := b.OverlayOf(t); o != nil {
		o.Spans = append(o.Spans, s)
		return
	}
	b.Overlays = append(b.Overlays, Overlay{Type: t, Spans: []Span{s}})
}

// OverlaySpan returns a pointer to the span with the given ID in the
// source-side overlay of type t, or nil. The pointer is into the block's
// storage, so callers may mutate the span in place.
func (b *Block) OverlaySpan(t OverlayType, id string) *Span {
	o := b.OverlayOf(t)
	if o == nil {
		return nil
	}
	for i := range o.Spans {
		if o.Spans[i].ID == id {
			return &o.Spans[i]
		}
	}
	return nil
}

// RemoveOverlay drops the source-side overlay of type t entirely (all its
// spans). Used when an operation invalidates an overlay's run-anchored ranges —
// e.g. a source-transform tool that consumes the entity overlay and then
// rewrites the source must drop it, since the spans no longer anchor to the new
// runs.
func (b *Block) RemoveOverlay(t OverlayType) {
	out := b.Overlays[:0]
	for _, o := range b.Overlays {
		if o.Type == t && o.Variant == nil {
			continue
		}
		out = append(out, o)
	}
	b.Overlays = out
}

// RemoveOverlaySpan removes the span with the given ID from the source-side
// overlay of type t, reporting whether it was found.
func (b *Block) RemoveOverlaySpan(t OverlayType, id string) bool {
	o := b.OverlayOf(t)
	if o == nil {
		return false
	}
	for i := range o.Spans {
		if o.Spans[i].ID == id {
			o.Spans = append(o.Spans[:i], o.Spans[i+1:]...)
			return true
		}
	}
	return false
}
