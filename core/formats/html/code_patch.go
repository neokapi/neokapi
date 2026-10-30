package html

import (
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// An edit to a block's source is written as a patch of the block's own bytes.
// Every character and code the edit kept is copied as the document spells it:
// the whitespace extraction normalized, a quote, a bare ampersand, a code's
// markup. A code whose markup the edit changed (set_attribute) gets its new
// markup, and a code the edit added (mark) is inserted where it sits. Text the
// edit added is escaped the minimum element content needs, `&` and `<`.

// patchedSource writes a block whose source an edit changed as a patch of its
// bytes in the document, and returns what the block rendered to as read.
// original is the bytes a SkeletonOriginal recorded for the block, or nil when
// the reader kept its bytes as read. It reports false when the writer writes a
// translation of the block, the block is unedited, or the edit is one
// patchEdit declines.
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
	src, _ := block.Edition(model.EditionKey{})
	patched, ok = patchEdit(doc, read, src.Runs)
	return patched, asRead, ok
}

// patchItem is one element of a block's runs for the patch: a byte of a text
// run, or a code. start and end locate an element of the block as read in its
// original bytes.
type patchItem struct {
	text       byte
	code       bool
	key, data  string
	start, end int
}

// patchItems flattens runs into the elements patchEdit aligns. spans, when
// given, are where each run sits in the original bytes (alignRuns).
func patchItems(runs []model.Run, spans []runSpan) []patchItem {
	var items []patchItem
	for i, r := range runs {
		if r.Text == nil {
			k, _ := patchKey(r)
			it := patchItem{code: true, key: k, data: runData(r)}
			if spans != nil {
				it.start, it.end = spans[i].start, spans[i].end
			}
			items = append(items, it)
			continue
		}
		t := r.Text.Text
		for j := range len(t) {
			it := patchItem{text: t[j]}
			if spans != nil {
				it.start, it.end = spans[i].text[j], spans[i].text[j+1]
			}
			items = append(items, it)
		}
	}
	return items
}

// patchEdit writes edited, the block's runs after an edit, as original with
// the edit's changes. original is the block's bytes in the document, which read
// renders to once the reader's whitespace normalization is undone. The two run
// sequences are aligned element by element (format.MatchEdit): a text byte
// matches the same byte, a code the code of the same kind and id. A kept
// element is copied from original, a kept code with its new markup when the
// edit changed it. An added element goes right after the kept element before
// it, and a removed one is left out. Whitespace the reader peeled off the
// block's edges is copied. It reports false when either side holds a run
// other than text and codes, or original does not align with read.
func patchEdit(original string, read, edited []model.Run) (string, bool) {
	if !flatRuns(read) || !flatRuns(edited) {
		return "", false
	}
	spans, ok := alignRuns(original, read)
	if !ok {
		return "", false
	}
	from, to := patchItems(read, spans), patchItems(edited, nil)
	match := format.MatchEdit(len(from), len(to), func(i, j int) bool {
		a, b := from[i], to[j]
		if a.code || b.code {
			return a.code && b.code && a.key == b.key
		}
		return a.text == b.text
	})

	var b strings.Builder
	b.Grow(len(original) + 64)
	oc, j := 0, 0
	started := false
	var added []byte
	// insert writes the edited elements before index upto. Before the first
	// element as read, the bytes leading up to it are copied first, so added
	// content sits against the content it precedes.
	insert := func(upto int, lead int) {
		if j >= upto {
			return
		}
		if !started {
			b.WriteString(original[oc:lead])
			oc = lead
		}
		for ; j < upto; j++ {
			if e := to[j]; !e.code {
				added = append(added, e.text)
				continue
			}
			_, _ = htmlTextEscaper.WriteString(&b, string(added))
			added = added[:0]
			b.WriteString(to[j].data)
		}
		_, _ = htmlTextEscaper.WriteString(&b, string(added))
		added = added[:0]
	}
	for i, it := range from {
		if match[i] < 0 {
			b.WriteString(original[oc:it.start])
			oc = it.end
			started = true
			continue
		}
		insert(match[i], it.start)
		started = true
		b.WriteString(original[oc:it.start])
		e := to[match[i]]
		switch {
		case e.code && e.data != it.data:
			b.WriteString(e.data)
		case !e.code && e.text == '<' && !nextKept(match, i, len(to)),
			!e.code && e.text == '&' && !referenceTailKept(from, match, i, len(to)):
			// A bare `<` the document holds as text reads as text because of
			// the byte after it, and a bare `&` because of the name after it
			// (`&am` is no reference, `&amp;` is). The edit changed what
			// follows, so the kept character is escaped and cannot open a tag
			// or a reference with it.
			_, _ = htmlTextEscaper.WriteString(&b, string(e.text))
		default:
			b.WriteString(original[it.start:it.end])
		}
		oc = it.end
		j = match[i] + 1
	}
	insert(len(to), 0)
	b.WriteString(original[oc:])
	return b.String(), true
}

// nextKept reports whether the element after element i as read is still the
// element after it in the edit: the next element as read is kept right after
// it, or element i is the last of both sequences.
func nextKept(match []int, i, edited int) bool {
	if i+1 < len(match) {
		return match[i+1] >= 0 && match[i+1] == match[i]+1
	}
	return match[i]+1 == edited
}

// referenceTailKept reports whether what follows the `&` at element i as read
// is unchanged by the edit: each letter, digit or `#` after it, and the element
// that ends them, is kept in place, or the block ends after them on both
// sides.
func referenceTailKept(from []patchItem, match []int, i, edited int) bool {
	for k := i + 1; ; k++ {
		if !nextKept(match, k-1, edited) {
			return false
		}
		if k >= len(from) || !referenceNameByte(from[k]) {
			return true
		}
	}
}

// referenceNameByte reports whether it is a byte of a character reference's
// name: a letter, a digit, or the `#` of a numeric reference.
func referenceNameByte(it patchItem) bool {
	c := it.text
	return !it.code && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '#')
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
