package bridgetest

import "github.com/neokapi/neokapi/core/model"

// FirstRuns returns the runs of the block's first source segment, or
// nil if the block has no source. Replaces b.FirstFragment().Spans
// walks in tests by giving direct access to the canonical Run
// sequence.
func FirstRuns(b *model.Block) []model.Run {
	if b == nil || len(b.Source) == 0 {
		return nil
	}
	return b.Source[0].Runs
}

// CountInlineCodes returns the number of non-text runs (Ph, PcOpen,
// PcClose, Sub) in the run sequence. Replaces len(frag.Spans) when
// the test only cares about the inline-code count.
func CountInlineCodes(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.Text == nil && r.Plural == nil && r.Select == nil {
			n++
		}
	}
	return n
}

// CountPcOpen counts opening paired-code runs.
func CountPcOpen(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.PcOpen != nil {
			n++
		}
	}
	return n
}

// CountPcClose counts closing paired-code runs.
func CountPcClose(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.PcClose != nil {
			n++
		}
	}
	return n
}

// CountPlaceholders counts placeholder runs (Ph).
func CountPlaceholders(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.Ph != nil {
			n++
		}
	}
	return n
}

// HasPcOpen reports whether the run sequence has any opening pc run.
func HasPcOpen(runs []model.Run) bool {
	for _, r := range runs {
		if r.PcOpen != nil {
			return true
		}
	}
	return false
}

// HasPcClose reports whether the run sequence has any closing pc run.
func HasPcClose(runs []model.Run) bool {
	for _, r := range runs {
		if r.PcClose != nil {
			return true
		}
	}
	return false
}

// HasPlaceholder reports whether the run sequence has any placeholder run.
func HasPlaceholder(runs []model.Run) bool {
	for _, r := range runs {
		if r.Ph != nil {
			return true
		}
	}
	return false
}

// HasInlineCode reports whether the run sequence has any non-text run.
func HasInlineCode(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil && r.Plural == nil && r.Select == nil {
			return true
		}
	}
	return false
}

// InlineCodeData returns the Data of the i-th non-text run (in the
// flat run sequence). Returns "" if i is out of range.
func InlineCodeData(runs []model.Run, i int) string {
	idx := 0
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			if idx == i {
				return r.PcOpen.Data
			}
			idx++
		case r.PcClose != nil:
			if idx == i {
				return r.PcClose.Data
			}
			idx++
		case r.Ph != nil:
			if idx == i {
				return r.Ph.Data
			}
			idx++
		}
	}
	return ""
}
