package klf

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// AnnotationFile is the in-memory form of an `annotations/*.klfl`
// file: one header record plus zero or more annotation records.
type AnnotationFile struct {
	Header      AnnotationFileHeader
	Annotations []Annotation
}

// AnnotationFileHeader is the first line of a .klfl file: identifies
// the annotation type, its producer, and the archive state the
// annotations were produced against.
type AnnotationFileHeader struct {
	Type              string             `json:"type"`
	AnnotationType    string             `json:"annotationType"`
	AnnotationVersion string             `json:"annotationVersion"`
	Producer          AnnotationProducer `json:"producer"`
	Created           string             `json:"created"`
	TargetArchive     string             `json:"targetArchive"`
}

// AnnotationProducer identifies the tool that wrote an annotation
// file.
type AnnotationProducer struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// Annotation is one record in a .klfl file (every line after the
// header).
type Annotation struct {
	Type   string           `json:"type"`
	ID     string           `json:"id"`
	Anchor AnnotationAnchor `json:"anchor"`
	// Data is the producer-specific payload. The framework imposes
	// no schema here; consumers that understand the annotation type
	// negotiate via AnnotationType.
	Data json.RawMessage `json:"data,omitempty"`
}

// AnchorKind discriminates AnnotationAnchor shapes. Four values:
// block, run, range, form.
type AnchorKind string

const (
	AnchorBlock AnchorKind = "block"
	AnchorRun   AnchorKind = "run"
	AnchorRange AnchorKind = "range"
	AnchorForm  AnchorKind = "form"
)

// AnnotationAnchor is a flattened shape covering all four anchor
// kinds. The `Kind` field discriminates which other fields are
// meaningful:
//
//   - block: Block
//   - run:   Block, Path, RunID
//   - range: Block, Path, Offset, Length
//   - form:  Block, Path, Key
type AnnotationAnchor struct {
	Kind   AnchorKind `json:"kind"`
	Block  string     `json:"block"`
	Path   RunPath    `json:"path,omitempty"`
	RunID  string     `json:"runId,omitempty"`
	Offset int        `json:"offset,omitempty"`
	Length int        `json:"length,omitempty"`
	Key    string     `json:"key,omitempty"`
}

// RunPath is a path through a block's nested run structure. Empty
// path refers to the block's top-level source runs themselves.
type RunPath []RunPathStep

// RunPathStep is one hop in a RunPath. Exactly one of Index,
// PluralForm, or SelectValue is meaningful per step — discriminated
// by the Kind field. The JSON form is a bare number OR an object
// `{"plural":"<form>"}` OR `{"select":"<value>"}`.
type RunPathStep struct {
	Kind  RunPathStepKind
	Index int
	// PluralForm is populated when Kind == StepPlural.
	PluralForm PluralForm
	// SelectValue is populated when Kind == StepSelect.
	SelectValue string
}

// RunPathStepKind discriminates RunPathStep cases.
type RunPathStepKind int

const (
	// StepIndex is a numeric index into a Run[] sequence.
	StepIndex RunPathStepKind = iota
	// StepPlural is a step into a plural run's form.
	StepPlural
	// StepSelect is a step into a select run's case.
	StepSelect
)

// MarshalJSON emits the RunPathStep's discriminated shape: a bare
// number for StepIndex, an object for StepPlural / StepSelect.
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
	return nil, fmt.Errorf("klf: run path step has unknown kind %d", s.Kind)
}

// UnmarshalJSON decodes the discriminated RunPathStep shape.
func (s *RunPathStep) UnmarshalJSON(data []byte) error {
	// Probe: is the token a number?
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')) {
		var n int
		if err := json.Unmarshal(data, &n); err != nil {
			return fmt.Errorf("klf: decode run path index step: %w", err)
		}
		*s = RunPathStep{Kind: StepIndex, Index: n}
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("klf: decode run path step: %w", err)
	}
	if raw, ok := obj["plural"]; ok {
		var form PluralForm
		if err := json.Unmarshal(raw, &form); err != nil {
			return fmt.Errorf("klf: decode run path plural step: %w", err)
		}
		*s = RunPathStep{Kind: StepPlural, PluralForm: form}
		return nil
	}
	if raw, ok := obj["select"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("klf: decode run path select step: %w", err)
		}
		*s = RunPathStep{Kind: StepSelect, SelectValue: v}
		return nil
	}
	return errors.New("klf: run path step has no recognized discriminator")
}

// ───────── anchor resolution ─────────

// AnchorResolveReason is a machine-readable reason an anchor didn't
// resolve. Mirrors the six reasons in
// packages/kapi-format/src/annotation.ts.
type AnchorResolveReason string

const (
	ReasonBlockNotFound    AnchorResolveReason = "block-not-found"
	ReasonPathOutOfBounds  AnchorResolveReason = "path-out-of-bounds"
	ReasonPathWrongKind    AnchorResolveReason = "path-wrong-kind"
	ReasonRunIDMismatch    AnchorResolveReason = "run-id-mismatch"
	ReasonRangeOutOfBounds AnchorResolveReason = "range-out-of-bounds"
	ReasonFormNotFound     AnchorResolveReason = "form-not-found"
)

// AnchorResolution is the result of ResolveAnchor. On success,
// exactly one of the *Target fields is populated according to
// Kind; on failure, Err is the machine-readable failure reason.
type AnchorResolution struct {
	OK   bool
	Kind AnchorKind
	Err  AnchorResolveReason

	// Populated on AnchorBlock success.
	BlockTarget *Block
	// Populated on AnchorRun success.
	RunTarget *Run
	// Populated on AnchorRange success.
	RangeText   string
	RangeOffset int
	RangeLength int
	// Populated on AnchorForm success.
	FormRuns []Run
}

// ResolveAnchor resolves an annotation anchor against a Block and
// returns either the resolved entity or a machine-readable failure
// reason. Mirrors resolveAnchor in
// packages/kapi-format/src/annotation.ts.
func ResolveAnchor(block *Block, anchor AnnotationAnchor) AnchorResolution {
	if block == nil || anchor.Block != block.ID {
		return AnchorResolution{OK: false, Err: ReasonBlockNotFound}
	}

	if anchor.Kind == AnchorBlock {
		return AnchorResolution{OK: true, Kind: AnchorBlock, BlockTarget: block}
	}

	walked, ok := walkPath(block.Source, anchor.Path)
	if !ok {
		return AnchorResolution{OK: false, Err: ReasonPathOutOfBounds}
	}

	switch anchor.Kind {
	case AnchorRun:
		if walked == nil {
			return AnchorResolution{OK: false, Err: ReasonPathOutOfBounds}
		}
		id := walked.RunID()
		if id == "" {
			return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
		}
		if id != anchor.RunID {
			return AnchorResolution{OK: false, Err: ReasonRunIDMismatch}
		}
		return AnchorResolution{OK: true, Kind: AnchorRun, RunTarget: walked}

	case AnchorRange:
		if walked == nil || walked.Text == nil {
			return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
		}
		utf16Len := utf16Length(walked.Text.Text)
		if anchor.Offset < 0 || anchor.Offset+anchor.Length > utf16Len {
			return AnchorResolution{OK: false, Err: ReasonRangeOutOfBounds}
		}
		return AnchorResolution{
			OK: true, Kind: AnchorRange,
			RangeText:   walked.Text.Text,
			RangeOffset: anchor.Offset,
			RangeLength: anchor.Length,
		}

	case AnchorForm:
		if walked == nil {
			return AnchorResolution{OK: false, Err: ReasonPathOutOfBounds}
		}
		if walked.Plural != nil {
			form, has := walked.Plural.Forms[PluralForm(anchor.Key)]
			if !has {
				return AnchorResolution{OK: false, Err: ReasonFormNotFound}
			}
			return AnchorResolution{OK: true, Kind: AnchorForm, FormRuns: form}
		}
		if walked.Select != nil {
			caseRuns, has := walked.Select.Cases[anchor.Key]
			if !has {
				return AnchorResolution{OK: false, Err: ReasonFormNotFound}
			}
			return AnchorResolution{OK: true, Kind: AnchorForm, FormRuns: caseRuns}
		}
		return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
	}

	return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
}

// walkPath walks `path` through `top`, returning the run the path
// lands on (or nil for an empty path / a path that ends mid-sequence
// after a plural/select descent). The second return is false on any
// out-of-bounds or wrong-kind step.
func walkPath(top []Run, path RunPath) (*Run, bool) {
	if len(path) == 0 {
		return nil, true
	}
	current := top
	var currentRun *Run
	for _, step := range path {
		switch step.Kind {
		case StepIndex:
			if step.Index < 0 || step.Index >= len(current) {
				return nil, false
			}
			r := current[step.Index]
			currentRun = &r
		case StepPlural:
			if currentRun == nil || currentRun.Plural == nil {
				return nil, false
			}
			form, has := currentRun.Plural.Forms[step.PluralForm]
			if !has {
				return nil, false
			}
			current = form
			currentRun = nil
		case StepSelect:
			if currentRun == nil || currentRun.Select == nil {
				return nil, false
			}
			caseRuns, has := currentRun.Select.Cases[step.SelectValue]
			if !has {
				return nil, false
			}
			current = caseRuns
			currentRun = nil
		}
	}
	return currentRun, true
}

// utf16Length returns the number of UTF-16 code units in s. Mirrors
// the TypeScript side, which measures offsets in UTF-16 code units.
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// AnnotationValidationError mirrors
// packages/kapi-format/src/annotation.ts's AnnotationValidationError.
type AnnotationValidationError struct {
	AnnotationID string
	BlockID      string
	Reason       AnchorResolveReason
	Message      string
}

// ValidateAnchor checks an annotation's anchor against a Block and
// returns a validation error if it doesn't resolve. Suitable for
// orphan-detection validators that process annotation files after
// blocks may have been re-extracted.
func ValidateAnchor(block *Block, ann Annotation) *AnnotationValidationError {
	res := ResolveAnchor(block, ann.Anchor)
	if res.OK {
		return nil
	}
	return &AnnotationValidationError{
		AnnotationID: ann.ID,
		BlockID:      blockIDFromAnchor(ann.Anchor, block),
		Reason:       res.Err,
		Message:      messageFor(res.Err, ann),
	}
}

func blockIDFromAnchor(a AnnotationAnchor, fallback *Block) string {
	if fallback != nil {
		return fallback.ID
	}
	return a.Block
}

func messageFor(reason AnchorResolveReason, ann Annotation) string {
	switch reason {
	case ReasonBlockNotFound:
		return fmt.Sprintf("annotation %q targets block %q which does not match", ann.ID, ann.Anchor.Block)
	case ReasonPathOutOfBounds:
		return fmt.Sprintf("annotation %q path is out of bounds in block %q", ann.ID, ann.Anchor.Block)
	case ReasonPathWrongKind:
		return fmt.Sprintf("annotation %q path lands on a run of the wrong kind for its anchor", ann.ID)
	case ReasonRunIDMismatch:
		return fmt.Sprintf("annotation %q resolves to a run whose id does not match the recorded id (possible orphan)", ann.ID)
	case ReasonRangeOutOfBounds:
		return fmt.Sprintf("annotation %q character range exceeds the target text run", ann.ID)
	case ReasonFormNotFound:
		return fmt.Sprintf("annotation %q targets a plural form or select case that does not exist on the block", ann.ID)
	}
	return fmt.Sprintf("annotation %q: %s", ann.ID, reason)
}

// ───────── annotation file I/O (.klfl) ─────────

// DecodeAnnotationFile parses a JSON-Lines annotation overlay from r.
// The first non-empty line must be a header record; subsequent
// non-empty lines are annotation records.
func DecodeAnnotationFile(r io.Reader) (*AnnotationFile, error) {
	br := bufio.NewReader(r)
	var out AnnotationFile

	// Read header.
	header, err := readJSONLine(br)
	if err != nil {
		return nil, fmt.Errorf("klf: read annotation header: %w", err)
	}
	if header == nil {
		return nil, errors.New("klf: empty annotation file")
	}
	if err := json.Unmarshal(header, &out.Header); err != nil {
		return nil, fmt.Errorf("klf: decode annotation header: %w", err)
	}
	if out.Header.Type != "header" {
		return nil, fmt.Errorf("klf: annotation header has unexpected type %q", out.Header.Type)
	}

	// Read records.
	for {
		line, err := readJSONLine(br)
		if err != nil {
			return nil, fmt.Errorf("klf: read annotation record: %w", err)
		}
		if line == nil {
			break
		}
		var ann Annotation
		if err := json.Unmarshal(line, &ann); err != nil {
			return nil, fmt.Errorf("klf: decode annotation record: %w", err)
		}
		if ann.Type != "annotation" {
			return nil, fmt.Errorf("klf: annotation record has unexpected type %q", ann.Type)
		}
		out.Annotations = append(out.Annotations, ann)
	}
	return &out, nil
}

// readJSONLine reads one line from br, skipping empty lines, and
// returns nil at EOF.
func readJSONLine(br *bufio.Reader) ([]byte, error) {
	for {
		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err == io.EOF {
			return nil, nil
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "" {
			if err == io.EOF {
				return nil, nil
			}
			continue
		}
		return []byte(trimmed), nil
	}
}

// EncodeAnnotationFile writes a JSON-Lines annotation overlay to w.
// Each line is compact JSON (no indentation) terminated by LF; this
// keeps the file grep-friendly and diff-friendly as required by RFC
// 0001.
func EncodeAnnotationFile(w io.Writer, f *AnnotationFile) error {
	if f == nil {
		return errors.New("klf: encode nil annotation file")
	}
	// Header.
	headerLine := AnnotationFileHeader{
		Type:              "header",
		AnnotationType:    f.Header.AnnotationType,
		AnnotationVersion: f.Header.AnnotationVersion,
		Producer:          f.Header.Producer,
		Created:           f.Header.Created,
		TargetArchive:     f.Header.TargetArchive,
	}
	if err := writeJSONLine(w, headerLine); err != nil {
		return fmt.Errorf("klf: write annotation header: %w", err)
	}
	for i := range f.Annotations {
		ann := f.Annotations[i]
		if ann.Type == "" {
			ann.Type = "annotation"
		}
		if err := writeJSONLine(w, ann); err != nil {
			return fmt.Errorf("klf: write annotation record: %w", err)
		}
	}
	return nil
}

func writeJSONLine(w io.Writer, v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(buf); err != nil {
		return err
	}
	_, err = w.Write([]byte{'\n'})
	return err
}
