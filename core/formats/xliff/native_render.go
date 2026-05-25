package xliff

import (
	"strings"

	"golang.org/x/text/encoding"

	"github.com/neokapi/neokapi/core/model"
)

// renderOpts controls optional escaping behaviors during IR rendering.
// Threaded through the emit walkers so OkapiCompatConfig flags can
// influence text emission without leaking into the IR data structures.
type renderOpts struct {
	// EncodableAs, when non-nil, drives encoder-aware entity escaping:
	// runes the encoder cannot represent are emitted as `&#xNNNN;`
	// entities. Used to mirror okapi's XMLEncoder behavior when the
	// source declared a non-UTF-8 encoding (windows-1252, ISO-8859-1).
	// nil = no encoding-aware escaping (UTF-8 sources or flag off).
	EncodableAs *encoding.Encoder
	// StripCREntities post-processes emitted text to drop &#xD; entity
	// sequences (matching okapi's CR-loss behavior).
	StripCREntities bool
}

func (o renderOpts) escapeText(s string) string {
	out := xmlEscapeText(s)
	if o.EncodableAs != nil {
		out = escapeUnencodableAsEntities(out, o.EncodableAs)
	}
	if o.StripCREntities {
		out = stripCDataCREntities(out)
	}
	return out
}

// renderNativeWithRuns serializes a NativeContent back to xliff inline
// bytes. Inline-element bytes (bpt/ept/ph/it/g/x/bx/ex/mrk/sub) come
// from the native IR with full attribute fidelity. Text-node bytes
// come from the supplied Runs in order, so tools that mutate text
// (pseudo-translate, AI-translate) propagate while inline-code
// attributes survive.
//
// If runs has fewer TextRuns than the native IR has Text nodes, the
// remaining native text is emitted verbatim (covers the case where
// a tool collapsed multiple TextRuns into one). If runs has more
// TextRuns, the surplus is appended at the end.
func renderNativeWithRuns(nc *NativeContent, runs []model.Run) string {
	return renderNativeWithRunsOpts(nc, runs, renderOpts{})
}

func renderNativeWithRunsOpts(nc *NativeContent, runs []model.Run, opts renderOpts) string {
	if nc == nil {
		return ""
	}
	texts := extractTextRuns(runs)
	var b strings.Builder
	idx := 0
	emitInlinesOpts(&b, nc.Inlines, texts, &idx, true, opts)
	for ; idx < len(texts); idx++ {
		b.WriteString(opts.escapeText(texts[idx]))
	}
	return b.String()
}

// extractTextRuns flattens a Run slice into the ordered text payloads.
// Plural and Select runs contribute their "other" branch's text so the
// downconversion stays consistent with model.Segment.Text().
func extractTextRuns(runs []model.Run) []string {
	var out []string
	collectTexts(&out, runs)
	return out
}

func collectTexts(out *[]string, runs []model.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			*out = append(*out, r.Text.Text)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[model.PluralOther]; ok {
				collectTexts(out, form)
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				collectTexts(out, form)
			}
		}
	}
}

// emitInlinesOpts walks an inline tree and emits XML, with a
// `translatable` flag governing whether bare text nodes consume from
// the runs slice (true) or fall back to the native IR's verbatim text
// (false). The flag flips false when descending into bpt/ept/ph/it
// inner content (which is opaque native code, not translatable text)
// and back to true when descending into a <sub> sub-flow (which IS
// translatable — that's the whole point of <sub>).
//
// `opts` carries optional text-emission behaviors threaded down from
// the writer's OkapiCompatConfig (e.g. non-ASCII entity escaping).
func emitInlinesOpts(b *strings.Builder, inls []Inline, texts []string, idx *int, translatable bool, opts renderOpts) {
	for _, in := range inls {
		switch {
		case in.Text != nil:
			if translatable && *idx < len(texts) {
				b.WriteString(opts.escapeText(texts[*idx]))
				*idx++
			} else {
				b.WriteString(opts.escapeText(in.Text.Content))
			}
		case in.G != nil:
			b.WriteString("<g")
			writeAttrs(b, in.G.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.G.Children, texts, idx, translatable, opts)
			b.WriteString("</g>")
		case in.X != nil:
			b.WriteString("<x")
			writeAttrs(b, in.X.Attrs)
			b.WriteString("/>")
		case in.Bx != nil:
			b.WriteString("<bx")
			writeAttrs(b, in.Bx.Attrs)
			b.WriteString("/>")
		case in.Ex != nil:
			b.WriteString("<ex")
			writeAttrs(b, in.Ex.Attrs)
			b.WriteString("/>")
		case in.Bpt != nil:
			b.WriteString("<bpt")
			writeAttrs(b, in.Bpt.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Bpt.Inner, texts, idx, false, opts)
			b.WriteString("</bpt>")
		case in.Ept != nil:
			b.WriteString("<ept")
			writeAttrs(b, in.Ept.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Ept.Inner, texts, idx, false, opts)
			b.WriteString("</ept>")
		case in.Ph != nil:
			b.WriteString("<ph")
			writeAttrs(b, in.Ph.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Ph.Inner, texts, idx, false, opts)
			b.WriteString("</ph>")
		case in.It != nil:
			b.WriteString("<it")
			writeAttrs(b, in.It.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.It.Inner, texts, idx, false, opts)
			b.WriteString("</it>")
		case in.Mrk != nil:
			b.WriteString("<mrk")
			writeAttrs(b, in.Mrk.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Mrk.Children, texts, idx, translatable, opts)
			b.WriteString("</mrk>")
		case in.Sub != nil:
			b.WriteString("<sub")
			writeAttrs(b, in.Sub.Attrs)
			b.WriteString(">")
			// <sub> wraps a translatable sub-flow nested inside an
			// inline code. Even though the parent ph/bpt/it set
			// translatable=false, sub re-enables substitution for its
			// own children — that's the whole point of <sub>.
			emitInlinesOpts(b, in.Sub.Children, texts, idx, true, opts)
			b.WriteString("</sub>")
		}
	}
}

// writeAttrs emits each attribute as ` ns:local="escaped-value"`,
// preserving the source's namespace prefix and order. The xliff reader
// captures attrs verbatim, so this preserves cms:*, MadCap:*, and other
// custom-namespace attributes that the well-known semantic fields don't
// surface.
func writeAttrs(b *strings.Builder, attrs []Attr) {
	for _, a := range attrs {
		b.WriteString(` `)
		if a.Space != "" {
			b.WriteString(a.Space)
			b.WriteString(`:`)
		}
		b.WriteString(a.Local)
		b.WriteString(`="`)
		b.WriteString(xmlEscapeAttr(a.Value))
		b.WriteString(`"`)
	}
}

// renderBodyWithSegments renders a full <source>/<target> body IR
// where top-level <mrk mtype="seg"> wrappers map to the supplied
// segments by position. Inside each mrk, text comes from the matching
// segment's TextRuns; everything else (mrk attributes, between-mrk
// whitespace, top-level inline codes) comes from the native IR
// verbatim.
//
// When the body contains no top-level mrks, this is equivalent to
// renderNativeWithRuns(nc, segs[0].Runs) — flat unsegmented body.
func renderBodyWithSegments(nc *NativeContent, segs []segView) string {
	return renderBodyWithSegmentsOpts(nc, segs, renderOpts{}, false)
}

// renderBodyWithSegmentsOpts is the opts-aware variant. unwrapSingleMrk
// strips a single top-level <mrk mtype="seg"> wrapper when there's
// exactly one such mrk in the body — mimicking okapi's behavior of
// dropping single-segment seg-source segmentation on round-trip.
func renderBodyWithSegmentsOpts(nc *NativeContent, segs []segView, opts renderOpts, unwrapSingleMrk bool) string {
	if nc == nil {
		return ""
	}
	mrkCount := 0
	for _, in := range nc.Inlines {
		if in.Mrk != nil {
			if mrkAttrIsSeg(in.Mrk) {
				mrkCount++
			}
		}
	}
	if mrkCount == 0 {
		var runs []model.Run
		if len(segs) > 0 {
			runs = segs[0].Runs
		}
		return renderNativeWithRunsOpts(nc, runs, opts)
	}
	if unwrapSingleMrk && mrkCount == 1 {
		// Walk the IR but render the single mrk's children inline
		// without the wrapper. Between-mrk content (whitespace usually)
		// is suppressed — okapi's unwrap collapses to just the inner
		// segment text.
		var b strings.Builder
		var segRuns []model.Run
		if len(segs) > 0 {
			segRuns = segs[0].Runs
		}
		texts := extractTextRuns(segRuns)
		idx := 0
		for _, in := range nc.Inlines {
			if in.Mrk != nil && mrkAttrIsSeg(in.Mrk) {
				emitInlinesOpts(&b, in.Mrk.Children, texts, &idx, true, opts)
			}
			// drop other inlines (whitespace between mrks, etc.)
		}
		return b.String()
	}
	var b strings.Builder
	mrkIdx := 0
	for _, in := range nc.Inlines {
		if in.Mrk != nil {
			b.WriteString("<mrk")
			writeAttrs(&b, in.Mrk.Attrs)
			b.WriteString(">")
			var segRuns []model.Run
			if mrkIdx < len(segs) {
				segRuns = segs[mrkIdx].Runs
			}
			texts := extractTextRuns(segRuns)
			idx := 0
			emitInlinesOpts(&b, in.Mrk.Children, texts, &idx, true, opts)
			b.WriteString("</mrk>")
			mrkIdx++
			continue
		}
		// Static skeleton content between or around mrks (often just
		// whitespace). Emit verbatim from native, no run substitution.
		dummyTexts := []string(nil)
		dummyIdx := 0
		emitInlinesOpts(&b, []Inline{in}, dummyTexts, &dummyIdx, true, opts)
	}
	return b.String()
}

// mrkAttrIsSeg reports whether a Mrk node is an mtype="seg"
// segmentation marker (vs. some other annotation marker like
// mtype="x-…" used for QA notes etc.).
func mrkAttrIsSeg(m *Mrk) bool {
	return AttrLookup(m.Attrs, "mtype") == "seg"
}

// irLacksInlinesNeededByRuns reports whether the native IR `nc` has
// fewer inline-code wrappers (bpt/ept/ph/it/x/bx/ex/g) than the segs'
// runs collectively contain. Used by the writer to detect when the
// target body IR (typically near-trivial when the source target was
// `<target></target>` or whitespace) can't carry the inline-code
// structure of pseudo-translated runs that were borrowed from the
// source. When this returns true the writer falls back to source body
// IR for structural emission.
//
// We compare counts rather than shapes — the runs are ordered, but the
// runs' inline-code positions don't necessarily map one-to-one onto IR
// positions, especially when runs have been generated by a tool. A
// strict count mismatch is enough to prove the target IR is unusable.
func irLacksInlinesNeededByRuns(nc *NativeContent, segs []segView) bool {
	if nc == nil {
		return true
	}
	irCodes := countInlineCodes(nc.Inlines)
	runCodes := 0
	for _, s := range segs {
		for _, r := range s.Runs {
			switch {
			case r.Ph != nil, r.PcOpen != nil, r.PcClose != nil:
				runCodes++
			}
		}
	}
	return runCodes > irCodes
}

// countInlineCodes recursively counts the number of model.Run inline-
// code entries the IR inline tree would map to when round-tripped via
// the reader's parseInlineContent. <g> contributes TWO (PcOpen on open,
// PcClose on close); paired bpt/ept and singleton ph/x each contribute
// one. <mrk> is NOT counted (structural, not an inline code) but its
// children are walked. <sub> children are also walked.
//
// This count is what the writer compares against runs' inline-code
// totals to detect IR-vs-runs mismatch (e.g. target IR is too sparse
// to carry the runs' inline structure).
func countInlineCodes(inls []Inline) int {
	n := 0
	for _, in := range inls {
		switch {
		case in.Bpt != nil:
			n++
			n += countInlineCodes(in.Bpt.Inner)
		case in.Ept != nil:
			n++
			n += countInlineCodes(in.Ept.Inner)
		case in.Ph != nil:
			n++
			n += countInlineCodes(in.Ph.Inner)
		case in.It != nil:
			n++
			n += countInlineCodes(in.It.Inner)
		case in.X != nil, in.Bx != nil, in.Ex != nil:
			n++
		case in.G != nil:
			n += 2 // PcOpen on open, PcClose on close
			n += countInlineCodes(in.G.Children)
		case in.Mrk != nil:
			n += countInlineCodes(in.Mrk.Children)
		case in.Sub != nil:
			n += countInlineCodes(in.Sub.Children)
		}
	}
	return n
}
