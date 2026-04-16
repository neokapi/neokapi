package model

// Segment is a single segment within a Block's source or target content.
//
// Phase 2 migration note: during the Fragment → Run rewrite, Segment
// continues to carry Content *Fragment so format readers that haven't
// been ported yet keep working. Readers that emit Runs natively call
// SetRuns(); downstream tools that want the Run shape call Runs().
// The two forms are automatically kept in sync via the converters in
// run.go. Once every reader/writer/tool speaks Runs, the Fragment
// field will be removed and Runs becomes the sole content field.
type Segment struct {
	ID         string
	Content    *Fragment
	Properties map[string]string // Optional segment-level properties
}

// Runs returns the segment's content as a Run sequence. If SetRuns
// has been called, those runs are returned verbatim. Otherwise the
// current Content fragment is converted on the fly.
func (s *Segment) Runs() []Run {
	if s == nil {
		return nil
	}
	return FragmentToRuns(s.Content)
}

// SetRuns replaces the segment's content with the given Run
// sequence. The Fragment/Spans representation is derived via
// RunsToFragment so readers that still consume Content keep
// working during the migration.
func (s *Segment) SetRuns(runs []Run) {
	if s == nil {
		return
	}
	s.Content = RunsToFragment(runs)
}

// SetRunsText is a convenience that sets the segment's content to a
// single text run. Primarily used by simple readers (plaintext,
// json key-value) that have no inline markup.
func (s *Segment) SetRunsText(text string) {
	s.SetRuns([]Run{{Text: &TextRun{Text: text}}})
}

// NewRunsSegment constructs a Segment directly from a Run sequence.
func NewRunsSegment(id string, runs []Run) *Segment {
	seg := &Segment{ID: id}
	seg.SetRuns(runs)
	return seg
}
