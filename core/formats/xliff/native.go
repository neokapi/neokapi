package xliff

import "github.com/neokapi/neokapi/core/model"

// NativeContent is the xliff-native representation of a <source>,
// <target>, or <seg-source> body. It preserves every inline element
// and attribute so the writer can emit byte-faithful output even when
// the model.Run downconversion (consumed by generic tools) would lose
// detail like ctype, xid, crc, or pos.
//
// The xliff reader attaches a *NativeContent for each segment of a
// Block via a per-span SegmentNativeAnnotation keyed on the segment's
// span id (see segNativeKey / targetSegNativeKey). The writer prefers
// the native content over the generic Runs when present, filling text
// positions from the (possibly tool-modified) Runs in order so
// pseudo-translate wrapping and AI-translated text still propagate.
type NativeContent struct {
	Inlines []Inline
}

// SegmentNativeAnnotation wraps a NativeContent so it can ride on
// model.Block.Annotations alongside other block-level data. Because the
// Run-based content model has no per-segment annotation slot, the
// reader stores one of these per source/target segment under a span-
// keyed block annotation (segNativeKey for source, targetSegNativeKey
// for a target locale). The segmentation overlay defines the span ids.
type SegmentNativeAnnotation struct {
	Content *NativeContent
}

// AnnotationType identifies the annotation key.
func (a *SegmentNativeAnnotation) AnnotationType() string { return "xliff:native" }

// SourceBodyNativeAnnotation rides on model.Block.Annotations and
// captures the full <source> body — including any <mrk mtype="seg">
// segmentation wrappers and the raw text between mrks. The writer
// walks this tree to reconstruct the body, substituting text inside
// each mrk from the runs of the matching source segment (the i-th
// span of the source segmentation overlay).
type SourceBodyNativeAnnotation struct {
	Content *NativeContent
}

// AnnotationType identifies the annotation key.
func (a *SourceBodyNativeAnnotation) AnnotationType() string { return "xliff:source-body" }

// TargetBodyNativeAnnotation captures the full <target> body native IR
// for one locale, parallel to SourceBodyNativeAnnotation.
type TargetBodyNativeAnnotation struct {
	Locale  model.LocaleID
	Content *NativeContent
}

// AnnotationType identifies the annotation key.
func (a *TargetBodyNativeAnnotation) AnnotationType() string { return "xliff:target-body" }

// TargetAttrsAnnotation carries the source <target> element's
// attributes (state, state-qualifier, xml:lang, custom-namespace)
// so the writer can rebuild the entire element verbatim instead of
// just injecting inner content. Crucial for empty/self-closing
// <target/> in source: inner-content injection lands outside the
// element, so we replace the whole element instead.
type TargetAttrsAnnotation struct {
	Attrs []Attr
}

// AnnotationType identifies the annotation key.
func (a *TargetAttrsAnnotation) AnnotationType() string { return "xliff:target-attrs" }

// DivergentSegSourceAnnotation marks a Block whose `<seg-source>` content
// disagreed with `<source>` content at read time and was discarded under
// okapi-compat (XLIFFFilter.java:2278). The writer post-process consults
// this marker to drop the literal seg-source bytes that come through from
// the skeleton, matching okapi's "log error and use un-segmented source"
// branch. No payload — presence is the signal.
type DivergentSegSourceAnnotation struct{}

// AnnotationType identifies the annotation key.
func (a *DivergentSegSourceAnnotation) AnnotationType() string { return "xliff:divergent-segsource" }

func init() {
	model.RegisterAnnotation("xliff:native", func() model.Annotation {
		return &SegmentNativeAnnotation{Content: &NativeContent{}}
	})
	model.RegisterAnnotation("xliff:source-body", func() model.Annotation {
		return &SourceBodyNativeAnnotation{Content: &NativeContent{}}
	})
	model.RegisterAnnotation("xliff:target-body", func() model.Annotation {
		return &TargetBodyNativeAnnotation{Content: &NativeContent{}}
	})
	model.RegisterAnnotation("xliff:target-attrs", func() model.Annotation {
		return &TargetAttrsAnnotation{}
	})
	model.RegisterAnnotation("xliff:divergent-segsource", func() model.Annotation {
		return &DivergentSegSourceAnnotation{}
	})
}

// Inline is one element in a <source> / <target> / <seg-source> body.
// Exactly one of the pointer fields is set per node — the discriminant
// is "whichever non-nil field you find."
type Inline struct {
	Text *Text
	G    *G
	X    *X
	Bx   *Bx
	Ex   *Ex
	Bpt  *Bpt
	Ept  *Ept
	Ph   *Ph
	It   *It
	Mrk  *Mrk
	Sub  *Sub
}

// Attr is one XML attribute on an inline element. Stored verbatim
// (including namespace prefix) so the writer round-trips unknown
// attributes — e.g. custom CMS namespaces — that the semantic fields
// don't surface.
type Attr struct {
	Space string
	Local string
	Value string
}

// Text is a plain text node. Content is the unescaped text.
type Text struct {
	Content string
}

// G is the <g> generic group element. Wraps inline children.
type G struct {
	Attrs    []Attr
	Children []Inline
}

// X is the standalone <x> placeholder (no children, no data).
type X struct {
	Attrs []Attr
}

// Bx is the begin-half of a paired placeholder with no data content.
type Bx struct {
	Attrs []Attr
}

// Ex is the end-half of a paired placeholder with no data content.
type Ex struct {
	Attrs []Attr
}

// Bpt is the begin-half of a paired placeholder. Inner contains the
// native code bytes (e.g. "<b>") and may include nested <sub> for
// translatable sub-flows.
type Bpt struct {
	Attrs []Attr
	Inner []Inline
}

// Ept is the end-half of a paired placeholder.
type Ept struct {
	Attrs []Attr
	Inner []Inline
}

// Ph is the standalone placeholder with native code data inside.
type Ph struct {
	Attrs []Attr
	Inner []Inline
}

// It is an isolated tag — a begin or end half of a paired code where
// the other half lives in a different segment.
type It struct {
	Attrs []Attr
	Inner []Inline
}

// Mrk is the <mrk> annotation marker.
type Mrk struct {
	Attrs    []Attr
	Children []Inline
}

// Sub is a translatable sub-flow nested inside an inline code.
type Sub struct {
	Attrs    []Attr
	Children []Inline
}

// AttrLookup returns the value of the named attribute (matching by
// local name, ignoring namespace prefix), or "" if not present.
func AttrLookup(attrs []Attr, name string) string {
	for _, a := range attrs {
		if a.Local == name {
			return a.Value
		}
	}
	return ""
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
		case in.G != nil:
			if !Walk(in.G.Children, fn) {
				return false
			}
		case in.Bpt != nil:
			if !Walk(in.Bpt.Inner, fn) {
				return false
			}
		case in.Ept != nil:
			if !Walk(in.Ept.Inner, fn) {
				return false
			}
		case in.Ph != nil:
			if !Walk(in.Ph.Inner, fn) {
				return false
			}
		case in.It != nil:
			if !Walk(in.It.Inner, fn) {
				return false
			}
		case in.Mrk != nil:
			if !Walk(in.Mrk.Children, fn) {
				return false
			}
		case in.Sub != nil:
			if !Walk(in.Sub.Children, fn) {
				return false
			}
		}
	}
	return true
}
