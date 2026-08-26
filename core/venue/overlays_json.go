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
	Variant *model.VariantKey `json:"variant,omitempty"`
	Layer   string            `json:"layer,omitempty"`
	Spans   []spanWire        `json:"spans,omitempty"`
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
	if len(overlays) == 0 {
		return []byte(emptyOverlaysJSON), nil
	}
	wire := make([]overlayWire, len(overlays))
	for i, o := range overlays {
		w := overlayWire{Type: o.Type, Variant: o.Variant, Layer: o.Layer}
		if len(o.Spans) > 0 {
			w.Spans = make([]spanWire, len(o.Spans))
			for j, s := range o.Spans {
				sw := spanWire{ID: s.ID, Range: s.Range, Props: s.Props}
				if s.Value != nil {
					data, err := json.Marshal(s.Value)
					if err != nil {
						return nil, fmt.Errorf("marshal overlay span value (%s): %w", model.PayloadTypeName(s.Value), err)
					}
					sw.Value = &payloadEnvelope{Type: model.PayloadTypeName(s.Value), Data: data}
				}
				w.Spans[j] = sw
			}
		}
		wire[i] = w
	}
	return json.Marshal(wire)
}

// UnmarshalOverlays reverses MarshalOverlays, rehydrating each span's typed
// Value through the model payload registry. An empty / "[]" / "null" input
// yields nil overlays.
func UnmarshalOverlays(data []byte) ([]model.Overlay, error) {
	if s := strings.TrimSpace(string(data)); s == "" || s == emptyOverlaysJSON || s == "null" {
		return nil, nil
	}
	var wire []overlayWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if len(wire) == 0 {
		return nil, nil
	}
	overlays := make([]model.Overlay, len(wire))
	for i, w := range wire {
		o := model.Overlay{Type: w.Type, Variant: w.Variant, Layer: w.Layer}
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
		overlays[i] = o
	}
	return overlays, nil
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
