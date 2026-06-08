package xliff2

import (
	"github.com/beevik/etree"

	"github.com/neokapi/neokapi/core/model"
)

// This file defines the xliff2-native IR for inline content inside
// <source> / <target> / <pc> / <mrk>. The reader populates these
// structures and attaches them to Block.Annotations; the writer prefers
// the native IR over the generic model.Run downconversion when
// reconstructing inline markup.
//
// The split mirrors the xliff 1.x package's pattern (core/formats/xliff/native.go)
// but the type set is XLIFF 2-specific: <ph>, <pc>, <sc>, <ec>, <mrk>,
// <sm>/<em>, <cp>. Every spec attribute (XLIFF 2.2 Part 1 §3.3) is a
// typed Go field — no string-keyed attribute bags — so downstream tools
// (QA, MT, segmentation) can act on attribute semantics directly.

// Content is the body of a <source>, <target>, <pc>, or <mrk> element:
// an ordered sequence of inline nodes (text + inline elements). The
// reader resolves <cp hex="X"/> markers to the actual code point in the
// adjacent Text node; the writer re-emits invalid XML chars as <cp>.
type Content struct {
	Inlines []Inline
}

// SegmentInlineAnnotation rides on Segment.Annotations and captures the
// full inline IR for one segment's <source> or <target> body. The writer
// walks this when reconstructing each segment with full attribute
// fidelity for inline codes (<ph>, <pc>, <sc>, <ec>, <mrk>, <sm>, <em>).
//
// Per-segment is the right level of granularity: an XLIFF 2 <unit> can
// hold many <segment>s, each with its own inline content. Per-Block
// annotations would require a slicing strategy and are lossy when the
// IR doesn't align cleanly with segment boundaries.
type SegmentInlineAnnotation struct {
	Content *Content
}

// AnnotationType identifies the annotation key.
func (a *SegmentInlineAnnotation) AnnotationType() string { return "xliff2:segment-inline" }

// OriginalDataAnnotation carries a unit's <originalData><data id=…/>
// entries — the verbatim original markup keyed by data id. Inline codes
// (<ph>/<sc>/<ec>/<pc>) reference these via dataRef / dataRefStart /
// dataRefEnd. The writer re-emits <originalData> iff at least one
// inline code on the unit references it.
type OriginalDataAnnotation struct {
	// Entries maps data id → ordered (text, cp) sequence per spec
	// (data content model is `(text|cp)*`).
	Entries map[string]*Content
}

// AnnotationType identifies the annotation key.
func (a *OriginalDataAnnotation) AnnotationType() string { return "xliff2:original-data" }

// SourceDOMAnnotation carries the original etree document AND the raw
// input bytes captured by the reader. The writer's round-trip mode
// patches the DOM in place and re-serializes — yielding a minimal
// diff for modified segments. When NO segment was patched, the writer
// short-circuits and emits Original verbatim, achieving byte-equal
// output that bypasses etree's own serialization quirks (multi-line
// attribute collapse, optional-character over-escaping, etc.).
//
// When this annotation is absent, the writer falls back to generation
// mode (fresh DOM build, canonical formatting).
type SourceDOMAnnotation struct {
	Doc      *etree.Document
	Original []byte
}

// AnnotationType identifies the annotation key.
func (a *SourceDOMAnnotation) AnnotationType() string { return "xliff2:source-dom" }

func init() {
	model.RegisterPayload("xliff2:segment-inline", func() any {
		return &SegmentInlineAnnotation{Content: &Content{}}
	})
	model.RegisterPayload("xliff2:original-data", func() any {
		return &OriginalDataAnnotation{Entries: make(map[string]*Content)}
	})
	model.RegisterPayload("xliff2:source-dom", func() any {
		return &SourceDOMAnnotation{}
	})
}

// Inline is one node in a body's inline content sequence. Exactly one
// of the pointer fields is set per node — the discriminant is
// "whichever non-nil field you find."
type Inline struct {
	Text *Text
	Ph   *Ph
	Pc   *Pc
	Sc   *Sc
	Ec   *Ec
	Mrk  *Mrk
	Sm   *Sm
	Em   *Em
}

// Text is a plain text node. Content holds the text after
// XML-entity decoding and after <cp hex="X"/> resolution to the actual
// code point.
type Text struct {
	Content string
}

// CodeAttrs collects all attributes shared by inline-code elements
// (<ph>, <pc>, <sc>, <ec>) per XLIFF 2.2 Part 1 §3.2.3 + §3.3.
// Empty strings mean "attribute absent on the source"; the writer
// only emits attributes that are non-empty AND not equal to the
// element's spec default. See SpecDefault* constants below.
type CodeAttrs struct {
	ID            string // §3.3 (REQUIRED on ph/pc/sc; CONDITIONAL on ec)
	CanCopy       string // "yes"|"no" (default "yes")
	CanDelete     string // "yes"|"no" (default "yes")
	CanReorder    string // "yes"|"firstNo"|"no" (default "yes")
	CanOverlap    string // "yes"|"no" — default varies: "no" on pc, "yes" on sc/ec
	CopyOf        string // NMTOKEN ref to base code id
	DataRef       string // ph/sc/ec only — ref to <data id=…>
	DataRefStart  string // pc only — ref to <data id=…>
	DataRefEnd    string // pc only — ref to <data id=…>
	Dir           string // "ltr"|"rtl"|"auto" (sc/ec only; pc inherits)
	Disp          string // ph/sc/ec only — display representation
	DispStart     string // pc only
	DispEnd       string // pc only
	Equiv         string // ph/sc/ec only — plain-text equivalent (default "")
	EquivStart    string // pc only
	EquivEnd      string // pc only
	SubFlows      string // ph/sc/ec only — NMTOKEN list of unit ids
	SubFlowsStart string // pc only
	SubFlowsEnd   string // pc only
	SubType       string // free-form secondary type (e.g. "xlf:b")
	Type          string // "fmt"|"ui"|"quote"|"link"|"image"|"other"
	Isolated      string // "yes"|"no" (sc/ec only; default "no")
	StartRef      string // ec only — ref to corresponding sc id (when not isolated)
}

// Ph is the <ph/> standalone code/placeholder. Empty content model.
type Ph struct {
	CodeAttrs
}

// Pc is the <pc>…</pc> paired/spanning code with inline children.
type Pc struct {
	CodeAttrs
	Children []Inline
}

// Sc is the <sc/> start code. Empty content model. Pairs with an
// Ec by id (or is "isolated" meaning the ec lives in a different unit).
type Sc struct {
	CodeAttrs
}

// Ec is the <ec/> end code. Empty content model. Pairs with an Sc
// via StartRef (or has its own ID when Isolated="yes").
type Ec struct {
	CodeAttrs
}

// MrkAttrs collects attributes for annotation markers (<mrk>/<sm>/<em>).
type MrkAttrs struct {
	ID        string // REQUIRED on mrk/sm
	Type      string // "generic"|"comment"|"term"|... (free-form)
	Translate string // "yes"|"no" (inherits)
	Ref       string // anyURI
	Value     string // free-form metadata
}

// Mrk is the <mrk>…</mrk> annotation marker with inline children.
type Mrk struct {
	MrkAttrs
	Children []Inline
}

// Sm is the <sm/> start marker. Empty content model. Pairs with an
// Em by ID + StartRef (the em's startRef must match the sm's id).
type Sm struct {
	MrkAttrs
}

// Em is the <em/> end marker. Empty content model. References its sm
// via StartRef.
type Em struct {
	StartRef string // REQUIRED — ref to corresponding sm id
}

// Walk visits every Inline in tree order. The callback may return
// false to abort traversal (early-exit propagates upward).
func Walk(inls []Inline, fn func(*Inline) bool) bool {
	for i := range inls {
		in := &inls[i]
		if !fn(in) {
			return false
		}
		switch {
		case in.Pc != nil:
			if !Walk(in.Pc.Children, fn) {
				return false
			}
		case in.Mrk != nil:
			if !Walk(in.Mrk.Children, fn) {
				return false
			}
		}
	}
	return true
}
