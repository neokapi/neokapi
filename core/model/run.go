package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// This file defines the Run-based content model introduced by RFC
// 0001 Phase 2. A Run is one element in a Block's flat inline
// content sequence. Discriminated union keyed by the present field.
//
// Phase 2 migration note: the existing Fragment / Span model still
// exists and provides converters to/from Runs. Readers, writers,
// and tools migrate one at a time; once none of them references
// Fragment/Span anymore the old types will be removed in a final
// cleanup commit.

// PluralForm enumerates ICU plural forms.
type PluralForm string

const (
	PluralZero  PluralForm = "zero"
	PluralOne   PluralForm = "one"
	PluralTwo   PluralForm = "two"
	PluralFew   PluralForm = "few"
	PluralMany  PluralForm = "many"
	PluralOther PluralForm = "other"
)

// BlockContentType classifies a Block per RFC 0001. Distinct from
// the legacy Block.Type string so the two shapes can coexist during
// the Phase-2 migration without clashing on the field name.
type BlockContentType string

const (
	BlockContentJSXElement   BlockContentType = "jsx:element"
	BlockContentJSXAttribute BlockContentType = "jsx:attribute"
)

// PlaceholderKind discriminates how a placeholder was extracted.
type PlaceholderKind string

const (
	PlaceholderVariable PlaceholderKind = "variable"
	PlaceholderElement  PlaceholderKind = "element"
	PlaceholderNode     PlaceholderKind = "node"
	PlaceholderICUPivot PlaceholderKind = "icu-pivot"
)

// RunConstraints are the default editing constraints applied to
// matching runs, driven by vocabulary plus any Run-level override.
type RunConstraints struct {
	Deletable   bool `json:"deletable"`
	Cloneable   bool `json:"cloneable"`
	Reorderable bool `json:"reorderable"`
}

// TextRun is a plain text chunk.
type TextRun struct {
	Text string `json:"text"`
}

// PlaceholderRun is a self-closing placeholder run — a variable, a
// conditional JSX expression, a <br/>, a redaction, an icon, etc.
type PlaceholderRun struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	SubType     string          `json:"subType,omitempty"`
	Data        string          `json:"data"`
	Equiv       string          `json:"equiv"`
	Disp        string          `json:"disp,omitempty"`
	Constraints *RunConstraints `json:"constraints,omitempty"`
}

// PcOpenRun is the opening half of a paired code.
type PcOpenRun struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	SubType     string          `json:"subType,omitempty"`
	Data        string          `json:"data"`
	Equiv       string          `json:"equiv"`
	Disp        string          `json:"disp,omitempty"`
	Constraints *RunConstraints `json:"constraints,omitempty"`
}

// PcCloseRun is the closing half of a paired code. Shares ID with
// its PcOpen inside the same runs scope.
type PcCloseRun struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	SubType string `json:"subType,omitempty"`
	Data    string `json:"data"`
	Equiv   string `json:"equiv,omitempty"`
}

// SubRun is a reference to a subblock. Used for sub-filter output:
// when an outer format extracts a field whose value is itself a
// mini-document in another format, the subfilter produces a
// separate Block and the outer Block contains a SubRun pointing at
// it by id.
type SubRun struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Equiv string `json:"equiv"`
}

// PluralRun is the inner part of a structured plural construct.
type PluralRun struct {
	Pivot string               `json:"pivot"`
	Forms map[PluralForm][]Run `json:"forms"`
}

// SelectRun is the inner part of a structured select construct,
// symmetric to PluralRun but keyed by arbitrary string values.
type SelectRun struct {
	Pivot string           `json:"pivot"`
	Cases map[string][]Run `json:"cases"`
}

// Run is the discriminated union of inline content primitives.
// Exactly one of the pointer fields is non-nil per Run. JSON
// encoding matches RFC 0001: a Run is an object with exactly one
// of the keys text, ph, pcOpen, pcClose, sub, plural, or select.
type Run struct {
	Text    *TextRun        `json:"text,omitempty"`
	Ph      *PlaceholderRun `json:"ph,omitempty"`
	PcOpen  *PcOpenRun      `json:"pcOpen,omitempty"`
	PcClose *PcCloseRun     `json:"pcClose,omitempty"`
	Sub     *SubRun         `json:"sub,omitempty"`
	Plural  *PluralRun      `json:"plural,omitempty"`
	Select  *SelectRun      `json:"select,omitempty"`
}

// RunKind is a short string naming a Run's discriminator.
type RunKind string

const (
	RunKindText    RunKind = "text"
	RunKindPh      RunKind = "ph"
	RunKindPcOpen  RunKind = "pcOpen"
	RunKindPcClose RunKind = "pcClose"
	RunKindSub     RunKind = "sub"
	RunKindPlural  RunKind = "plural"
	RunKindSelect  RunKind = "select"
)

// Kind returns the run's discriminator. Returns the empty string
// for a zero Run (which is invalid).
func (r Run) Kind() RunKind {
	switch {
	case r.Text != nil:
		return RunKindText
	case r.Ph != nil:
		return RunKindPh
	case r.PcOpen != nil:
		return RunKindPcOpen
	case r.PcClose != nil:
		return RunKindPcClose
	case r.Sub != nil:
		return RunKindSub
	case r.Plural != nil:
		return RunKindPlural
	case r.Select != nil:
		return RunKindSelect
	}
	return ""
}

// RunID returns the run's id for kinds that carry one (ph, pcOpen,
// pcClose, sub). Returns empty string for other kinds.
func (r Run) RunID() string {
	switch {
	case r.Ph != nil:
		return r.Ph.ID
	case r.PcOpen != nil:
		return r.PcOpen.ID
	case r.PcClose != nil:
		return r.PcClose.ID
	case r.Sub != nil:
		return r.Sub.ID
	}
	return ""
}

// UnmarshalJSON decodes a single Run from its discriminated-union
// JSON shape. Rejects objects with zero or multiple discriminators.
func (r *Run) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("model: decode run: %w", err)
	}
	*r = Run{}
	seen := 0
	for key, val := range raw {
		switch key {
		case "text":
			r.Text = &TextRun{}
			if err := json.Unmarshal(val, r.Text); err != nil {
				return fmt.Errorf("model: decode text run: %w", err)
			}
			seen++
		case "ph":
			r.Ph = &PlaceholderRun{}
			if err := json.Unmarshal(val, r.Ph); err != nil {
				return fmt.Errorf("model: decode ph run: %w", err)
			}
			seen++
		case "pcOpen":
			r.PcOpen = &PcOpenRun{}
			if err := json.Unmarshal(val, r.PcOpen); err != nil {
				return fmt.Errorf("model: decode pcOpen run: %w", err)
			}
			seen++
		case "pcClose":
			r.PcClose = &PcCloseRun{}
			if err := json.Unmarshal(val, r.PcClose); err != nil {
				return fmt.Errorf("model: decode pcClose run: %w", err)
			}
			seen++
		case "sub":
			r.Sub = &SubRun{}
			if err := json.Unmarshal(val, r.Sub); err != nil {
				return fmt.Errorf("model: decode sub run: %w", err)
			}
			seen++
		case "plural":
			r.Plural = &PluralRun{}
			if err := json.Unmarshal(val, r.Plural); err != nil {
				return fmt.Errorf("model: decode plural run: %w", err)
			}
			seen++
		case "select":
			r.Select = &SelectRun{}
			if err := json.Unmarshal(val, r.Select); err != nil {
				return fmt.Errorf("model: decode select run: %w", err)
			}
			seen++
		}
	}
	if seen == 0 {
		return errors.New("model: run has no discriminator")
	}
	if seen > 1 {
		return fmt.Errorf("model: run has multiple discriminators (%d)", seen)
	}
	return nil
}

// MarshalJSON enforces that exactly one discriminator is set before
// emitting JSON.
func (r Run) MarshalJSON() ([]byte, error) {
	switch r.Kind() {
	case "":
		return nil, errors.New("model: run has no discriminator")
	case RunKindText:
		return json.Marshal(struct {
			Text *TextRun `json:"text"`
		}{r.Text})
	case RunKindPh:
		return json.Marshal(struct {
			Ph *PlaceholderRun `json:"ph"`
		}{r.Ph})
	case RunKindPcOpen:
		return json.Marshal(struct {
			PcOpen *PcOpenRun `json:"pcOpen"`
		}{r.PcOpen})
	case RunKindPcClose:
		return json.Marshal(struct {
			PcClose *PcCloseRun `json:"pcClose"`
		}{r.PcClose})
	case RunKindSub:
		return json.Marshal(struct {
			Sub *SubRun `json:"sub"`
		}{r.Sub})
	case RunKindPlural:
		return json.Marshal(struct {
			Plural *PluralRun `json:"plural"`
		}{r.Plural})
	case RunKindSelect:
		return json.Marshal(struct {
			Select *SelectRun `json:"select"`
		}{r.Select})
	}
	return nil, fmt.Errorf("model: run has unknown discriminator %q", r.Kind())
}

// Placeholder is metadata about a variable or element token
// referenced by the runs of a Block.
type Placeholder struct {
	Name       string          `json:"name"`
	Kind       PlaceholderKind `json:"kind"`
	JSType     string          `json:"jsType,omitempty"`
	SourceExpr string          `json:"sourceExpr"`
	Optional   bool            `json:"optional,omitempty"`
}

// BlockProperties mirrors klf.BlockProperties for use on Block.
type BlockProperties struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Component string `json:"component"`
	JSXPath   string `json:"jsxPath"`
	Element   string `json:"element"`
	LocNote   string `json:"locNote,omitempty"`
}

// BlockPreviewHints mirrors klf.BlockPreviewHints.
type BlockPreviewHints struct {
	StoryID      string                 `json:"storyId,omitempty"`
	SnapshotPath string                 `json:"snapshotPath,omitempty"`
	SampleValues map[string]interface{} `json:"sampleValues,omitempty"`
}

// ───────── Runs ↔ Fragment conversion (Phase-2 migration bridge) ─────────

// FragmentToRuns converts a Fragment (legacy CodedText + Spans) into
// a Run sequence. Marker characters in CodedText become placeholder
// or paired-code runs with metadata drawn from the corresponding
// entry in Spans.
//
// This is a bridge used while readers, writers, and tools are being
// migrated to the Run-based model. Once every reader emits Runs
// natively, FragmentToRuns becomes unused and can be removed.
func FragmentToRuns(f *Fragment) []Run {
	if f == nil {
		return nil
	}
	if len(f.Spans) == 0 {
		// Fast path: no markers to resolve.
		if f.CodedText == "" {
			return nil
		}
		return []Run{{Text: &TextRun{Text: f.CodedText}}}
	}
	var runs []Run
	spanIdx := 0
	var text []rune
	flushText := func() {
		if len(text) > 0 {
			runs = append(runs, Run{Text: &TextRun{Text: string(text)}})
			text = text[:0]
		}
	}
	for _, r := range f.CodedText {
		if !isMarker(r) {
			text = append(text, r)
			continue
		}
		flushText()
		if spanIdx >= len(f.Spans) {
			// Defensive: mismatched span count.
			continue
		}
		span := f.Spans[spanIdx]
		spanIdx++
		runs = append(runs, spanToRun(span, r))
	}
	flushText()
	return runs
}

// spanToRun lifts a legacy Span + marker into the appropriate Run
// kind. Opening/closing markers become PcOpen/PcClose; placeholder
// markers become Ph runs.
func spanToRun(span *Span, marker rune) Run {
	constraints := &RunConstraints{
		Deletable:   span.Deletable,
		Cloneable:   span.Cloneable,
		Reorderable: span.CanReorder,
	}
	switch marker {
	case MarkerOpening:
		return Run{PcOpen: &PcOpenRun{
			ID: span.ID, Type: span.Type, SubType: span.SubType,
			Data: span.Data, Equiv: span.EquivText, Disp: span.DisplayText,
			Constraints: constraints,
		}}
	case MarkerClosing:
		return Run{PcClose: &PcCloseRun{
			ID: span.ID, Type: span.Type, SubType: span.SubType,
			Data: span.Data, Equiv: span.EquivText,
		}}
	case MarkerPlaceholder:
		return Run{Ph: &PlaceholderRun{
			ID: span.ID, Type: span.Type, SubType: span.SubType,
			Data: span.Data, Equiv: span.EquivText, Disp: span.DisplayText,
			Constraints: constraints,
		}}
	}
	return Run{Text: &TextRun{Text: span.Data}}
}

// RunsToFragment converts a Run sequence back into the legacy
// Fragment/Span shape. Used by writers that still consume Fragment
// until Phase 2 migration is complete.
func RunsToFragment(runs []Run) *Fragment {
	f := &Fragment{}
	for _, r := range runs {
		switch {
		case r.Text != nil:
			f.AppendText(r.Text.Text)
		case r.Ph != nil:
			f.AppendSpan(runToSpan(r, SpanPlaceholder))
		case r.PcOpen != nil:
			f.AppendSpan(runToSpan(r, SpanOpening))
		case r.PcClose != nil:
			f.AppendSpan(runToSpan(r, SpanClosing))
		case r.Sub != nil:
			// SubRun is a sub-filter reference; emit a placeholder
			// span carrying the ref + equiv so writers that don't
			// understand subblocks at least preserve the slot.
			span := &Span{
				SpanType:    SpanPlaceholder,
				Type:        "sub",
				ID:          r.Sub.ID,
				Data:        r.Sub.Ref,
				EquivText:   r.Sub.Equiv,
				DisplayText: r.Sub.Equiv,
			}
			f.AppendSpan(span)
		case r.Plural != nil, r.Select != nil:
			// Plural / Select are structured constructs with no
			// direct Fragment equivalent. We emit the 'other'
			// branch's runs (or the first form) so flattened text
			// is non-empty; writers that want the full structure
			// must switch to Runs natively.
			f.AppendText(FlattenRuns([]Run{r}))
		}
	}
	return f
}

func runToSpan(r Run, kind SpanType) *Span {
	switch {
	case r.Ph != nil:
		return &Span{
			SpanType:    kind,
			Type:        r.Ph.Type,
			SubType:     r.Ph.SubType,
			ID:          r.Ph.ID,
			Data:        r.Ph.Data,
			EquivText:   r.Ph.Equiv,
			DisplayText: r.Ph.Disp,
			Deletable:   constraintOrTrue(r.Ph.Constraints, func(c RunConstraints) bool { return c.Deletable }),
			Cloneable:   constraintOrFalse(r.Ph.Constraints, func(c RunConstraints) bool { return c.Cloneable }),
			CanReorder:  constraintOrTrue(r.Ph.Constraints, func(c RunConstraints) bool { return c.Reorderable }),
		}
	case r.PcOpen != nil:
		return &Span{
			SpanType:    kind,
			Type:        r.PcOpen.Type,
			SubType:     r.PcOpen.SubType,
			ID:          r.PcOpen.ID,
			Data:        r.PcOpen.Data,
			EquivText:   r.PcOpen.Equiv,
			DisplayText: r.PcOpen.Disp,
			Deletable:   constraintOrTrue(r.PcOpen.Constraints, func(c RunConstraints) bool { return c.Deletable }),
			Cloneable:   constraintOrFalse(r.PcOpen.Constraints, func(c RunConstraints) bool { return c.Cloneable }),
			CanReorder:  constraintOrTrue(r.PcOpen.Constraints, func(c RunConstraints) bool { return c.Reorderable }),
		}
	case r.PcClose != nil:
		return &Span{
			SpanType:  kind,
			Type:      r.PcClose.Type,
			SubType:   r.PcClose.SubType,
			ID:        r.PcClose.ID,
			Data:      r.PcClose.Data,
			EquivText: r.PcClose.Equiv,
		}
	}
	return &Span{SpanType: kind}
}

func constraintOrTrue(c *RunConstraints, pick func(RunConstraints) bool) bool {
	if c == nil {
		return true
	}
	return pick(*c)
}

func constraintOrFalse(c *RunConstraints, pick func(RunConstraints) bool) bool {
	if c == nil {
		return false
	}
	return pick(*c)
}

// FlattenRuns returns the plain text form of a Run sequence.
// Placeholder runs contribute `{equiv}`, paired codes contribute
// their content (text between open/close), and plural/select
// constructs contribute their 'other' branch (or the first branch
// if 'other' is absent).
func FlattenRuns(runs []Run) string {
	var b []rune
	flattenRunsTo(&b, runs)
	return string(b)
}

// RenderRunsWithData writes the Run sequence as markup-preserving
// text: TextRun content verbatim, and inline-code runs re-emit their
// captured Data string (the original tag/token the reader extracted).
// Plural / Select runs recurse into the 'other' branch (or the first
// branch present). Sub runs emit their Ref string.
//
// This is the canonical rendering path for format writers that only
// need to splice opaque inline markup back into a flat string — HTML,
// XML, and markdown writers all use this helper.
func RenderRunsWithData(runs []Run) string {
	var b strings.Builder
	renderRunsDataTo(&b, runs)
	return b.String()
}

func renderRunsDataTo(buf *strings.Builder, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(r.Text.Text)
		case r.Ph != nil:
			buf.WriteString(r.Ph.Data)
		case r.PcOpen != nil:
			buf.WriteString(r.PcOpen.Data)
		case r.PcClose != nil:
			buf.WriteString(r.PcClose.Data)
		case r.Sub != nil:
			buf.WriteString(r.Sub.Ref)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				renderRunsDataTo(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				renderRunsDataTo(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				renderRunsDataTo(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				renderRunsDataTo(buf, form)
				break
			}
		}
	}
}

func flattenRunsTo(buf *[]rune, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			*buf = append(*buf, []rune(r.Text.Text)...)
		case r.Ph != nil:
			*buf = append(*buf, '{')
			*buf = append(*buf, []rune(r.Ph.Equiv)...)
			*buf = append(*buf, '}')
		case r.Sub != nil:
			*buf = append(*buf, '[')
			*buf = append(*buf, []rune(r.Sub.Equiv)...)
			*buf = append(*buf, ']')
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				flattenRunsTo(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				flattenRunsTo(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				flattenRunsTo(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				flattenRunsTo(buf, form)
				break
			}
		}
	}
}

// AsRuns renders a CodedText + Spans fragment into Runs. Callers in
// the migration path can use this without allocating a throwaway
// Fragment wrapper.
func AsRuns(codedText string, spans []*Span) []Run {
	return FragmentToRuns(&Fragment{CodedText: codedText, Spans: spans})
}

// AsCodedText flattens Runs into a PUA-marker coded string + Spans
// list, matching the legacy Fragment format. Intended as the
// hot-path helper that RFC 0001 Phase 2 calls out: tools that need
// O(1) substring ops on a flat string materialize this form on
// demand and discard it after use. The materialization is ephemeral;
// never persist the output.
func AsCodedText(runs []Run) (string, []*Span) {
	f := RunsToFragment(runs)
	return f.CodedText, f.Spans
}
