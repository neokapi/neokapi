package model

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// A plural or select run holds one sequence per form, and RunsText reads only
// one of them (other, or the first present). A judgement made over RunsText
// therefore never sees the other forms: a translation can use a term in its
// `other` form and drop it in `one`, and nothing reading the flat text knows.
//
// A FormReading is the sequence read with one form chosen for each top-level
// plural and select. FormReadings returns enough of them that every form of
// every top-level plural and select is read once: the reading with every
// default form, then one reading per other form, which varies that run alone.
// A position found in a reading maps back to an anchor that names the form it
// sits in (FormReading.Anchor), so a finding inside a form points there.
//
// A plural nested inside a form is read at its default form. Nesting is rare
// in content, and reading every combination would multiply the readings.

// FormChoice is the form a reading takes for one top-level plural or select.
type FormChoice struct {
	// Run is the index of the plural or select run in the top-level sequence.
	Run int
	// Pivot is the variable the run selects on.
	Pivot string
	// Key is the form or case read: "one", "other", "female".
	Key string
	// Select marks a select run; a plural run leaves it false.
	Select bool
}

// FormReading is a run sequence read with one form chosen for each top-level
// plural and select.
type FormReading struct {
	// Text is the reading's flat text: text runs verbatim, codes contributing
	// nothing, each plural and select contributing its chosen form.
	Text string
	// Choices are the forms read, one per top-level plural and select, in run
	// order. Empty for a sequence with none.
	Choices []FormChoice
	// Varied is the choice this reading makes differently from the default
	// reading, or nil for the default reading itself.
	Varied *FormChoice

	runs []Run
	segs []readingSeg
}

// readingSeg is the stretch of a reading's text one top-level run contributes.
type readingSeg struct {
	start, end int // byte offsets into Text
	run        int // index of the top-level run
	choice     *FormChoice
}

// Form is the form this reading stands for: the varied choice's key, or the
// first choice's key for the default reading. Empty when the sequence holds no
// plural or select.
func (r FormReading) Form() string {
	if r.Varied != nil {
		return r.Varied.Key
	}
	if len(r.Choices) > 0 {
		return r.Choices[0].Key
	}
	return ""
}

// FormReadings returns the readings of runs that together read every form of
// every top-level plural and select: the default reading first, then one per
// non-default form, in run order and then key order. A sequence with no plural
// or select has one reading, whose Text is RunsText(runs).
func FormReadings(runs []Run) []FormReading {
	base := readWith(runs, nil)
	out := []FormReading{base}
	for i, r := range runs {
		def, keys, sel, pivot := branchKeys(r)
		for _, k := range keys {
			if k == def {
				continue
			}
			c := FormChoice{Run: i, Pivot: pivot, Key: k, Select: sel}
			reading := readWith(runs, map[int]string{i: k})
			reading.Varied = &c
			out = append(out, reading)
		}
	}
	return out
}

// FormReadingFor is the reading of runs that, for each top-level plural or
// select on pivot, takes the form key when the run has it, and the default
// form otherwise. It pairs a source with a translation's reading: the source's
// `one` form is what a translation's `one` form renders.
func FormReadingFor(runs []Run, pivot, key string) FormReading {
	choose := map[int]string{}
	for i, r := range runs {
		_, keys, _, p := branchKeys(r)
		if p == pivot && slices.Contains(keys, key) {
			choose[i] = key
		}
	}
	return readWith(runs, choose)
}

// readWith reads runs, taking the form choose names for a run index and the
// default form for every other plural and select.
func readWith(runs []Run, choose map[int]string) FormReading {
	var buf strings.Builder
	reading := FormReading{runs: runs}
	for i, r := range runs {
		switch r.Kind() {
		case RunKindText:
			start := buf.Len()
			buf.WriteString(r.Text.Text)
			reading.segs = append(reading.segs, readingSeg{start: start, end: buf.Len(), run: i})
		case RunKindPlural, RunKindSelect:
			def, _, sel, pivot := branchKeys(r)
			key := def
			if k, ok := choose[i]; ok {
				key = k
			}
			c := FormChoice{Run: i, Pivot: pivot, Key: key, Select: sel}
			reading.Choices = append(reading.Choices, c)
			start := buf.Len()
			buf.WriteString(RunsText(branchRuns(r, key)))
			reading.segs = append(reading.segs, readingSeg{start: start, end: buf.Len(), run: i, choice: &c})
		}
	}
	reading.Text = buf.String()
	return reading
}

// branchKeys returns a plural or select run's default form (other, or the
// first key in order), every key in order, whether it is a select, and its
// pivot. Any other run has no keys.
func branchKeys(r Run) (def string, keys []string, sel bool, pivot string) {
	switch r.Kind() {
	case RunKindPlural:
		for k := range r.Plural.Forms {
			keys = append(keys, string(k))
		}
		pivot = r.Plural.Pivot
	case RunKindSelect:
		for k := range r.Select.Cases {
			keys = append(keys, k)
		}
		sel, pivot = true, r.Select.Pivot
	default:
		return "", nil, false, ""
	}
	slices.Sort(keys)
	if slices.Contains(keys, string(PluralOther)) {
		def = string(PluralOther)
	} else if len(keys) > 0 {
		def = keys[0]
	}
	return def, keys, sel, pivot
}

// branchRuns is the sequence of a plural or select run's form key.
func branchRuns(r Run, key string) []Run {
	if r.Plural != nil {
		return r.Plural.Forms[PluralForm(key)]
	}
	if r.Select != nil {
		return r.Select.Cases[key]
	}
	return nil
}

// Anchor is the anchor of the half-open byte span [byteStart, byteEnd) of the
// reading's Text. A span inside one form is a range over that form's runs,
// pathed to it (the plural's index, then the form). A span over top-level text
// is a range over the top-level runs. A span that runs from top-level text into
// a form, or across two forms, is the form anchor of the first form it enters.
func (r FormReading) Anchor(byteStart, byteEnd int) Anchor {
	s, e := r.segAt(byteStart, false), r.segAt(byteEnd, true)
	if s < 0 || e < 0 {
		return Anchor{Kind: AnchorBlock}
	}
	ss, es := r.segs[s], r.segs[e]
	if s == e && ss.choice != nil {
		form := branchRuns(r.runs[ss.run], ss.choice.Key)
		a := RangeAnchorForBytes(form, byteStart-ss.start, byteEnd-ss.start)
		a.Path = formPath(*ss.choice)
		return a
	}
	if ss.choice == nil && es.choice == nil {
		return SpanAnchor(r.startPos(s, byteStart), r.topPos(es, byteEnd))
	}
	for i := s; i <= e; i++ {
		if c := r.segs[i].choice; c != nil {
			return FormAnchor(RunPath{{Kind: StepIndex, Index: c.Run}}, c.Key)
		}
	}
	return Anchor{Kind: AnchorBlock}
}

// formPath is the run path into a choice's form.
func formPath(c FormChoice) RunPath {
	step := RunPathStep{Kind: StepPlural, PluralForm: PluralForm(c.Key)}
	if c.Select {
		step = RunPathStep{Kind: StepSelect, SelectValue: c.Key}
	}
	return RunPath{{Kind: StepIndex, Index: c.Run}, step}
}

// segAt is the segment holding byte offset b: the one it starts inside, or for
// an end offset, the one it ends inside. -1 when no segment holds it.
func (r FormReading) segAt(b int, end bool) int {
	for i, sg := range r.segs {
		if sg.start == sg.end {
			continue
		}
		if !end && b >= sg.start && b < sg.end {
			return i
		}
		if end && b > sg.start && b <= sg.end {
			return i
		}
	}
	return -1
}

// startPos is the run position a span starting at byte offset b in segment i
// starts at. A span starting where a text run starts begins just past the
// text before it, as RangeAnchor places it, so a code between the two belongs
// to the span; at the very start of the text it is the first run.
func (r FormReading) startPos(i, b int) RunPos {
	if b > r.segs[i].start {
		return r.topPos(r.segs[i], b)
	}
	for p := i - 1; p >= 0; p-- {
		if r.segs[p].end > r.segs[p].start {
			return RunPos{Run: r.segs[p].run + 1}
		}
	}
	return RunPos{}
}

// topPos is the run position of byte offset b inside a top-level text
// segment. A position at the end of a text run is the start of the run after
// it, as RangeAnchor writes it.
func (r FormReading) topPos(sg readingSeg, b int) RunPos {
	if b >= sg.end {
		return RunPos{Run: sg.run + 1}
	}
	return RunPos{Run: sg.run, Offset: utf8.RuneCountInString(r.Text[sg.start:b])}
}

// TextIn is the text an anchor covers in runs, following its path into a
// plural or select form. Empty when the path does not resolve. A form anchor
// covers its whole form; a block anchor, the whole sequence.
func (a Anchor) TextIn(runs []Run) string {
	switch a.Kind {
	case AnchorBlock:
		return RunsText(runs)
	case AnchorForm:
		if len(a.Path) == 0 {
			return ""
		}
		// A form anchor's path ends on the plural's index, which
		// ResolveRunPath does not take: resolve the sequence holding it.
		seq, ok := ResolveRunPath(runs, a.Path[:len(a.Path)-1])
		if !ok {
			return ""
		}
		step := a.Path[len(a.Path)-1]
		if step.Kind != StepIndex || step.Index >= len(seq) {
			return ""
		}
		return RunsText(branchRuns(seq[step.Index], a.Key))
	}
	seq, ok := ResolveRunPath(runs, a.Path)
	if !ok {
		return ""
	}
	return RunsText(a.ExtractRuns(seq))
}
