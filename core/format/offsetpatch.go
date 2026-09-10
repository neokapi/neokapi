package format

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// ErrPatchStale means an offset plan's original source is no longer available.
var ErrPatchStale = errors.New("patch source changed; inspect again before editing")

// OffsetPatch replaces [Start, End) in the original source bytes. Entry names a
// ZIP member for packaged documents and is empty for an ordinary file. Offsets
// never refer to the result of another patch. Before guards the exact range.
type OffsetPatch struct {
	Entry       string `json:"entry,omitempty"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Before      string `json:"before"`
	Replacement string `json:"replacement"`
}

// OffsetPatchPlan is an immutable-by-contract edit description. Writers validate
// every patch against Snapshot before producing output. Callers retain the
// original document and do not mutate its parsed content model.
type OffsetPatchPlan struct {
	Format   string            `json:"format"`
	Snapshot string            `json:"snapshot"`
	Patches  []OffsetPatch     `json:"patches"`
	Range    *model.BlockRange `json:"range,omitempty"`
}

// SourceSnapshot binds a plan to all bytes of a file or package.
func SourceSnapshot(source []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(source))
}

// WriteOffsetPatches applies a plan against the original source. Validation and
// rendering finish before the destination receives any bytes. The caller owns
// atomic file replacement and must recheck the on-disk source before committing.
func WriteOffsetPatches(dst io.Writer, source []byte, plan OffsetPatchPlan) error {
	data, err := ApplyOffsetPatches(source, plan)
	if err != nil {
		return err
	}
	_, err = dst.Write(data)
	return err
}

// ApplyOffsetPatches is the buffered writer used by previews and file writes.
// It never changes source or the plan's patches.
func ApplyOffsetPatches(source []byte, plan OffsetPatchPlan) ([]byte, error) {
	if plan.Snapshot == "" || plan.Snapshot != SourceSnapshot(source) {
		return nil, ErrPatchStale
	}
	if len(plan.Patches) == 0 {
		return nil, errors.New("offset plan has no patches")
	}
	switch plan.Format {
	case "markdown", "html":
		for _, patch := range plan.Patches {
			if patch.Entry != "" {
				return nil, errors.New("flat-file patch cannot name a ZIP entry")
			}
		}
		return spliceOffsets(source, plan.Patches)
	case "docx":
		return patchZIP(source, plan.Patches)
	default:
		return nil, fmt.Errorf("unsupported offset-plan format %q", plan.Format)
	}
}

func spliceOffsets(source []byte, patches []OffsetPatch) ([]byte, error) {
	ordered := slices.Clone(patches)
	slices.SortFunc(ordered, func(a, b OffsetPatch) int {
		if a.Start < b.Start {
			return -1
		}
		if a.Start > b.Start {
			return 1
		}
		return 0
	})
	previousEnd := 0
	previousStart := -1
	for _, patch := range ordered {
		invalidRange := patch.Start < previousEnd || patch.End < patch.Start || patch.End > len(source)
		if invalidRange || patch.Start == previousStart {
			return nil, errors.New("offset patches overlap or address an invalid source range")
		}
		if string(source[patch.Start:patch.End]) != patch.Before {
			return nil, fmt.Errorf("offset patch at %d: %w", patch.Start, ErrPatchStale)
		}
		previousStart, previousEnd = patch.Start, patch.End
	}
	var out bytes.Buffer
	cursor := 0
	for _, patch := range ordered {
		out.Write(source[cursor:patch.Start])
		out.WriteString(patch.Replacement)
		cursor = patch.End
	}
	out.Write(source[cursor:])
	return out.Bytes(), nil
}

func patchZIP(source []byte, patches []OffsetPatch) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		return nil, err
	}
	if err := safeio.DefaultZipLimits.CheckReader(reader); err != nil {
		return nil, err
	}
	if err := rejectSignedPackage(reader); err != nil {
		return nil, err
	}
	byEntry := map[string][]OffsetPatch{}
	for _, patch := range patches {
		// The section writer supports the Word main document only. Changes
		// requiring relationships, styles or new assets need another planner.
		if patch.Entry != "word/document.xml" {
			return nil, fmt.Errorf("unsupported DOCX patch entry %q", patch.Entry)
		}
		byEntry[patch.Entry] = append(byEntry[patch.Entry], patch)
	}
	modified := map[string][]byte{}
	seen := map[string]bool{}
	for _, file := range reader.File {
		if seen[file.Name] {
			return nil, fmt.Errorf("duplicate ZIP entry %q", file.Name)
		}
		seen[file.Name] = true
		entryPatches := byEntry[file.Name]
		if len(entryPatches) == 0 {
			continue
		}
		const maxEntrySize = 64 << 20
		if file.UncompressedSize64 > maxEntrySize {
			return nil, errors.New("DOCX document XML exceeds the 64 MiB section-edit limit")
		}
		r, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(r, maxEntrySize+1))
		closeErr := r.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > maxEntrySize {
			return nil, errors.New("DOCX document XML exceeds the 64 MiB section-edit limit")
		}
		modified[file.Name], err = spliceOffsets(data, entryPatches)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, err)
		}
	}
	for entry := range byEntry {
		if !seen[entry] {
			return nil, fmt.Errorf("missing ZIP patch entry %q", entry)
		}
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	if err := writer.SetComment(reader.Comment); err != nil {
		return nil, err
	}
	for _, file := range reader.File {
		data, changed := modified[file.Name]
		if !changed {
			if err := writer.Copy(file); err != nil {
				return nil, err
			}
			continue
		}
		header := file.FileHeader
		w, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Package signatures cover part contents. A source edit cannot retain a valid
// signature without the signer's key; signed packages need a separate workflow.
func rejectSignedPackage(reader *zip.Reader) error {
	for _, file := range reader.File {
		if strings.HasPrefix(strings.ToLower(file.Name), "_xmlsignatures/") {
			return errors.New("section patches do not support signed DOCX packages")
		}
		if !strings.HasSuffix(file.Name, ".rels") {
			continue
		}
		data, err := safeio.DefaultZipLimits.ReadEntry(file)
		if err != nil {
			return err
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("package relationships: %w", err)
			}
			element, ok := token.(xml.StartElement)
			if !ok || element.Name.Local != "Relationship" {
				continue
			}
			for _, attr := range element.Attr {
				if attr.Name.Local == "Type" && strings.Contains(attr.Value, "/digital-signature/") {
					return errors.New("section patches do not support signed DOCX packages")
				}
			}
		}
	}
	return nil
}
