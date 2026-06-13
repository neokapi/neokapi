package model

// This file defines the canonical editor-anchor overlay (format-maturity §2.3,
// the E2 "stable identity binding" criterion). An editor integration that
// pushes content into the native editor that owns a format (Word, Figma,
// Google Docs, a headless CMS) and pulls edits back needs a stable handle that
// survives an edit cycle *inside* that editor — incumbents re-derive identity
// from rendered text, which is fragile. The editor-anchor overlay is that
// handle: one overlay kind (OverlayEditorAnchor) whose spans carry a
// per-ecosystem EditorAnchor payload, a union discriminated by System, so the
// same Block round-trips through both the file path and the embedded path.
//
// The spec-case "anchor-survivability in-out operation" (format-spec-cases.md)
// that proves an anchor survives a real edit cycle in the native editor rides
// issue #847; this file defines only the canonical model and its serialization,
// it does not wire the spec-case harness.

// OverlayEditorAnchor marks an editor-integration anchor — a stable identity
// binding into the native editor that owns the format (format-maturity §2.3).
// Each span carries an [EditorAnchor] payload on its Value. Unlike the
// linguistic overlays (segmentation, term, entity, qa, alignment), an editor
// anchor is an integration binding set by a connector/add-in, not produced or
// consumed by a pipeline tool — so it is not a flow-editor IO-contract port.
const OverlayEditorAnchor OverlayType = "editor-anchor"

// EditorAnchorSystem names the native-editor ecosystem an [EditorAnchor] binds
// into. It discriminates the payload union: each system stores its own native
// handle in Ref (plus Extra), per the per-ecosystem mapping in
// format-maturity §2.3.
type EditorAnchorSystem string

const (
	// EditorSystemWord binds to a Word content control; Ref is the content
	// control tag (or id).
	EditorSystemWord EditorAnchorSystem = "word"
	// EditorSystemFigma binds to a Figma node; Ref is the node id, with an
	// optional sub-node character span in Range.
	EditorSystemFigma EditorAnchorSystem = "figma"
	// EditorSystemGDocs binds to a Google Docs named range; Ref is the named
	// range id (or name).
	EditorSystemGDocs EditorAnchorSystem = "gdocs"
	// EditorSystemCMS binds to a headless-CMS field; Ref is the entry + field
	// path (e.g. "entry-123#title").
	EditorSystemCMS EditorAnchorSystem = "cms"
)

// EditorAnchor is the per-ecosystem payload of an [OverlayEditorAnchor] span: a
// stable identity binding into the native editor that owns the format
// (format-maturity §2.3, E2). It is one payload type registered under
// "editor-anchor"; the concrete handle is a union discriminated by System:
//
//   - word:  Ref = content-control tag
//   - figma: Ref = node id, Range = optional sub-node character span
//   - gdocs: Ref = named-range id
//   - cms:   Ref = entry + field path
//
// Range is an optional sub-block anchor into the block's runs — e.g. a Figma
// node that covers only part of a Block — expressed in the same run-anchored
// coordinates as the carrying [Span]; when nil the anchor binds the whole span
// (its Span.Range). Extra carries system-specific metadata that does not fit
// Ref (revision ids, document ids, locale hints) without growing the union.
type EditorAnchor struct {
	System EditorAnchorSystem `json:"system"`
	Ref    string             `json:"ref"`
	Range  *RunRange          `json:"range,omitempty"`
	Extra  map[string]string  `json:"extra,omitempty"`
}

// TypeName returns the stable payload type name ("editor-anchor"), matching
// [OverlayEditorAnchor], so the wire and store layers rehydrate it from the
// payload registry like any other typed [Payload].
func (*EditorAnchor) TypeName() string { return string(OverlayEditorAnchor) }

// AddEditorAnchor attaches an editor anchor to the source side of the block,
// anchored at run range r under span id, carrying the per-ecosystem payload a.
// It is a thin wrapper over [Block.AddOverlaySpan] so editor anchors share the
// positional-overlay machinery (lookup, remap, wire/store round-trip) with the
// other overlays.
func (b *Block) AddEditorAnchor(id string, r RunRange, a *EditorAnchor) {
	b.AddOverlaySpan(OverlayEditorAnchor, Span{ID: id, Range: r, Value: a})
}

// EditorAnchors returns the editor-anchor payloads on the source-side overlay,
// in span order; nil when the block carries none.
func (b *Block) EditorAnchors() []*EditorAnchor {
	o := b.OverlayOf(OverlayEditorAnchor)
	if o == nil {
		return nil
	}
	out := make([]*EditorAnchor, 0, len(o.Spans))
	for i := range o.Spans {
		if a, ok := o.Spans[i].Value.(*EditorAnchor); ok {
			out = append(out, a)
		}
	}
	return out
}

// EditorAnchorByID returns the editor-anchor payload of the source-side span
// with the given id, or nil if absent (or if its Value is not an EditorAnchor).
func (b *Block) EditorAnchorByID(id string) *EditorAnchor {
	s := b.OverlaySpan(OverlayEditorAnchor, id)
	if s == nil {
		return nil
	}
	a, _ := s.Value.(*EditorAnchor)
	return a
}
