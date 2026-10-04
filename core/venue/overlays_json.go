package venue

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// This is the canonical JSON codec for a block's stand-off overlays — the
// positional, run-anchored layers (segmentation, term, entity, term-candidate,
// qa, alignment, and any plugin-defined type) that ride alongside a block's
// source runs. It lives in the framework-only bowrain/core module so BOTH the
// JSON sync-pull projection (host/venue/client) and the server-side content
// store (bowrain/store, which may import bowrain/core) share one implementation
// rather than mirroring it — the store delegates here.
//
// The proto sync-push path does NOT use this codec: it carries overlays as the
// typed OverlayMessage (see convert.go / the SyncBlock.overlays field). This
// JSON form is the labeled lossy projection used by the REST pull wire, where
// the sibling fields (annotations, skeleton, …) are already JSON blobs.
//
// The model types carry JSON tags on every field, so the wire shape below is
// the model's own JSON — EXCEPT Span.Value, a polymorphic model.Payload
// interface that plain encoding/json cannot rehydrate to its concrete type.
// That one field crosses as a type-discriminated {"type","data"} envelope,
// rehydrated through the model payload registry (model.NewPayload) so every
// registered overlay kind round-trips faithfully and an unknown kind degrades
// to a GenericAnnotation rather than being silently dropped.

// emptyOverlaysJSON is the byte-stable encoding of a block with no overlays,
// matching the store `overlays` column default so a nil/empty round-trip is a
// no-op.
const emptyOverlaysJSON = "[]"

// overlayWire mirrors model.Overlay's own JSON shape; only Span.Value needs a
// discriminated envelope (see payloadEnvelope).
type overlayWire struct {
	Type    model.OverlayType `json:"type"`
	Variant *model.EditionKey `json:"variant,omitempty"`
	Layer   string            `json:"layer,omitempty"`
	Spans   []spanWire        `json:"spans,omitempty"`
}

// editionWire is the wire form of the edition an overlay names: absent for the
// zero key, the edition the block was read in.
func editionWire(k model.EditionKey) *model.EditionKey {
	if k.IsZero() {
		return nil
	}
	return &k
}

// spanWire mirrors model.Span but carries Value as a discriminated envelope so
// the polymorphic model.Payload interface can be reconstructed on decode.
type spanWire struct {
	ID    string            `json:"id,omitempty"`
	Range model.Anchor      `json:"range"`
	Props map[string]string `json:"props,omitempty"`
	Value *payloadEnvelope  `json:"value,omitempty"`
}

// payloadEnvelope carries an overlay span value's concrete type name alongside
// its JSON so the polymorphic model.Payload interface can be reconstructed —
// the same discriminated shape the annotation codec uses.
type payloadEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// MarshalOverlays encodes a block's stand-off overlays as JSON. Nil/empty
// overlays encode to the byte-stable "[]" default so an unset round-trip is a
// no-op.
func MarshalOverlays(overlays []model.Overlay) ([]byte, error) {
	return marshalOverlays(overlays, nil)
}

// MarshalBlockOverlays encodes every overlay b carries: Overlays, then the
// overlays on the translation filed under no language, each with an explicit
// empty "variant" (the zero key, the edition the block was read in, leaves
// "variant" out). A codec that carries that translation carries its overlays
// through this pair; UnmarshalBlockOverlays reverses it.
func MarshalBlockOverlays(b *model.Block) ([]byte, error) {
	return marshalOverlays(b.Overlays, b.UnlabelledOverlays())
}

func marshalOverlays(overlays, unlabelled []model.Overlay) ([]byte, error) {
	if len(overlays)+len(unlabelled) == 0 {
		return []byte(emptyOverlaysJSON), nil
	}
	wire := make([]overlayWire, 0, len(overlays)+len(unlabelled))
	for _, o := range overlays {
		w, err := toOverlayWire(o)
		if err != nil {
			return nil, err
		}
		wire = append(wire, w)
	}
	for _, o := range unlabelled {
		w, err := toOverlayWire(o)
		if err != nil {
			return nil, err
		}
		w.Variant = &model.EditionKey{}
		wire = append(wire, w)
	}
	return json.Marshal(wire)
}

// toOverlayWire converts one overlay to its wire form.
func toOverlayWire(o model.Overlay) (overlayWire, error) {
	w := overlayWire{Type: o.Type, Variant: editionWire(o.Edition), Layer: o.Layer}
	if len(o.Spans) > 0 {
		w.Spans = make([]spanWire, len(o.Spans))
		for j, s := range o.Spans {
			sw := spanWire{ID: s.ID, Range: s.Range, Props: s.Props}
			if s.Value != nil {
				data, err := json.Marshal(s.Value)
				if err != nil {
					return overlayWire{}, fmt.Errorf("marshal overlay span value (%s): %w", model.PayloadTypeName(s.Value), err)
				}
				sw.Value = &payloadEnvelope{Type: model.PayloadTypeName(s.Value), Data: data}
			}
			w.Spans[j] = sw
		}
	}
	return w, nil
}

// UnmarshalOverlays reverses MarshalOverlays, rehydrating each span's typed
// Value through the model payload registry. An empty / "[]" / "null" input
// yields nil overlays. An overlay with an explicit empty "variant" sits on a
// translation filed under no language, which a list of overlays has no place
// for, so it is left out rather than read as an overlay on the edition the
// block was read in; UnmarshalBlockOverlays keeps it.
func UnmarshalOverlays(data []byte) ([]model.Overlay, error) {
	overlays, _, err := unmarshalOverlays(data)
	return overlays, err
}

// UnmarshalBlockOverlays reverses MarshalBlockOverlays onto b, replacing its
// overlays: an overlay with an explicit empty "variant" goes on the translation
// filed under no language, and every other on the edition its variant names.
// On an error b is left as it was.
func UnmarshalBlockOverlays(b *model.Block, data []byte) error {
	overlays, unlabelled, err := unmarshalOverlays(data)
	if err != nil {
		return err
	}
	b.Overlays = overlays
	b.SetUnlabelledOverlays(unlabelled)
	return nil
}

// unmarshalOverlays decodes overlays JSON into the overlays on an edition with
// a key (or the zero key) and those on the translation filed under no
// language.
func unmarshalOverlays(data []byte) (overlays, unlabelled []model.Overlay, err error) {
	if s := strings.TrimSpace(string(data)); s == "" || s == emptyOverlaysJSON || s == "null" {
		return nil, nil, nil
	}
	var wire []overlayWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, nil, err
	}
	for _, w := range wire {
		o := model.Overlay{Type: w.Type, Layer: w.Layer}
		if w.Variant != nil {
			o.Edition = *w.Variant
		}
		if len(w.Spans) > 0 {
			o.Spans = make([]model.Span, len(w.Spans))
			for j, sw := range w.Spans {
				sp := model.Span{ID: sw.ID, Range: sw.Range, Props: sw.Props}
				if sw.Value != nil {
					sp.Value = decodeSpanValue(*sw.Value)
				}
				o.Spans[j] = sp
			}
		}
		if w.Variant != nil && w.Variant.IsZero() {
			unlabelled = append(unlabelled, o)
			continue
		}
		overlays = append(overlays, o)
	}
	return overlays, unlabelled, nil
}

// decodeSpanValue rehydrates a span value from its discriminated envelope via
// the payload registry, degrading an unregistered or mismatched type to a
// GenericAnnotation carrying the raw fields rather than dropping the value.
func decodeSpanValue(env payloadEnvelope) model.Payload {
	p, ok := model.NewPayload(env.Type)
	if !ok {
		p = &model.GenericAnnotation{Kind: env.Type}
	}
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, p); err != nil {
			var fields map[string]any
			_ = json.Unmarshal(env.Data, &fields)
			return &model.GenericAnnotation{Kind: env.Type, Fields: fields}
		}
	}
	return p
}
