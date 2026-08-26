package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// An Anchor says where inside a block something is: the whole block, one run, a
// span of characters, or a branch of a structured run. Findings, annotations
// and overlays all address content with it, so a position means the same thing
// wherever it is recorded and whatever produced it.
//
// Two properties shape it.
//
// Positions are RUN-RELATIVE, not offsets into flattened text. A boundary given
// as (run index, rune offset into that run) stays where it was put when a
// neighbouring run is rewritten, and it can sit either side of a placeholder —
// a flat character offset can do neither, because placeholders contribute no
// text and every offset after an edit shifts.
//
// Positions are PATHED. A block's content is a tree, not a list: a plural run
// holds a sequence per form and a select run one per case. Path walks to the
// sequence being addressed, so a term inside the `other` form of a plural is
// addressable rather than approximated by a position in the flattening.
type Anchor struct {
	Kind AnchorKind `json:"kind"`
	// Path is the run sequence being addressed. Empty means the block's own
	// top-level runs.
	Path RunPath `json:"path,omitempty"`
	// RunID identifies the run, for AnchorRun.
	RunID string `json:"runId,omitempty"`
	// Start and End bound a half-open span [Start, End), for AnchorRange.
	Start RunPos `json:"start,omitzero"`
	End   RunPos `json:"end,omitzero"`
	// Key names the branch, for AnchorForm: a plural form or a select case.
	Key string `json:"key,omitempty"`
}

// AnchorKind discriminates what an Anchor addresses.
type AnchorKind string

const (
	// AnchorBlock addresses a whole block.
	AnchorBlock AnchorKind = "block"
	// AnchorRun addresses one run by id.
	AnchorRun AnchorKind = "run"
	// AnchorRange addresses a half-open span of characters.
	AnchorRange AnchorKind = "range"
	// AnchorForm addresses one branch of a plural or select run.
	AnchorForm AnchorKind = "form"
)

// RunPos is a character boundary in a run sequence: an index into the sequence
// and a rune offset into that run's text. A run carrying no text — a
// placeholder, either half of a paired code — takes offset 0, and an index of
// len(runs) with offset 0 is the boundary past the last run.
type RunPos struct {
	Run    int `json:"run"`
	Offset int `json:"offset,omitempty"`
}

// RunPath walks into a block's nested run structure. Each step is an index into
// a sequence, a plural form, or a select case.
type RunPath []RunPathStep

// RunPathStep is one hop of a RunPath. Kind says which of the other fields
// carries the step.
type RunPathStep struct {
	Kind        RunPathStepKind
	Index       int
	PluralForm  PluralForm
	SelectValue string
}

// RunPathStepKind discriminates a RunPathStep.
type RunPathStepKind int

const (
	// StepIndex indexes a Run sequence.
	StepIndex RunPathStepKind = iota
	// StepPlural steps into a plural run's form.
	StepPlural
	// StepSelect steps into a select run's case.
	StepSelect
)

// MarshalJSON writes a step as a bare number, `{"plural":"<form>"}` or
// `{"select":"<value>"}` — the three shapes a step can take, so a path reads as
// the walk it describes rather than as a record of tagged unions.
func (s RunPathStep) MarshalJSON() ([]byte, error) {
	switch s.Kind {
	case StepIndex:
		return json.Marshal(s.Index)
	case StepPlural:
		return json.Marshal(struct {
			Plural PluralForm `json:"plural"`
		}{s.PluralForm})
	case StepSelect:
		return json.Marshal(struct {
			Select string `json:"select"`
		}{s.SelectValue})
	}
	return nil, fmt.Errorf("model: run path step has unknown kind %d", s.Kind)
}

// UnmarshalJSON decodes the three step shapes.
func (s *RunPathStep) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')) {
		var n int
		if err := json.Unmarshal(data, &n); err != nil {
			return fmt.Errorf("model: decode run path index step: %w", err)
		}
		*s = RunPathStep{Kind: StepIndex, Index: n}
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("model: decode run path step: %w", err)
	}
	if raw, ok := obj["plural"]; ok {
		var form PluralForm
		if err := json.Unmarshal(raw, &form); err != nil {
			return fmt.Errorf("model: decode run path plural step: %w", err)
		}
		*s = RunPathStep{Kind: StepPlural, PluralForm: form}
		return nil
	}
	if raw, ok := obj["select"]; ok {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("model: decode run path select step: %w", err)
		}
		*s = RunPathStep{Kind: StepSelect, SelectValue: value}
		return nil
	}
	return fmt.Errorf("model: run path step names neither an index, a plural form nor a select case: %s", trimmed)
}

// BlockAnchor addresses a whole block.
func BlockAnchor() Anchor { return Anchor{Kind: AnchorBlock} }

// RunAnchor addresses one run of the sequence at path.
func RunAnchor(path RunPath, runID string) Anchor {
	return Anchor{Kind: AnchorRun, Path: path, RunID: runID}
}

// FormAnchor addresses one branch of a plural or select run.
func FormAnchor(path RunPath, key string) Anchor {
	return Anchor{Kind: AnchorForm, Path: path, Key: key}
}

// SpanAnchor addresses the half-open span between two run positions.
func SpanAnchor(start, end RunPos) Anchor {
	return Anchor{Kind: AnchorRange, Start: start, End: end}
}

// RangeAnchor addresses the half-open rune span [startRune, endRune) of a run
// sequence's flattened text, resolved to run-relative positions.
func RangeAnchor(runs []Run, startRune, endRune int) Anchor {
	sr, so := runPosition(runs, startRune)
	er, eo := runPosition(runs, endRune)
	return SpanAnchor(RunPos{Run: sr, Offset: so}, RunPos{Run: er, Offset: eo})
}

// RangeAnchorForBytes addresses the half-open byte span [byteStart, byteEnd) of
// a run sequence's flattened text. Detectors that report byte offsets — term
// lookup, entity extraction — hand their positions over here, and the
// conversion to runes keeps the result right for content that is not ASCII.
func RangeAnchorForBytes(runs []Run, byteStart, byteEnd int) Anchor {
	text := RunsText(runs)
	toRune := func(b int) int {
		if b <= 0 {
			return 0
		}
		if b > len(text) {
			b = len(text)
		}
		return utf8.RuneCountInString(text[:b])
	}
	return RangeAnchor(runs, toRune(byteStart), toRune(byteEnd))
}

// IsEmpty reports whether a range anchor covers no content — its start and end
// are the same position. Distinct from IsZero: an empty range is a position
// that was computed and came out empty, not the absence of one.
func (a Anchor) IsEmpty() bool { return a.Start == a.End }

// IsZero reports whether the anchor addresses nothing at all — the zero value,
// meaning no position was recorded.
func (a Anchor) IsZero() bool {
	return a.Kind == "" && len(a.Path) == 0 && a.RunID == "" &&
		a.Start == (RunPos{}) && a.End == (RunPos{}) && a.Key == ""
}

// ExtractRuns returns the sub-sequence a range anchor covers. Boundary text
// runs are split at their offsets; a run carrying no text is atomic and comes
// through whole unless it sits on the exclusive end.
func (a Anchor) ExtractRuns(runs []Run) []Run {
	n := len(runs)
	if a.Start.Run < 0 || a.Start.Run > n || a.End.Run < a.Start.Run {
		return nil
	}
	out := make([]Run, 0, a.End.Run-a.Start.Run+1)
	for i := a.Start.Run; i <= a.End.Run && i < n; i++ {
		run := runs[i]
		if run.Text != nil {
			rs := []rune(run.Text.Text)
			s, e := 0, len(rs)
			if i == a.Start.Run {
				s = clampInt(a.Start.Offset, 0, len(rs))
			}
			if i == a.End.Run {
				e = clampInt(a.End.Offset, 0, len(rs))
			}
			if s < e {
				out = append(out, Run{Text: &TextRun{Text: string(rs[s:e])}})
			}
			continue
		}
		if i == a.End.Run && a.End.Offset == 0 {
			continue
		}
		out = append(out, run)
	}
	return out
}

// InBounds reports whether a range anchor still addresses valid positions: run
// indices within [0, len(runs)], start not past end, and each offset within its
// run. It is what a remapped or surviving overlay is checked against after the
// content it points into has been rewritten.
func (a Anchor) InBounds(runs []Run) bool {
	if a.Start.Run < 0 || a.End.Run < a.Start.Run || a.End.Run > len(runs) {
		return false
	}
	return offsetInBounds(runs, a.Start.Run, a.Start.Offset) &&
		offsetInBounds(runs, a.End.Run, a.End.Offset)
}

// TextSpan projects a range anchor to rune offsets [start, end) in the
// text-only flattening of runs, for consumers that highlight over flat text.
func (a Anchor) TextSpan(runs []Run) (start, end int) {
	return runTextOffset(runs, a.Start.Run, a.Start.Offset),
		runTextOffset(runs, a.End.Run, a.End.Offset)
}

// ByteSpan projects a range anchor to byte offsets [start, end) in the
// text-only flattening of runs.
func (a Anchor) ByteSpan(runs []Run) (start, end int) {
	text := RunsText(runs)
	rs, re := a.TextSpan(runs)
	return runeToByteOffset(text, rs), runeToByteOffset(text, re)
}
