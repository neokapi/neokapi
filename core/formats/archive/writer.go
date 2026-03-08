package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for ZIP archive files.
type Writer struct {
	format.BaseFormatWriter
	originalContent []byte
}

// NewWriter creates a new archive writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "archive",
		},
	}
}

// SetOriginalContent provides the original archive bytes for roundtrip fidelity.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write consumes Parts from a channel and writes a reconstructed ZIP archive.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all parts to build the output
	var allParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeArchive(allParts)
			}
			allParts = append(allParts, part)
		}
	}
}

func (w *Writer) writeArchive(parts []*model.Part) error {
	// Build a map of entry name -> translated lines
	entryBlocks := make(map[string][]string)
	for _, part := range parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}
		entry := block.Properties["entry"]
		if entry == "" {
			continue
		}

		text := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			text = block.TargetText(w.Locale)
		}
		entryBlocks[entry] = append(entryBlocks[entry], text)
	}

	// If we have original content, do a roundtrip preserving all entries
	if w.originalContent != nil {
		return w.writeRoundtrip(entryBlocks)
	}

	// Otherwise write only the entries we have blocks for
	return w.writeFromParts(parts, entryBlocks)
}

func (w *Writer) writeRoundtrip(entryBlocks map[string][]string) error {
	zr, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return fmt.Errorf("archive writer: reading original: %w", err)
	}

	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			_, err := zw.Create(file.Name)
			if err != nil {
				return err
			}
			continue
		}

		header := file.FileHeader
		writer, err := zw.CreateHeader(&header)
		if err != nil {
			return err
		}

		if lines, ok := entryBlocks[file.Name]; ok {
			// Write translated content
			content := strings.Join(lines, "\n") + "\n"
			if _, err := io.WriteString(writer, content); err != nil {
				return err
			}
		} else {
			// Copy original content
			rc, err := file.Open()
			if err != nil {
				return err
			}
			if _, err := io.Copy(writer, rc); err != nil {
				rc.Close()
				return err
			}
			rc.Close()
		}
	}

	return nil
}

func (w *Writer) writeFromParts(parts []*model.Part, entryBlocks map[string][]string) error {
	zw := zip.NewWriter(w.Output)
	defer zw.Close()

	// Also include Data parts (binary entries) as empty placeholders
	written := make(map[string]bool)

	// Write text entries
	for entry, lines := range entryBlocks {
		writer, err := zw.Create(entry)
		if err != nil {
			return err
		}
		content := strings.Join(lines, "\n") + "\n"
		if _, err := io.WriteString(writer, content); err != nil {
			return err
		}
		written[entry] = true
	}

	// Write data entries as empty files (preserving structure)
	for _, part := range parts {
		if part.Type != model.PartData {
			continue
		}
		data, ok := part.Resource.(*model.Data)
		if !ok {
			continue
		}
		entry := data.Properties["entry"]
		if entry == "" || written[entry] {
			continue
		}
		if _, err := zw.Create(entry); err != nil {
			return err
		}
		written[entry] = true
	}

	return nil
}
