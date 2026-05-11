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

// openxmlPerRunRPrAnnotationKey is the model.Block.Annotations map
// key under which the writer reads the per-text-run rPr fragments —
// one entry per text-bearing source run, in source order, mirroring
// the upstream Okapi RunBuilder/RunMerger contract that every source
// run keeps its FULL rPr verbatim
// (okapi/filters/openxml/RunBuilder.java lines 73-188 and
// RunMerger.java lines 156-229: heterogeneous-rPr paragraphs surface
// multiple <w:r> elements, each with its own rPr, rather than
// collapsing to a single rPr-less <w:r>).
//
// The annotation value is a model.GenericAnnotation with
// Fields["fragments"] holding a []string. Each entry is the
// children-only XML of one source run's <w:rPr> (no <w:rPr>
// wrapper), in the same order as the text-bearing entries of
// `runs []textRun` (i.e. excluding sentinels and lone "\n" line
// breaks — see `commonRPrChildren` for the exact filtering rule).
// An entry is the empty string when the corresponding run had no
// rPr children at all.
//
// This is the reader-side capture half of the per-run rPr sidecar
// described in PARITY_NOTES.md (1083-* per-run rPr cluster). The
// writer wire-up (Phase 2) will consume this annotation; until then
// it is stashed but not read, so behaviour is unchanged.
//
// References:
//   - ECMA-376-1 §17.3.2 — <w:rPr> on <w:r>.
//   - okapi/filters/openxml/RunBuilder.java lines 73-188 — every
//     source run carries its full rPr through the builder.
//   - okapi/filters/openxml/RunMerger.java lines 156-229 —
//     RunProperties.equals gates run fusion, so heterogeneous rPr
//     stays heterogeneous on the way to the writer.
const openxmlPerRunRPrAnnotationKey = "openxml-per-run-rpr"

// openxmlPerRunSrcRunStartAnnotationKey is the model.Block.Annotations
// map key under which the writer reads the per-text-run "starts a new
// source <w:r>" boolean sidecar — one entry per text-bearing source
// run, in source order, aligned with the perRunRPr fragments.
//
// The reader sets textRun.srcRunStart on the FIRST emission of each
// source <w:r>; perRunSrcRunStartFlags copies that signal into the
// sidecar so the writer can decide whether a text run should reuse
// the still-open <w:r> from a preceding standalone <w:br/> / <w:tab/>
// or open a fresh <w:r>. Mirrors upstream Okapi RunBuilder
// (okapi/filters/openxml/RunBuilder.java:73-188) which scopes
// <w:br/> / <w:tab/> markup chunks to their containing <w:r>;
// RunMerger does not fuse runs across source-run boundaries
// (RunMerger.java:156-229). Per ECMA-376-1 §17.3.2.1 (CT_R) the
// <w:r> envelope is meaningful even when the rPr is identical to
// a neighbouring run.
//
// The annotation value is a model.GenericAnnotation with
// Fields["flags"] holding a []bool. Length equals the perRunRPr
// fragments length; entry i is true iff the i-th text-bearing source
// run was the FIRST content of a fresh source <w:r> (i.e. preceded
// by nothing — no text, tab, or break — within that <w:r>).
const openxmlPerRunSrcRunStartAnnotationKey = "openxml-per-run-src-run-start"

// perRunSrcRunStartFlags returns one bool per text-bearing source
// run, aligned with perRunRPrFragments. The boolean is true when
// the run was the FIRST textRun emitted from a fresh source <w:r>.
// Same filtering rule as perRunRPrFragments (skip sentinels and
// "\n" line breaks).
func perRunSrcRunStartFlags(runs []textRun) []bool {
	var out []bool
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if r.text == "\n" {
			continue
		}
		out = append(out, r.srcRunStart)
	}
	return out
}

// perRunRPrFragments returns one rPr-children XML fragment per
// text-bearing source run in `runs`, in source order.
//
// The filtering rule mirrors `commonRPrChildren`: sentinel runs
// (tab/image/footnoteRef/hyperlink wrappers/paragraph-level
// opaque/field markup) and lone "\n" line breaks are skipped — they
// don't carry text the writer reuses a per-run rPr for. The result
// length therefore equals the number of text-bearing source runs.
//
// Each entry is the children-only XML serialisation (no <w:rPr>
// wrapper); a run whose rPr was empty (or absent entirely) yields
// the empty string at its slot.
//
// Per RunBuilder.java lines 73-188 the source run's full rPr — both
// the toggle children (b/i/u/strike/vertAlign/vanish) AND the
// non-toggle children (rStyle/color/sz/lang/highlight/...) — must
// be preserved. `runProps.rPrChildren` already excludes the toggles
// the native writer reconstructs from PcOpen/PcClose; per-run rPr
// emission must therefore COMBINE the per-run sidecar (non-toggle
// children) with the writer's toggle reconstruction at write time.
// Phase 1 only captures the sidecar; Phase 2 wires it into the
// writer.
func perRunRPrFragments(runs []textRun) []string {
	var out []string
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if r.text == "\n" {
			continue
		}
		out = append(out, joinRPrChildren(r.props.rPrChildren))
	}
	return out
}

// commonRPrChildren returns the rPr child elements present and
// equal across every text-bearing source run in the paragraph.
// Mirrors upstream Okapi
// StyleOptimisation.Default.commonRunPropertiesOf
// (StyleOptimisation.java lines 204-237): a child is in the result
// only when EVERY run carries an equal entry. Order is preserved
// from the first contributing run.
//
// <w:rFonts> is special-cased: the common rFonts is the per-attribute
// intersection of every run's rFonts (an attribute is kept iff every
// run that has rFonts agrees on the value AND every run has rFonts).
// This mirrors upstream Okapi's effective behaviour: RunMerger fuses
// adjacent runs whose rFonts are mergeable (RunFonts.canBeMerged +
// RunFonts.merge — okapi/filters/openxml/RunFonts.java lines 190-247)
// BEFORE StyleOptimisation runs, so by the time WSO sees the runs all
// rFonts are already the merged consensus and exact equality holds.
// Native does not run RunMerger, so we approximate the merge here.
// Per ECMA-376-1 §17.3.2.26, rFonts attributes (ascii, hAnsi, cs,
// eastAsia, *Theme, hint) are independent and an rFonts may carry any
// subset, so the per-attribute intersection is itself a valid rFonts.
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
	// Track text-bearing runs so the rFonts merger sees the same
	// population the per-element intersection sees.
	var textRuns []textRun
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
		textRuns = append(textRuns, r)
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
	// Re-merge rFonts using per-attribute intersection. The naive
	// intersection above keeps the seed run's rFonts only if every
	// run carries the BYTE-EQUAL same rFonts; otherwise rFonts is
	// dropped from common (and from the synthesised paragraph style
	// via WSO). Replace any kept rFonts entry with the per-attribute
	// merged form.
	merged, mergedOK := mergeRFontsAcrossRuns(textRuns)
	out := make([]rPrChild, 0, len(common))
	rFontsInjected := false
	for _, p := range common {
		if p.name == "rFonts" {
			if !rFontsInjected {
				rFontsInjected = true
				if mergedOK {
					out = append(out, rPrChild{name: "rFonts", xml: merged})
				}
			}
			continue
		}
		out = append(out, p)
	}
	// If the seed run had rFonts but other runs differed → naive
	// intersection dropped it. The per-attribute merge may still
	// yield a non-empty rFonts, so inject it now.
	if !rFontsInjected && mergedOK {
		out = append(out, rPrChild{name: "rFonts", xml: merged})
	}
	return out
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

// mergeRFontsAcrossRuns computes the per-attribute intersection of
// every run's <w:rFonts>. Returns the synthesised rFonts XML and true
// iff every text-bearing run has an rFonts AND the intersection is
// non-empty.
//
// Mirrors upstream Okapi RunFonts.canBeMerged + RunFonts.merge
// (okapi/filters/openxml/RunFonts.java lines 190-315) at the granularity
// our post-write pass can observe (we don't have detection categories,
// so we use simple per-attribute consensus).
//
// Attribute order in the output matches the FIRST text-bearing run
// that has rFonts (mirroring upstream which preserves the surviving
// run's source order through the merge — RunFonts.getAttributes
// iterates ContentCategory enum values in declaration order, but
// retains source order for attributes that survive the merge).
func mergeRFontsAcrossRuns(runs []textRun) (string, bool) {
	if len(runs) == 0 {
		return "", false
	}
	var firstAttrs []rfontsAttr
	allAttrSets := make([]map[string]string, len(runs))
	for i, r := range runs {
		var rfonts *rPrChild
		for k := range r.props.rPrChildren {
			c := &r.props.rPrChildren[k]
			if c.name == "rFonts" {
				if rfonts != nil {
					return "", false // duplicate rFonts in one rPr → bail
				}
				rfonts = c
			}
		}
		if rfonts == nil {
			return "", false // some run lacks rFonts → not common
		}
		attrs, ok := parseRFontsAttrs(rfonts.xml)
		if !ok {
			return "", false
		}
		if i == 0 {
			firstAttrs = attrs
		}
		m := make(map[string]string, len(attrs))
		for _, a := range attrs {
			m[a.name] = a.value
		}
		allAttrSets[i] = m
	}
	// Walk first-run attribute order; keep iff every run has the
	// same name=value.
	var kept []rfontsAttr
	for _, a := range firstAttrs {
		ok := true
		for j := 1; j < len(allAttrSets); j++ {
			v, present := allAttrSets[j][a.name]
			if !present || v != a.value {
				ok = false
				break
			}
		}
		if ok {
			kept = append(kept, a)
		}
	}
	if len(kept) == 0 {
		return "", false
	}
	prefix := extractRFontsElemName(runs[0].props.rPrChildren)
	if prefix == "" {
		prefix = "w:rFonts"
	}
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(prefix)
	for _, a := range kept {
		b.WriteByte(' ')
		b.WriteString(a.name)
		b.WriteByte('=')
		q := a.quote
		if q == 0 {
			q = '"'
		}
		b.WriteByte(q)
		b.WriteString(a.value)
		b.WriteByte(q)
	}
	b.WriteString("/>")
	return b.String(), true
}

// extractRFontsElemName (rPrChild slice version) returns the prefixed
// element name of the first rFonts found in the children, e.g.
// "w:rFonts". Returns "" if not found or malformed.
func extractRFontsElemName(children []rPrChild) string {
	for _, c := range children {
		if c.name != "rFonts" {
			continue
		}
		if len(c.xml) < 2 || c.xml[0] != '<' {
			return ""
		}
		end := strings.IndexAny(c.xml[1:], " \t\n\r/>")
		if end < 0 {
			return ""
		}
		return c.xml[1 : 1+end]
	}
	return ""
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
