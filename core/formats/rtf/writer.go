package rtf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for RTF files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstBlock    bool
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new RTF writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "rtf",
		},
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed RTF.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		// Collect all blocks, then write from skeleton
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case part, ok := <-parts:
				if !ok {
					return w.writeFromSkeleton()
				}
				if part.Type == model.PartBlock {
					if block, ok := part.Resource.(*model.Block); ok {
						w.blocks = append(w.blocks, block)
					}
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				// End of stream - close the RTF document.
				if !w.firstBlock {
					if _, err := fmt.Fprint(w.Output, "}\n"); err != nil {
						return err
					}
				}
				return nil
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("rtf writer: flush skeleton: %w", err)
	}

	// First pass: collect all entries to know the total original length per block
	// and which refs are the last for each block.
	type skelEntry struct {
		entry format.SkeletonEntry
	}
	var entries []skelEntry

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("rtf writer: read skeleton: %w", err)
		}
		entries = append(entries, skelEntry{entry: entry})
	}

	// Find the last token index for each block
	lastTokenIdx := make(map[int]int) // blockIdx -> highest tokenIdx seen
	for _, se := range entries {
		if se.entry.Type != format.SkeletonRef {
			continue
		}
		parts := splitRefParts(string(se.entry.Data))
		if len(parts) != 3 {
			continue
		}
		blockIdx, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		tokenIdx, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		if tokenIdx > lastTokenIdx[blockIdx] {
			lastTokenIdx[blockIdx] = tokenIdx
		}
	}

	// Second pass: write output
	blockOffsets := make(map[int]int) // blockIdx -> bytes consumed so far

	for _, se := range entries {
		switch se.entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(se.entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			parts := splitRefParts(string(se.entry.Data))
			if len(parts) != 3 {
				continue
			}
			blockIdx, err := strconv.Atoi(parts[0])
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			tokenIdx, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			origLen, err := strconv.Atoi(parts[2])
			if err != nil {
				continue
			}

			block := w.blocks[blockIdx]
			text := block.SourceText()
			offset := blockOffsets[blockIdx]
			isLast := tokenIdx == lastTokenIdx[blockIdx]

			if isLast {
				// Last token for this block: write all remaining text
				if offset < len(text) {
					if _, err := io.WriteString(w.Output, text[offset:]); err != nil {
						return err
					}
				}
				blockOffsets[blockIdx] = len(text)
			} else {
				// Not the last token: write origLen bytes
				end := min(offset+origLen, len(text))
				if offset < len(text) {
					if _, err := io.WriteString(w.Output, text[offset:end]); err != nil {
						return err
					}
				}
				blockOffsets[blockIdx] = end
			}
		}
	}
	return nil
}

// splitRefParts splits a ref ID by colons. We can't use strings.SplitN because
// we need exactly 3 parts.
func splitRefParts(s string) []string {
	idx1 := -1
	idx2 := -1
	for i, c := range s {
		if c == ':' {
			if idx1 < 0 {
				idx1 = i
			} else {
				idx2 = i
				break
			}
		}
	}
	if idx1 < 0 || idx2 < 0 {
		return nil
	}
	return []string{s[:idx1], s[idx1+1 : idx2], s[idx2+1:]}
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartLayerStart:
		return w.writeHeader()
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartData:
		return w.writeData(part)
	default:
		return nil
	}
}

func (w *Writer) writeHeader() error {
	// \uc1 declares that each \uN is followed by exactly one ANSI fallback
	// character. Emitting this explicitly (even though it is the spec
	// default) makes the output unambiguous for consumers that don't assume
	// the default.
	if _, err := fmt.Fprint(w.Output, "{\\rtf1\\ansi\\deff0\\uc1\n"); err != nil {
		return err
	}
	w.firstBlock = false
	return nil
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("rtf writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Escape special RTF characters in the text.
	escaped := escapeRTF(text)

	if _, err := fmt.Fprintf(w.Output, "\\pard %s\\par\n", escaped); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("rtf writer: expected Data resource")
	}
	raw := data.Properties["raw"]
	if raw != "" {
		if _, err := fmt.Fprint(w.Output, raw); err != nil {
			return err
		}
	}
	return nil
}

// escapeRTF escapes special characters for RTF output.
func escapeRTF(s string) string {
	var out []byte
	for _, r := range s {
		switch {
		case r == '\\':
			out = append(out, '\\', '\\')
		case r == '{':
			out = append(out, '\\', '{')
		case r == '}':
			out = append(out, '\\', '}')
		case r == '\t':
			out = append(out, '\\', 't', 'a', 'b', ' ')
		case r > 127:
			out = append(out, fmt.Appendf(nil, "\\u%d?", r)...)
		default:
			out = append(out, byte(r))
		}
	}
	return string(out)
}
