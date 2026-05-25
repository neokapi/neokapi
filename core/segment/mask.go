package segment

import (
	"fmt"
	"unicode"

	"github.com/neokapi/neokapi/core/model"
)

// MaskOptions controls how inline codes are flattened for boundary detection
// and how segment boundaries are trimmed. It mirrors the relevant Okapi SRX
// options. Options that only matter when materializing standalone segments to
// a bilingual format (inline-code renumbering, include-start/end-code shifting)
// are applied at projection time, not here — in the overlay model a code run
// is whole and belongs to whichever span's range covers it.
type MaskOptions struct {
	// TreatIsolatedCodesAsWhitespace renders an isolated code (placeholder or
	// subblock) as a single space in the flattened text, so a rule keyed on
	// whitespace can break across it. By default isolated codes contribute
	// nothing to the flattened text.
	TreatIsolatedCodesAsWhitespace bool
	// TrimLeadingWS / TrimTrailingWS move each segment boundary inward past
	// leading / trailing whitespace, leaving that whitespace as uncovered
	// inter-segment material (an implicit ignorable).
	TrimLeadingWS  bool
	TrimTrailingWS bool
}

// Flattened is the masked view of a run sequence that a boundary engine
// operates on, plus the mapping needed to project break offsets back to
// run-anchored spans. Only TextRun content is "real" text; inline-code,
// plural, and select runs are atomic (never split) and contribute either
// nothing or — for isolated codes under TreatIsolatedCodesAsWhitespace — a
// single space.
type Flattened struct {
	runs  []model.Run
	runes []rune // the masked text, rune by rune
	real  []bool // real[i]: runes[i] is TextRun content (counts toward the run mapping)
	opt   MaskOptions
}

// Flatten builds the masked view of runs under the given options.
func Flatten(runs []model.Run, opt MaskOptions) *Flattened {
	fl := &Flattened{runs: runs, opt: opt}
	for _, r := range runs {
		switch {
		case r.Text != nil:
			for _, ru := range r.Text.Text {
				fl.runes = append(fl.runes, ru)
				fl.real = append(fl.real, true)
			}
		case r.Ph != nil, r.Sub != nil, r.Plural != nil, r.Select != nil:
			// Atomic, non-splittable runs. Optionally stand in for a space so
			// whitespace-keyed rules can break across an isolated code; plural
			// and select constructs are likewise treated as opaque tokens.
			if opt.TreatIsolatedCodesAsWhitespace {
				fl.runes = append(fl.runes, ' ')
				fl.real = append(fl.real, false)
			}
			// PcOpen / PcClose: paired codes contribute nothing.
		}
	}
	return fl
}

// Text returns the masked string a boundary engine should segment.
func (fl *Flattened) Text() string { return string(fl.runes) }

// Runes returns the masked text as a rune slice (for engines that index by
// rune). The slice must not be mutated.
func (fl *Flattened) Runes() []rune { return fl.runes }

// Len reports the masked text length in runes.
func (fl *Flattened) Len() int { return len(fl.runes) }

// realOffset converts a rune offset into the masked text to the equivalent
// offset into the TextRun-only flattening that model.RunRangeFor expects.
func (fl *Flattened) realOffset(maskOff int) int {
	if maskOff > len(fl.runes) {
		maskOff = len(fl.runes)
	}
	n := 0
	for i := range maskOff {
		if fl.real[i] {
			n++
		}
	}
	return n
}

// Spans projects internal break offsets — rune indices into Text(), each the
// position at which a new segment begins — to ordered, run-anchored spans.
// Offsets outside (0, Len()) are ignored; duplicates and unsorted input are
// tolerated. Empty segments (covering only codes or trimmed-away whitespace)
// are dropped, so uncovered runs become implicit inter-segment material.
func (fl *Flattened) Spans(breaks []int) []model.Span {
	n := len(fl.runes)
	if n == 0 {
		return nil
	}
	edges := dedupeSorted(breaks, n)
	edges = append([]int{0}, edges...)
	edges = append(edges, n)

	spans := make([]model.Span, 0, len(edges)-1)
	for i := 0; i+1 < len(edges); i++ {
		s, e := edges[i], edges[i+1]
		if fl.opt.TrimLeadingWS {
			for s < e && unicode.IsSpace(fl.runes[s]) {
				s++
			}
		}
		if fl.opt.TrimTrailingWS {
			for e > s && unicode.IsSpace(fl.runes[e-1]) {
				e--
			}
		}
		rs, re := fl.realOffset(s), fl.realOffset(e)
		if rs >= re {
			continue // code-only / whitespace-only: implicit ignorable
		}
		spans = append(spans, model.Span{
			ID:    fmt.Sprintf("s%d", len(spans)+1),
			Range: model.RunRangeFor(fl.runs, rs, re),
		})
	}
	return spans
}

func dedupeSorted(breaks []int, n int) []int {
	seen := make(map[int]struct{}, len(breaks))
	out := make([]int, 0, len(breaks))
	for _, b := range breaks {
		if b <= 0 || b >= n {
			continue
		}
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		out = append(out, b)
	}
	insertionSort(out)
	return out
}

func insertionSort(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}
