package model

// The target overlay accessors address the overlays on a translation by the
// locale the target accessors take. A locale reaches the overlays in Overlays
// that name its key. The empty locale reaches the overlays on the translation
// a reader filed under no language (TargetEdition("")): that translation has
// no key, because the zero key names the edition the block was read in, so its
// overlays sit apart from Overlays, beside it. Code that reads or writes the
// overlays of the translation it took from TargetRuns(locale) goes through
// these, never through SegmentationFor(Variant(locale)), which for the empty
// locale reaches the source's overlays.

// TargetSegmentation returns the primary segmentation overlay on the
// translation TargetRuns(locale) reads, or nil.
func (b *Block) TargetSegmentation(locale LocaleID) *Overlay {
	return b.TargetSegmentationLayer(locale, LayerPrimary)
}

// TargetSegmentationLayer returns the segmentation overlay of layer ("" = the
// primary sentence segmentation) on the translation TargetRuns(locale) reads,
// or nil.
func (b *Block) TargetSegmentationLayer(locale LocaleID, layer string) *Overlay {
	key := Variant(locale)
	if key.IsZero() {
		return findOverlay(b.unlabelledOverlays, OverlaySegmentation, layer)
	}
	return b.SegmentationLayerFor(key, layer)
}

// SetTargetSegmentation replaces the primary segmentation overlay on the
// translation SetTargetRuns(locale) writes with one carrying spans. Empty spans
// removes it.
func (b *Block) SetTargetSegmentation(locale LocaleID, spans []Span) {
	b.SetTargetSegmentationLayer(locale, LayerPrimary, spans)
}

// SetTargetSegmentationLayer replaces the segmentation overlay of layer on the
// translation SetTargetRuns(locale) writes with one carrying spans, leaving
// other layers as they are. Empty spans removes that layer.
func (b *Block) SetTargetSegmentationLayer(locale LocaleID, layer string, spans []Span) {
	key := Variant(locale)
	if key.IsZero() {
		b.unlabelledOverlays = replaceSegmentation(b.unlabelledOverlays, EditionKey{}, layer, spans)
		return
	}
	b.SetSegmentationLayer(key, layer, spans)
}

// UnlabelledOverlays returns the overlays on the translation filed under no
// language, each naming the zero key. The slice is the block's own: treat it
// as read-only. A codec that carries that translation carries these with it.
func (b *Block) UnlabelledOverlays() []Overlay { return b.unlabelledOverlays }

// SetUnlabelledOverlays replaces the overlays on the translation filed under
// no language with overlays, which the block takes as its own. Each is filed
// under the zero key, whatever edition it named.
func (b *Block) SetUnlabelledOverlays(overlays []Overlay) {
	if len(overlays) == 0 {
		b.unlabelledOverlays = nil
		return
	}
	for i := range overlays {
		overlays[i].Edition = EditionKey{}
	}
	b.unlabelledOverlays = overlays
}

// findOverlay returns the first overlay in overlays of type t and layer, or
// nil.
func findOverlay(overlays []Overlay, t OverlayType, layer string) *Overlay {
	for i := range overlays {
		o := &overlays[i]
		if o.Type == t && o.Layer == layer {
			return o
		}
	}
	return nil
}
