package xliff

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

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
	if nc == nil {
		return ""
	}
	texts := extractTextRuns(runs)
	var b strings.Builder
	idx := 0
	emitInlines(&b, nc.Inlines, texts, &idx)
	// Append any leftover TextRuns the tool may have appended at the
	// end (e.g. some pseudo variants add a trailing marker).
	for ; idx < len(texts); idx++ {
		b.WriteString(xmlEscapeText(texts[idx]))
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

func emitInlines(b *strings.Builder, inls []Inline, texts []string, idx *int) {
	emitInlinesCtx(b, inls, texts, idx, true)
}

// emitInlinesCtx walks an inline tree and emits XML, with a
// `translatable` flag governing whether bare text nodes consume from
// the runs slice (true) or fall back to the native IR's verbatim text
// (false). The flag flips false when descending into bpt/ept/ph/it
// inner content (which is opaque native code, not translatable text)
// and back to true when descending into a <sub> sub-flow (which IS
// translatable — that's the whole point of <sub>).
func emitInlinesCtx(b *strings.Builder, inls []Inline, texts []string, idx *int, translatable bool) {
	for _, in := range inls {
		switch {
		case in.Text != nil:
			if translatable && *idx < len(texts) {
				b.WriteString(xmlEscapeText(texts[*idx]))
				*idx++
			} else {
				b.WriteString(xmlEscapeText(in.Text.Content))
			}
		case in.G != nil:
			b.WriteString("<g")
			writeAttrs(b, in.G.Attrs)
			b.WriteString(">")
			emitInlinesCtx(b, in.G.Children, texts, idx, translatable)
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
			emitInlinesCtx(b, in.Bpt.Inner, texts, idx, false)
			b.WriteString("</bpt>")
		case in.Ept != nil:
			b.WriteString("<ept")
			writeAttrs(b, in.Ept.Attrs)
			b.WriteString(">")
			emitInlinesCtx(b, in.Ept.Inner, texts, idx, false)
			b.WriteString("</ept>")
		case in.Ph != nil:
			b.WriteString("<ph")
			writeAttrs(b, in.Ph.Attrs)
			b.WriteString(">")
			emitInlinesCtx(b, in.Ph.Inner, texts, idx, false)
			b.WriteString("</ph>")
		case in.It != nil:
			b.WriteString("<it")
			writeAttrs(b, in.It.Attrs)
			b.WriteString(">")
			emitInlinesCtx(b, in.It.Inner, texts, idx, false)
			b.WriteString("</it>")
		case in.Mrk != nil:
			b.WriteString("<mrk")
			writeAttrs(b, in.Mrk.Attrs)
			b.WriteString(">")
			emitInlinesCtx(b, in.Mrk.Children, texts, idx, translatable)
			b.WriteString("</mrk>")
		case in.Sub != nil:
			b.WriteString("<sub")
			writeAttrs(b, in.Sub.Attrs)
			b.WriteString(">")
			// <sub> wraps a translatable sub-flow nested inside an
			// inline code. Even though the parent ph/bpt/it set
			// translatable=false, sub re-enables substitution for its
			// own children — that's the whole point of <sub>.
			emitInlinesCtx(b, in.Sub.Children, texts, idx, true)
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
func renderBodyWithSegments(nc *NativeContent, segs []*model.Segment) string {
	if nc == nil {
		return ""
	}
	hasMrks := false
	for _, in := range nc.Inlines {
		if in.Mrk != nil {
			hasMrks = true
			break
		}
	}
	if !hasMrks {
		var runs []model.Run
		if len(segs) > 0 && segs[0] != nil {
			runs = segs[0].Runs
		}
		return renderNativeWithRuns(nc, runs)
	}
	var b strings.Builder
	mrkIdx := 0
	for _, in := range nc.Inlines {
		if in.Mrk != nil {
			b.WriteString("<mrk")
			writeAttrs(&b, in.Mrk.Attrs)
			b.WriteString(">")
			var segRuns []model.Run
			if mrkIdx < len(segs) && segs[mrkIdx] != nil {
				segRuns = segs[mrkIdx].Runs
			}
			texts := extractTextRuns(segRuns)
			idx := 0
			emitInlines(&b, in.Mrk.Children, texts, &idx)
			b.WriteString("</mrk>")
			mrkIdx++
			continue
		}
		// Static skeleton content between or around mrks (often just
		// whitespace). Emit verbatim from native, no run substitution.
		dummyTexts := []string(nil)
		dummyIdx := 0
		emitInlines(&b, []Inline{in}, dummyTexts, &dummyIdx)
	}
	return b.String()
}
