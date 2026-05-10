// Package openxml — per-source-run rPr preservation (#592).
//
// The native openxml reader normalises a small set of run-property
// toggles (b, i, u, strike, vertAlign, vanish, fontName) into the
// model's PcOpen/PcClose codes. Every other rPr child element
// (rStyle, rFonts, color, sz, szCs, lang, highlight, bCs, iCs, …)
// would historically be silently dropped on write because the
// renderWMLBlock pipeline only knows how to re-emit the model's
// toggle codes.
//
// To match Okapi's behaviour (which preserves the full <w:rPr>
// verbatim per source run — see RunBuilder.java in
// okapi/filters/openxml — and then lets WordStyleOptimisation lift
// common rPr into a synthesised paragraph style — see
// StyleOptimisation.java lines 96-129), we capture each source
// run's rPr child elements during parsing (parseRunProps populates
// runProps.rPrChildren), compute the per-paragraph intersection
// (commonRPrChildren), stash the intersection's serialisation in a
// Block annotation (openxmlSourceRPrAnnotationKey), and re-emit it
// from the writer on every <w:r> for the block. The WSO post-pass
// then lifts those redundant rPr into a paragraph style, exactly as
// upstream does.
//
// References:
//   - ECMA-376-1 §17.3.2.30  <w:rPr> Run Properties.
//   - okapi/filters/openxml/RunBuilder.java lines 73-188 — every
//     non-toggle rPr child becomes a tracked Property on the run.
//   - okapi/filters/openxml/RunMerger.java lines 156-229 — adjacent
//     runs are mergeable only when their RunProperties are equal,
//     so multi-run paragraphs surface heterogeneous rPr to the
//     writer rather than collapsing to a single rPr-less <w:r>.
//   - okapi/filters/openxml/StyleOptimisation.java lines 96-129,
//     204-237 — common rPr across runs is lifted into a synthesised
//     paragraph style (mirrored by style_optimization.go in this
//     package).

package openxml

import "strings"

// openxmlSourceRPrAnnotationKey is the model.Block.Annotations map
// key under which the writer reads the per-paragraph common rPr
// XML serialisation. The annotation value is a model.GenericAnnotation
// with Fields["xml"] holding the raw rPr-children XML to prepend on
// every emitted <w:r>.
const openxmlSourceRPrAnnotationKey = "openxml-source-rpr"

// commonRPrChildren returns the rPr child elements present and
// equal across every text-bearing source run in the paragraph.
// Mirrors upstream Okapi
// StyleOptimisation.Default.commonRunPropertiesOf
// (StyleOptimisation.java lines 204-237): a child is in the result
// only when EVERY run carries an exact-XML-equal entry. Order is
// preserved from the first contributing run.
//
// Sentinel runs (tab, image, footnoteRef, hyperlink wrappers,
// paragraph-level opaque, line breaks) are skipped — those are
// modelled as Placeholder/PcOpen/PcClose runs at the block level
// and don't carry a rPr the writer can reuse for surrounding text.
//
// When fewer than 1 text-bearing run is present (an empty paragraph
// or a paragraph with only sentinels), the result is empty (the
// writer falls through to its toggle-only rPr path).
func commonRPrChildren(runs []textRun) []rPrChild {
	var common []rPrChild
	seeded := false
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if r.text == "\n" {
			continue
		}
		// A text-bearing run with no rPr at all clears the common
		// set: Okapi treats "direct rPr empty" as breaking the
		// intersection (StyleOptimisation.java lines 224-228).
		if len(r.props.rPrChildren) == 0 {
			return nil
		}
		if !seeded {
			common = append(common, r.props.rPrChildren...)
			seeded = true
			continue
		}
		common = intersectRPrChildren(common, r.props.rPrChildren)
		if len(common) == 0 {
			return nil
		}
	}
	if !seeded {
		return nil
	}
	return common
}

// intersectRPrChildren returns the rPrChildren of `a` that are also
// in `b` by exact xml-equality. Order is preserved from `a`.
func intersectRPrChildren(a, b []rPrChild) []rPrChild {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make([]rPrChild, 0, len(a))
	for _, p := range a {
		for _, q := range b {
			if p.name == q.name && p.xml == q.xml {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// joinRPrChildren concatenates the children's raw XML in source
// order. Returns the empty string for an empty slice.
func joinRPrChildren(children []rPrChild) string {
	if len(children) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range children {
		b.WriteString(c.xml)
	}
	return b.String()
}
