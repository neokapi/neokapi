package xliff

import (
	"strings"

	"golang.org/x/text/encoding"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
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
// The runs are the content; the IR is only its shape. Where the two
// disagree on how much text there is, the runs win: a text node the runs
// no longer reach emits nothing, and runs the IR has no node for are
// appended. Upstream okapi renders a `<target>` from its TextFragment
// alone (XLIFFSkeletonWriter.getSegmentedOutput → XLIFFContent), with no
// stored body to fall back on, so this is the same content decision made
// while keeping the inline-code fidelity a stored body buys.
func renderNativeWithRuns(nc *NativeContent, runs []model.Run) string {
	return renderNativeWithRunsOpts(nc, runs, renderOpts{})
}

func renderNativeWithRunsOpts(nc *NativeContent, runs []model.Run, opts renderOpts) string {
	if nc == nil {
		return ""
	}
	var b strings.Builder
	cur := &runCursor{texts: extractTextRuns(runs)}
	emitInlinesOpts(&b, nc.Inlines, cur, true, opts)
	cur.flushSurplus(&b, opts)
	return b.String()
}

// renderNativeVerbatimOpts re-emits a captured body exactly as it was read,
// with no run substitution. The caller must have established that the model
// still reads the way the capture does (sourceBodyIsCurrent); otherwise the
// captured bytes are a stale copy of content something has since edited.
func renderNativeVerbatimOpts(nc *NativeContent, opts renderOpts) string {
	if nc == nil {
		return ""
	}
	var b strings.Builder
	emitInlinesOpts(&b, nc.Inlines, nil, true, opts)
	return b.String()
}

// runCursor walks the model's text runs as the IR's translatable text nodes
// are emitted, one run per node in order. A nil *runCursor means the IR's own
// text is authoritative and no substitution happens at all.
type runCursor struct {
	texts []string
	idx   int
}

// next returns the text for the next translatable IR text node, and false when
// the runs are spent — which is a deletion, not an oversight: a tool that drops
// a trailing text run leaves the IR with a node the content no longer fills.
func (c *runCursor) next() (string, bool) {
	if c.idx >= len(c.texts) {
		return "", false
	}
	s := c.texts[c.idx]
	c.idx++
	return s, true
}

// flushSurplus emits the runs the IR had no node for, so text a tool appended
// past the end of the captured shape still reaches the file.
func (c *runCursor) flushSurplus(b *strings.Builder, opts renderOpts) {
	if c == nil {
		return
	}
	for ; c.idx < len(c.texts); c.idx++ {
		b.WriteString(opts.escapeText(c.texts[c.idx]))
	}
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

// sourceBodyIsCurrent reports whether a captured <source> body may still be
// re-emitted verbatim: the block's source must read exactly as it did when the
// capture was taken. See SourceBodyNativeAnnotation.SourceAsRead for why the
// witness is a snapshot rather than something derived from the capture.
func sourceBodyIsCurrent(sa *SourceBodyNativeAnnotation, block *model.Block) bool {
	if sa == nil || sa.SourceAsRead == "" {
		// No witness: either a genuinely empty source (in which case the
		// block's source is empty too and the capture is trivially current)
		// or an annotation that lost its snapshot, which must not be trusted.
		return model.RunsText(block.Source) == ""
	}
	return model.RunsText(block.Source) == sa.SourceAsRead
}

// emitInlinesOpts walks an inline tree and emits XML. Two things decide where a
// text node's bytes come from:
//
//   - `cur`, the model's run cursor. Nil means the IR's own text is
//     authoritative — a verbatim re-emission, or a skeleton fragment (the
//     whitespace between mrks) that no run corresponds to.
//   - `translatable`, which is false inside bpt/ept/ph/it inner content (opaque
//     native code, never translatable text) and true again inside a <sub>
//     sub-flow — that is the whole point of <sub>.
//
// With a cursor over translatable text, the runs are the content: a node the
// cursor cannot fill emits nothing, because the tool that shortened the runs
// deleted that text. Emitting the IR's copy instead is how a trimmed trailing
// space, and a pair of runs a tool collapsed into one, came back into the file.
//
// `opts` carries optional text-emission behaviors threaded down from
// the writer's OkapiCompatConfig (e.g. non-ASCII entity escaping).
func emitInlinesOpts(b *strings.Builder, inls []Inline, cur *runCursor, translatable bool, opts renderOpts) {
	for _, in := range inls {
		switch {
		case in.Text != nil:
			if cur == nil || !translatable {
				b.WriteString(opts.escapeText(in.Text.Content))
				continue
			}
			if text, ok := cur.next(); ok {
				b.WriteString(opts.escapeText(text))
			}
		case in.G != nil:
			b.WriteString("<g")
			writeAttrs(b, in.G.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.G.Children, cur, translatable, opts)
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
			emitInlinesOpts(b, in.Bpt.Inner, cur, false, opts)
			b.WriteString("</bpt>")
		case in.Ept != nil:
			b.WriteString("<ept")
			writeAttrs(b, in.Ept.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Ept.Inner, cur, false, opts)
			b.WriteString("</ept>")
		case in.Ph != nil:
			b.WriteString("<ph")
			writeAttrs(b, in.Ph.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Ph.Inner, cur, false, opts)
			b.WriteString("</ph>")
		case in.It != nil:
			b.WriteString("<it")
			writeAttrs(b, in.It.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.It.Inner, cur, false, opts)
			b.WriteString("</it>")
		case in.Mrk != nil:
			b.WriteString("<mrk")
			writeAttrs(b, in.Mrk.Attrs)
			b.WriteString(">")
			emitInlinesOpts(b, in.Mrk.Children, cur, translatable, opts)
			b.WriteString("</mrk>")
		case in.Sub != nil:
			b.WriteString("<sub")
			writeAttrs(b, in.Sub.Attrs)
			b.WriteString(">")
			// <sub> wraps a translatable sub-flow nested inside an
			// inline code. Even though the parent ph/bpt/it set
			// translatable=false, sub re-enables substitution for its
			// own children — that's the whole point of <sub>.
			emitInlinesOpts(b, in.Sub.Children, cur, true, opts)
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
		b.WriteString(xmlesc.Attr(a.Value))
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
		cur := &runCursor{texts: extractTextRuns(segRuns)}
		for _, in := range nc.Inlines {
			if in.Mrk != nil && mrkAttrIsSeg(in.Mrk) {
				emitInlinesOpts(&b, in.Mrk.Children, cur, true, opts)
			}
			// drop other inlines (whitespace between mrks, etc.)
		}
		cur.flushSurplus(&b, opts)
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
			cur := &runCursor{texts: extractTextRuns(segRuns)}
			emitInlinesOpts(&b, in.Mrk.Children, cur, true, opts)
			// Text a tool added past the segment's captured shape belongs inside
			// the segment's own mrk, not after the last one.
			cur.flushSurplus(&b, opts)
			b.WriteString("</mrk>")
			mrkIdx++
			continue
		}
		// Static skeleton content between or around mrks (often just
		// whitespace). It belongs to no segment, so no run governs it and the
		// IR's own bytes are emitted.
		emitInlinesOpts(&b, []Inline{in}, nil, true, opts)
	}
	return b.String()
}

// mrkAttrIsSeg reports whether a Mrk node is an mtype="seg"
// segmentation marker (vs. some other annotation marker like
// mtype="x-…" used for check notes etc.).
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
