package model

// Segment is a single segment within a Block's source or target
// content. Segments carry a Run sequence as the canonical inline
// content representation defined by RFC 0001.
type Segment struct {
	ID         string
	Runs       []Run
	Properties map[string]string // Optional segment-level properties
}

// Text returns the plain-text flattening of the segment's runs:
// TextRun content verbatim, inline-code runs (Ph / PcOpen / PcClose /
// Sub) contribute nothing, plural / select take the 'other' branch
// (or the first form if 'other' is absent).
func (s *Segment) Text() string {
	if s == nil {
		return ""
	}
	var buf []rune
	textOnly(&buf, s.Runs)
	return string(buf)
}

func textOnly(buf *[]rune, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			*buf = append(*buf, []rune(r.Text.Text)...)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				textOnly(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				textOnly(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				textOnly(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				textOnly(buf, form)
				break
			}
		}
	}
}

// SetRuns replaces the segment's content with the given Run
// sequence.
func (s *Segment) SetRuns(runs []Run) {
	if s == nil {
		return
	}
	s.Runs = runs
}

// SetRunsText is a convenience that sets the segment's content to a
// single text run.
func (s *Segment) SetRunsText(text string) {
	s.SetRuns([]Run{{Text: &TextRun{Text: text}}})
}

// HasInlineCodes reports whether the segment has any non-text run.
// Replaces the legacy HasSpans() check.
func (s *Segment) HasInlineCodes() bool {
	if s == nil {
		return false
	}
	for _, r := range s.Runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// NewRunsSegment constructs a Segment directly from a Run sequence.
func NewRunsSegment(id string, runs []Run) *Segment {
	return &Segment{ID: id, Runs: runs}
}

// CodedText returns the segment's content as a PUA-marker coded
// string — the legacy hot-path form expected by format writers that
// haven't been ported to walk Runs directly. Materialized on demand
// via AsCodedText; never persist the output.
func (s *Segment) CodedText() string {
	if s == nil {
		return ""
	}
	coded, _ := AsCodedText(s.Runs)
	return coded
}

// Spans returns the legacy Span slice form of the segment's inline
// codes. Materialized on demand via AsCodedText; positions line up
// with the PUA markers in CodedText().
func (s *Segment) Spans() []*Span {
	if s == nil {
		return nil
	}
	_, spans := AsCodedText(s.Runs)
	return spans
}

// Fragment materializes the segment's runs as a legacy Fragment —
// the hot-path representation with PUA markers and a parallel Span
// list. Used by writers that still walk Fragment internally while
// the per-format port lands. Returns a fresh Fragment each call;
// mutations on the returned value are not propagated back into the
// segment.
func (s *Segment) Fragment() *Fragment {
	if s == nil {
		return nil
	}
	return RunsToFragment(s.Runs)
}
