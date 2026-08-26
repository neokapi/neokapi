package kbf

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/neokapi/neokapi/core/model"
	"io"
	"strings"
)

// AnnotationFile is the in-memory form of an `annotations/*.overlays.jsonl`
// file: one header record plus zero or more annotation records.
type AnnotationFile struct {
	Header      AnnotationFileHeader
	Annotations []Annotation
}

// AnnotationFileHeader is the first line of a .overlays.jsonl file: identifies
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

// Annotation is one record in a .overlays.jsonl file (every line after the
// header).
type Annotation struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	// Block is the block the annotation is about; Anchor says where inside it.
	Block  string `json:"block"`
	Anchor Anchor `json:"anchor"`
	// Data is the producer-specific payload. The framework imposes
	// no schema here; consumers that understand the annotation type
	// negotiate via AnnotationType.
	Data json.RawMessage `json:"data,omitempty"`
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
	RangeRuns []Run
	RangeText string
	// Populated on AnchorForm success.
	FormRuns []Run
}

// ResolveAnchor resolves an annotation's anchor against the block it names,
// returning either the resolved entity or a machine-readable reason it did not.
// Mirrors resolveAnchor in packages/kapi-format/src/annotation.ts.
func ResolveAnchor(block *Block, anchor Anchor) AnchorResolution {
	if block == nil {
		return AnchorResolution{OK: false, Err: ReasonBlockNotFound}
	}

	if anchor.Kind == AnchorBlock {
		return AnchorResolution{OK: true, Kind: AnchorBlock, BlockTarget: block}
	}

	seq, landed, ok := walkPath(block.Source, anchor.Path)
	if !ok {
		return AnchorResolution{OK: false, Err: ReasonPathOutOfBounds}
	}

	switch anchor.Kind {
	case AnchorRun:
		run := landed
		if run == nil {
			for i := range seq {
				if seq[i].RunID() == anchor.RunID && anchor.RunID != "" {
					run = &seq[i]
					break
				}
			}
		}
		if run == nil {
			// The path resolved; nothing in the sequence carries that id.
			return AnchorResolution{OK: false, Err: ReasonRunIDMismatch}
		}
		id := run.RunID()
		if id == "" {
			return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
		}
		if id != anchor.RunID {
			return AnchorResolution{OK: false, Err: ReasonRunIDMismatch}
		}
		return AnchorResolution{OK: true, Kind: AnchorRun, RunTarget: run}

	case AnchorRange:
		if !anchor.InBounds(seq) {
			return AnchorResolution{OK: false, Err: ReasonRangeOutOfBounds}
		}
		runs := anchor.ExtractRuns(seq)
		return AnchorResolution{
			OK: true, Kind: AnchorRange,
			RangeRuns: runs,
			RangeText: model.RunsText(runs),
		}

	case AnchorForm:
		if landed == nil {
			return AnchorResolution{OK: false, Err: ReasonPathOutOfBounds}
		}
		if landed.Plural != nil {
			form, has := landed.Plural.Forms[PluralForm(anchor.Key)]
			if !has {
				return AnchorResolution{OK: false, Err: ReasonFormNotFound}
			}
			return AnchorResolution{OK: true, Kind: AnchorForm, FormRuns: form}
		}
		if landed.Select != nil {
			caseRuns, has := landed.Select.Cases[anchor.Key]
			if !has {
				return AnchorResolution{OK: false, Err: ReasonFormNotFound}
			}
			return AnchorResolution{OK: true, Kind: AnchorForm, FormRuns: caseRuns}
		}
		return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
	}

	return AnchorResolution{OK: false, Err: ReasonPathWrongKind}
}

// walkPath walks `path` through `top` and returns the run sequence it
// addresses — `top` itself for an empty path, otherwise the branch a plural or
// select step descended into — together with the run an index step last landed
// on, which is what a form anchor is about. The last return is false on any
// out-of-bounds or wrong-kind step.
func walkPath(top []Run, path RunPath) ([]Run, *Run, bool) {
	current := top
	var currentRun *Run
	for _, step := range path {
		switch step.Kind {
		case StepIndex:
			if step.Index < 0 || step.Index >= len(current) {
				return nil, nil, false
			}
			r := current[step.Index]
			currentRun = &r
		case StepPlural:
			if currentRun == nil || currentRun.Plural == nil {
				return nil, nil, false
			}
			form, has := currentRun.Plural.Forms[step.PluralForm]
			if !has {
				return nil, nil, false
			}
			current = form
			currentRun = nil
		case StepSelect:
			if currentRun == nil || currentRun.Select == nil {
				return nil, nil, false
			}
			caseRuns, has := currentRun.Select.Cases[step.SelectValue]
			if !has {
				return nil, nil, false
			}
			current = caseRuns
			currentRun = nil
		}
	}
	return current, currentRun, true
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
	// The record names the block it is about, so validating it against one that
	// is not that block is a mismatch rather than a resolution failure.
	if block == nil || ann.Block != block.ID {
		return &AnnotationValidationError{
			AnnotationID: ann.ID,
			BlockID:      ann.Block,
			Reason:       ReasonBlockNotFound,
			Message:      messageFor(ReasonBlockNotFound, ann),
		}
	}
	res := ResolveAnchor(block, ann.Anchor)
	if res.OK {
		return nil
	}
	return &AnnotationValidationError{
		AnnotationID: ann.ID,
		BlockID:      ann.Block,
		Reason:       res.Err,
		Message:      messageFor(res.Err, ann),
	}
}

func messageFor(reason AnchorResolveReason, ann Annotation) string {
	switch reason {
	case ReasonBlockNotFound:
		return fmt.Sprintf("annotation %q targets block %q which does not match", ann.ID, ann.Block)
	case ReasonPathOutOfBounds:
		return fmt.Sprintf("annotation %q path is out of bounds in block %q", ann.ID, ann.Block)
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

// ───────── annotation file I/O (.overlays.jsonl) ─────────

// DecodeAnnotationFile parses a JSON-Lines annotation overlay from r.
// The first non-empty line must be a header record; subsequent
// non-empty lines are annotation records.
func DecodeAnnotationFile(r io.Reader) (*AnnotationFile, error) {
	br := bufio.NewReader(r)
	var out AnnotationFile

	// Read header.
	header, err := readJSONLine(br)
	if err != nil {
		return nil, fmt.Errorf("kbf: read annotation header: %w", err)
	}
	if header == nil {
		return nil, errors.New("kbf: empty annotation file")
	}
	if err := json.Unmarshal(header, &out.Header); err != nil {
		return nil, fmt.Errorf("kbf: decode annotation header: %w", err)
	}
	if out.Header.Type != "header" {
		return nil, fmt.Errorf("kbf: annotation header has unexpected type %q", out.Header.Type)
	}

	// Read records.
	for {
		line, err := readJSONLine(br)
		if err != nil {
			return nil, fmt.Errorf("kbf: read annotation record: %w", err)
		}
		if line == nil {
			break
		}
		var ann Annotation
		if err := json.Unmarshal(line, &ann); err != nil {
			return nil, fmt.Errorf("kbf: decode annotation record: %w", err)
		}
		if ann.Type != "annotation" {
			return nil, fmt.Errorf("kbf: annotation record has unexpected type %q", ann.Type)
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
		return errors.New("kbf: encode nil annotation file")
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
		return fmt.Errorf("kbf: write annotation header: %w", err)
	}
	for i := range f.Annotations {
		ann := f.Annotations[i]
		if ann.Type == "" {
			ann.Type = "annotation"
		}
		if err := writeJSONLine(w, ann); err != nil {
			return fmt.Errorf("kbf: write annotation record: %w", err)
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
