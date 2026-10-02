package html

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// An edit that changes a block's codes and leaves its text as it was read
// (set_attribute respells a code's start tag, mark adds a code around words
// the block holds) is written as a patch of the block's own bytes: each held
// code's new markup replaces its old markup, each new code is inserted where
// it sits in the text, and every other byte, the whitespace extraction
// normalized and the characters the encoding pass would rewrite included,
// is copied from the document. An edit to the text takes the encoding pass
// (renderSourceRuns, htmlEncodeBlockText).

// patchedSource writes a block whose source edit changed only its codes as a
// patch of its bytes in the document, and returns what the block rendered to
// as read. original is the bytes a SkeletonOriginal recorded for the block, or
// nil when the reader kept its bytes as read. It reports false when the writer
// writes a translation of the block, the block is unedited, or the edit is
// one patchCodes declines.
func (w *Writer) patchedSource(block *model.Block, original []byte) (patched, asRead string, ok bool) {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return "", "", false
	}
	read, edited := block.SourceAsRead()
	if !edited {
		return "", "", false
	}
	asRead = model.RenderRunsWithData(read)
	doc := asRead
	if original != nil {
		doc = string(original)
	}
	patched, ok = patchCodes(doc, read, block.Source)
	return patched, asRead, ok
}

// patchCodes writes edited, which holds the text of read and differs from it
// only in its codes, as original with those code changes. original is the
// block's bytes in the document, which read renders to once the reader's
// whitespace normalization is undone. It reports false when the edit changes
// the text, moves or removes a code, or original does not align with read;
// the caller then renders the block.
func patchCodes(original string, read, edited []model.Run) (string, bool) {
	if !flatRuns(read) || !flatRuns(edited) || model.RunsText(read) != model.RunsText(edited) {
		return "", false
	}
	spans, ok := alignRuns(original, read)
	if !ok {
		return "", false
	}
	held := map[string]bool{}
	for _, r := range read {
		if k, ok := patchKey(r); ok {
			held[k] = true
		}
	}

	var b strings.Builder
	b.Grow(len(original) + 64)
	oc := 0
	copyTo := func(p int) bool {
		if p < oc {
			return false
		}
		b.WriteString(original[oc:p])
		oc = p
		return true
	}
	ri, off := 0, 0
	for _, e := range edited {
		if e.Text != nil {
			x := e.Text.Text
			for x != "" {
				if ri >= len(read) || read[ri].Text == nil {
					return "", false
				}
				rest := read[ri].Text.Text[off:]
				n := min(len(x), len(rest))
				if x[:n] != rest[:n] {
					return "", false
				}
				x, off = x[n:], off+n
				if off == len(read[ri].Text.Text) {
					ri, off = ri+1, 0
				}
			}
			continue
		}
		k, _ := patchKey(e)
		if off == 0 && ri < len(read) && read[ri].Text == nil {
			if rk, _ := patchKey(read[ri]); rk == k {
				if data := runData(e); data != runData(read[ri]) {
					if !copyTo(spans[ri].start) {
						return "", false
					}
					b.WriteString(data)
					oc = spans[ri].end
				}
				ri++
				continue
			}
		}
		if held[k] {
			return "", false
		}
		var at int
		switch {
		case ri < len(read) && read[ri].Text != nil:
			at = spans[ri].text[off]
		case e.PcClose != nil && ri > 0 && read[ri-1].Text != nil:
			at = spans[ri-1].end
		case ri < len(read):
			at = spans[ri].start
		case ri > 0:
			at = spans[ri-1].end
		}
		if !copyTo(at) {
			return "", false
		}
		b.WriteString(runData(e))
	}
	if ri != len(read) || !copyTo(len(original)) {
		return "", false
	}
	return b.String(), true
}

// runSpan is where a run of read sits in the block's original bytes. For a
// text run, text maps each byte offset of its text, its end included, to the
// offset in the original.
type runSpan struct {
	start, end int
	text       []int
}

// alignRuns locates each run of read in original. A code's markup sits there
// as the reader captured it. A text run's characters do too, except where the
// reader normalized whitespace: a run of whitespace in the text stands for a
// run of whitespace in the original, and whitespace the reader peeled off the
// edges of the text sits in the original between runs.
func alignRuns(original string, read []model.Run) ([]runSpan, bool) {
	spans := make([]runSpan, len(read))
	o := 0
	skipWS := func() {
		for o < len(original) && isHTMLWhitespace(rune(original[o])) {
			o++
		}
	}
	for i, r := range read {
		if r.Text == nil {
			data := runData(r)
			if !strings.HasPrefix(original[o:], data) {
				skipWS()
				if !strings.HasPrefix(original[o:], data) {
					return nil, false
				}
			}
			spans[i] = runSpan{start: o, end: o + len(data)}
			o += len(data)
			continue
		}
		t := r.Text.Text
		pos := make([]int, len(t)+1)
		if t != "" && !isHTMLWhitespace(rune(t[0])) {
			skipWS()
		}
		start := o
		for j := 0; j < len(t); {
			if !isHTMLWhitespace(rune(t[j])) {
				if o >= len(original) || original[o] != t[j] {
					return nil, false
				}
				pos[j] = o
				j, o = j+1, o+1
				continue
			}
			k := j
			for k < len(t) && isHTMLWhitespace(rune(t[k])) {
				k++
			}
			p := o
			for p < len(original) && isHTMLWhitespace(rune(original[p])) {
				p++
			}
			if p == o {
				return nil, false
			}
			for x := j; x < k; x++ {
				pos[x] = min(o+(x-j), p)
			}
			j, o = k, p
		}
		pos[len(t)] = o
		spans[i] = runSpan{start: start, end: o, text: pos}
	}
	for ; o < len(original); o++ {
		if !isHTMLWhitespace(rune(original[o])) {
			return nil, false
		}
	}
	return spans, true
}

// flatRuns reports whether runs hold only text and codes.
func flatRuns(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil && r.PcOpen == nil && r.PcClose == nil && r.Ph == nil {
			return false
		}
	}
	return true
}

// patchKey names a code by its kind and id.
func patchKey(r model.Run) (string, bool) {
	switch {
	case r.PcOpen != nil:
		return "open:" + r.PcOpen.ID, true
	case r.PcClose != nil:
		return "close:" + r.PcClose.ID, true
	case r.Ph != nil:
		return "ph:" + r.Ph.ID, true
	}
	return "", false
}

// runData is a code's markup.
func runData(r model.Run) string {
	switch {
	case r.PcOpen != nil:
		return r.PcOpen.Data
	case r.PcClose != nil:
		return r.PcClose.Data
	case r.Ph != nil:
		return r.Ph.Data
	}
	return ""
}
