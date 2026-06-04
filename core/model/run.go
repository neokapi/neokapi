package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// This file defines the Run-based content model from RFC 0001. A Run
// is one element in a Block's flat inline content sequence:
// discriminated union keyed by the present field.

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

// BlockContentType classifies a Block per RFC 0001. Kept distinct
// from the free-form Block.Type string so the RFC enum values and the
// format-specific type tag don't clash on one field.
type BlockContentType string

const (
	BlockContentJSXElement   BlockContentType = "jsx:element"
	BlockContentJSXAttribute BlockContentType = "jsx:attribute"
	// BlockContentJST is an explicit t() call from a JS/TS context
	// (e.g. label strings stored in data arrays). The extracted
	// source is the literal first argument; the block is hashed
	// with the "t" channel so it doesn't collide with JSX blocks
	// carrying the same text.
	BlockContentJST BlockContentType = "js:t"
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
			// Per Framework AD-002, text runs are a bare string: `{"text":"..."}`.
			r.Text = &TextRun{}
			if err := json.Unmarshal(val, &r.Text.Text); err != nil {
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
//
// HTML escaping is disabled: a run's `data`/`text` routinely holds
// source markup like `<span>` or an `&&` expression, and the KLF wire
// form is "no HTML escaping" (see core/klf.Marshal). The package-level
// json.Marshal would escape `<`, `>`, and `&` into `<` etc.;
// encoding through marshalRunNoEscapeHTML keeps the literal bytes so the
// Go output matches the TypeScript mirror (@neokapi/kapi-format) and the
// content hash stays implementation-independent.
func (r Run) MarshalJSON() ([]byte, error) {
	switch r.Kind() {
	case "":
		return nil, errors.New("model: run has no discriminator")
	case RunKindText:
		// Per Framework AD-002 §Block contents: text runs serialize flat —
		// `{"text":"literal"}` — not as a nested object. Every
		// other run kind nests its struct under its discriminator
		// key; text is the exception the spec calls out explicitly.
		return marshalRunNoEscapeHTML(struct {
			Text string `json:"text"`
		}{r.Text.Text})
	case RunKindPh:
		return marshalRunNoEscapeHTML(struct {
			Ph *PlaceholderRun `json:"ph"`
		}{r.Ph})
	case RunKindPcOpen:
		return marshalRunNoEscapeHTML(struct {
			PcOpen *PcOpenRun `json:"pcOpen"`
		}{r.PcOpen})
	case RunKindPcClose:
		return marshalRunNoEscapeHTML(struct {
			PcClose *PcCloseRun `json:"pcClose"`
		}{r.PcClose})
	case RunKindSub:
		return marshalRunNoEscapeHTML(struct {
			Sub *SubRun `json:"sub"`
		}{r.Sub})
	case RunKindPlural:
		return marshalRunNoEscapeHTML(struct {
			Plural *PluralRun `json:"plural"`
		}{r.Plural})
	case RunKindSelect:
		return marshalRunNoEscapeHTML(struct {
			Select *SelectRun `json:"select"`
		}{r.Select})
	}
	return nil, fmt.Errorf("model: run has unknown discriminator %q", r.Kind())
}

// marshalRunNoEscapeHTML JSON-encodes v with HTML escaping disabled,
// stripping the trailing newline json.Encoder appends. Nested Runs (in
// plural forms / select cases) recurse back through Run.MarshalJSON, so
// the whole tree stays unescaped and consistent.
func marshalRunNoEscapeHTML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
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

// RunsText returns the plain-text flattening of a Run sequence: TextRun
// content verbatim, inline-code runs (Ph / PcOpen / PcClose / Sub) contribute
// nothing, plural / select take the 'other' branch (or the first form if
// 'other' is absent). This is the text-only variant of FlattenRuns, which
// renders {equiv} for placeholders.
func RunsText(runs []Run) string {
	var buf []rune
	runsTextTo(&buf, runs)
	return string(buf)
}

func runsTextTo(buf *[]rune, runs []Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			*buf = append(*buf, []rune(r.Text.Text)...)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				runsTextTo(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				runsTextTo(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				runsTextTo(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				runsTextTo(buf, form)
				break
			}
		}
	}
}
