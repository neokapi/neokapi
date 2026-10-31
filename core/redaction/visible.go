package redaction

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// visibleSeg is one stretch of the text VisibleText draws: text the source
// reads as well (mapped), or a token or a structure holding one, which has
// no place in the source's text.
type visibleSeg struct {
	vStart, vEnd int // byte span in the visible text
	sStart       int // byte offset in model.RunsText, for a mapped segment
	mapped       bool
}

// VisibleSource returns the text VisibleText draws for runs and a function
// that maps a byte span of it to the same span of model.RunsText(runs), the
// text an overlay on the runs is anchored in. A reader that reports spans in
// the text it was given (an entity extractor) reads the tokens and anchors
// what it found in the runs. The function reports false for an empty span, a
// span that touches a token, and a span inside a plural form or select case
// that holds one.
func VisibleSource(runs []model.Run) (string, func(start, end int) (int, int, bool)) {
	var (
		text strings.Builder
		segs []visibleSeg
		src  int
	)
	add := func(s string, mapped bool, srcLen int) {
		if s == "" && srcLen == 0 {
			return
		}
		v := text.Len()
		text.WriteString(s)
		segs = append(segs, visibleSeg{vStart: v, vEnd: text.Len(), sStart: src, mapped: mapped})
		src += srcLen
	}
	for _, r := range runs {
		switch {
		case r.Text != nil:
			add(r.Text.Text, true, len(r.Text.Text))
		case r.Ph != nil && isRedactionPh(r.Ph):
			add(r.Ph.Disp, false, 0)
		case r.Plural != nil || r.Select != nil:
			one := []model.Run{r}
			read := model.RunsText(one)
			shown, _ := VisibleText(one)
			add(shown, shown == read, len(read))
		}
	}
	toSource := func(start, end int) (int, int, bool) {
		if start >= end {
			return 0, 0, false
		}
		sStart, sEnd, found := -1, -1, false
		for _, sg := range segs {
			if sg.vEnd <= start || sg.vStart >= end {
				continue
			}
			if !sg.mapped {
				return 0, 0, false
			}
			found = true
			if start >= sg.vStart && start < sg.vEnd {
				sStart = sg.sStart + (start - sg.vStart)
			}
			if end > sg.vStart && end <= sg.vEnd {
				sEnd = sg.sStart + (end - sg.vStart)
			}
		}
		if !found || sStart < 0 || sEnd < 0 {
			return 0, 0, false
		}
		return sStart, sEnd, true
	}
	return text.String(), toSource
}
