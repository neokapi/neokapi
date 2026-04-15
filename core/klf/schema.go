package klf

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SchemaVersion is the klf wire format version this package emits.
// Consumers MUST reject unknown major versions and SHOULD accept
// unknown minor versions of their major (forward-compat contract in
// RFC 0001 §Versioning).
const SchemaVersion = "1.0"

// Kind is the magic string on the root of a .klf JSON document.
const Kind = "kapi-localization-format"

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

// BlockType is the coarse classification of a Block.
type BlockType string

const (
	BlockTypeJSXElement   BlockType = "jsx:element"
	BlockTypeJSXAttribute BlockType = "jsx:attribute"
)

// PlaceholderKind discriminates how a placeholder was extracted.
type PlaceholderKind string

const (
	PlaceholderVariable PlaceholderKind = "variable"
	PlaceholderElement  PlaceholderKind = "element"
	PlaceholderNode     PlaceholderKind = "node"
	PlaceholderICUPivot PlaceholderKind = "icu-pivot"
)

// LocaleID is a BCP-47 locale tag.
type LocaleID = string

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

// PcOpenRun is the opening half of a paired code (an inline element
// that wraps content). The matching PcCloseRun uses the same ID
// within the same runs scope.
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
// mini-document in another format, the subfilter produces a separate
// Block and the outer Block contains a SubRun pointing at it by id.
type SubRun struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Equiv string `json:"equiv"`
}

// PluralRun is the inner part of a structured plural construct.
// The outer wrapper is a Run containing this via its Plural field.
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

// Run is the discriminated union of inline content primitives. Exactly
// one of the pointer fields is non-nil per Run. JSON encoding matches
// @neokapi/format: a Run is an object with exactly one of the keys
// `text`, `ph`, `pcOpen`, `pcClose`, `sub`, `plural`, or `select`.
type Run struct {
	Text    *TextRun        `json:"text,omitempty"`
	Ph      *PlaceholderRun `json:"ph,omitempty"`
	PcOpen  *PcOpenRun      `json:"pcOpen,omitempty"`
	PcClose *PcCloseRun     `json:"pcClose,omitempty"`
	Sub     *SubRun         `json:"sub,omitempty"`
	Plural  *PluralRun      `json:"plural,omitempty"`
	Select  *SelectRun      `json:"select,omitempty"`
}

// Kind returns a short string naming the discriminator of r.
// Returns the empty string for a zero Run (which is invalid).
func (r Run) Kind() string {
	switch {
	case r.Text != nil:
		return "text"
	case r.Ph != nil:
		return "ph"
	case r.PcOpen != nil:
		return "pcOpen"
	case r.PcClose != nil:
		return "pcClose"
	case r.Sub != nil:
		return "sub"
	case r.Plural != nil:
		return "plural"
	case r.Select != nil:
		return "select"
	}
	return ""
}

// ID returns the run's id for kinds that carry one (ph, pcOpen,
// pcClose, sub). Returns empty string for other kinds.
func (r Run) ID() string {
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
// JSON shape. Rejects objects with zero or multiple discriminators
// with a descriptive error; that's the same contract @neokapi/format
// validators enforce on the TS side.
func (r *Run) UnmarshalJSON(data []byte) error {
	// Keep the decode cheap by using RawMessage: we only touch the
	// fields that are actually present, avoiding spurious allocation.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("klf: decode run: %w", err)
	}
	*r = Run{}
	seen := 0
	for key, val := range raw {
		switch key {
		case "text":
			r.Text = &TextRun{}
			if err := json.Unmarshal(val, r.Text); err != nil {
				return fmt.Errorf("klf: decode text run: %w", err)
			}
			seen++
		case "ph":
			r.Ph = &PlaceholderRun{}
			if err := json.Unmarshal(val, r.Ph); err != nil {
				return fmt.Errorf("klf: decode ph run: %w", err)
			}
			seen++
		case "pcOpen":
			r.PcOpen = &PcOpenRun{}
			if err := json.Unmarshal(val, r.PcOpen); err != nil {
				return fmt.Errorf("klf: decode pcOpen run: %w", err)
			}
			seen++
		case "pcClose":
			r.PcClose = &PcCloseRun{}
			if err := json.Unmarshal(val, r.PcClose); err != nil {
				return fmt.Errorf("klf: decode pcClose run: %w", err)
			}
			seen++
		case "sub":
			r.Sub = &SubRun{}
			if err := json.Unmarshal(val, r.Sub); err != nil {
				return fmt.Errorf("klf: decode sub run: %w", err)
			}
			seen++
		case "plural":
			r.Plural = &PluralRun{}
			if err := json.Unmarshal(val, r.Plural); err != nil {
				return fmt.Errorf("klf: decode plural run: %w", err)
			}
			seen++
		case "select":
			r.Select = &SelectRun{}
			if err := json.Unmarshal(val, r.Select); err != nil {
				return fmt.Errorf("klf: decode select run: %w", err)
			}
			seen++
			// Unknown fields are tolerated per the forward-compat contract.
		}
	}
	if seen == 0 {
		return errors.New("klf: run has no discriminator")
	}
	if seen > 1 {
		return fmt.Errorf("klf: run has multiple discriminators (%d)", seen)
	}
	return nil
}

// MarshalJSON enforces that exactly one discriminator is set before
// emitting JSON. This catches mis-constructed Runs at serialization
// time, matching the @neokapi/format invariant.
func (r Run) MarshalJSON() ([]byte, error) {
	switch r.Kind() {
	case "":
		return nil, errors.New("klf: run has no discriminator")
	case "text":
		return json.Marshal(struct {
			Text *TextRun `json:"text"`
		}{r.Text})
	case "ph":
		return json.Marshal(struct {
			Ph *PlaceholderRun `json:"ph"`
		}{r.Ph})
	case "pcOpen":
		return json.Marshal(struct {
			PcOpen *PcOpenRun `json:"pcOpen"`
		}{r.PcOpen})
	case "pcClose":
		return json.Marshal(struct {
			PcClose *PcCloseRun `json:"pcClose"`
		}{r.PcClose})
	case "sub":
		return json.Marshal(struct {
			Sub *SubRun `json:"sub"`
		}{r.Sub})
	case "plural":
		return json.Marshal(struct {
			Plural *PluralRun `json:"plural"`
		}{r.Plural})
	case "select":
		return json.Marshal(struct {
			Select *SelectRun `json:"select"`
		}{r.Select})
	}
	return nil, fmt.Errorf("klf: run has unknown discriminator %q", r.Kind())
}

// Placeholder is metadata about a variable or element token
// referenced by the runs of a Block. One entry per unique
// placeholder name; drives validation and provides tools with
// metadata (jsType, sourceExpr, optional, icu-pivot flag) that
// doesn't fit on a Run.
type Placeholder struct {
	Name       string          `json:"name"`
	Kind       PlaceholderKind `json:"kind"`
	JSType     string          `json:"jsType,omitempty"`
	SourceExpr string          `json:"sourceExpr"`
	Optional   bool            `json:"optional,omitempty"`
}

// BlockProperties is translator-facing context about where a Block
// came from: file, component, element, etc.
type BlockProperties struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Component string `json:"component"`
	JSXPath   string `json:"jsxPath"`
	Element   string `json:"element"`
	LocNote   string `json:"locNote,omitempty"`
}

// BlockPreviewHints are optional hints for Level-2 / Level-3 renders.
type BlockPreviewHints struct {
	StoryID      string                 `json:"storyId,omitempty"`
	SnapshotPath string                 `json:"snapshotPath,omitempty"`
	SampleValues map[string]interface{} `json:"sampleValues,omitempty"`
}

// Block is the unit of translation tracking.
type Block struct {
	ID           string             `json:"id"`
	Hash         string             `json:"hash"`
	Translatable bool               `json:"translatable"`
	Type         BlockType          `json:"type"`
	Source       []Run              `json:"source"`
	Targets      map[LocaleID][]Run `json:"targets,omitempty"`
	Placeholders []Placeholder      `json:"placeholders"`
	Properties   BlockProperties    `json:"properties"`
	Preview      *BlockPreviewHints `json:"preview,omitempty"`
}

// DocumentType discriminates the source format of a document.
// Currently always "jsx" for @neokapi/react output; future extractors
// can introduce new values.
type DocumentType string

const (
	DocumentTypeJSX DocumentType = "jsx"
)

// Skeleton is the reference to an opaque skeleton payload that the
// owning extractor will use to reconstruct the source file. Inside a
// standalone .klf the inline field carries the skeleton bytes; inside
// a .klz the ref field points at a part path relative to the archive
// root.
type Skeleton struct {
	Ref    string `json:"ref,omitempty"`
	Inline string `json:"inline,omitempty"`
}

// Document is one source file's worth of extracted content.
type Document struct {
	ID           string       `json:"id"`
	DocumentType DocumentType `json:"documentType"`
	Path         string       `json:"path"`
	SourceHash   string       `json:"sourceHash,omitempty"`
	Skeleton     *Skeleton    `json:"skeleton,omitempty"`
	Blocks       []Block      `json:"blocks"`
}

// GeneratorInfo identifies the extractor that produced a .klf.
type GeneratorInfo struct {
	ID           string   `json:"id"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// ProjectInfo identifies the project a .klf belongs to.
type ProjectInfo struct {
	ID           string   `json:"id"`
	SourceLocale LocaleID `json:"sourceLocale"`
}

// Vocabulary lists vocabulary files this .klf depends on, in extends
// order (earlier entries loaded first).
type Vocabulary struct {
	Extends []string `json:"extends,omitempty"`
}

// File is the top-level shape of a .klf JSON document.
type File struct {
	SchemaVersion string        `json:"schemaVersion"`
	Kind          string        `json:"kind"`
	Created       string        `json:"created,omitempty"`
	Generator     GeneratorInfo `json:"generator"`
	Project       ProjectInfo   `json:"project"`
	Vocabulary    *Vocabulary   `json:"vocabulary,omitempty"`
	Documents     []Document    `json:"documents"`
}
