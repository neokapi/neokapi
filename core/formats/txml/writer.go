package txml

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Trados XML (TXML) files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	sourceLocale  string
	targetLocale  string
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TXML writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "txml",
		},
		cfg: cfg,
	}
}

// Config returns the writer configuration for external modification.
func (w *Writer) Config() *Config { return w.cfg }

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed TXML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		// Collect all parts, then write from skeleton
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

	headerWritten := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if headerWritten {
					if _, err := io.WriteString(w.Output, "</body>\n</txml>\n"); err != nil {
						return err
					}
				}
				return nil
			}
			if part.Type == model.PartLayerStart {
				layer, ok := part.Resource.(*model.Layer)
				if !ok {
					continue
				}
				w.sourceLocale = string(layer.Locale)
				if tl, ok := layer.Properties["target-locale"]; ok {
					w.targetLocale = tl
				}
				if !headerWritten {
					if err := w.writeHeader(); err != nil {
						return err
					}
					headerWritten = true
				}
				continue
			}
			if !headerWritten {
				if err := w.writeHeader(); err != nil {
					return err
				}
				headerWritten = true
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
		return fmt.Errorf("txml writer: flush skeleton: %w", err)
	}

	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("txml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			// Ref ID is "segIdx:elemType" where segIdx is 0-based
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			segIdx, err := strconv.Atoi(idxStr)
			if err != nil || segIdx < 0 || segIdx >= len(w.blocks) {
				continue
			}
			block := w.blocks[segIdx]
			elemType := refSuffix

			var text string
			if elemType == "source" {
				text = block.SourceText()
			} else {
				// target
				targetLocale := model.LocaleID(w.targetLocale)
				if !targetLocale.IsEmpty() && block.HasTarget(targetLocale) {
					text = block.TargetText(targetLocale)
				} else {
					// Try any available target
					text = block.SourceText() // fallback
					for locale := range block.Targets {
						if block.HasTarget(locale) {
							text = block.TargetText(locale)
							break
						}
					}
				}
			}

			if _, err := io.WriteString(w.Output, xmlEscape(text)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Writer) writeHeader() error {
	if _, err := io.WriteString(w.Output, `<?xml version="1.0" encoding="utf-8"?>`+"\n"); err != nil {
		return err
	}
	sourceLocale := w.sourceLocale
	if sourceLocale == "" {
		sourceLocale = "en-US"
	}
	targetLocale := w.targetLocale
	if targetLocale == "" && !w.Locale.IsEmpty() {
		targetLocale = string(w.Locale)
	}
	if _, err := fmt.Fprintf(w.Output, `<txml locale="%s" targetlocale="%s" version="1.0" datatype="xml">`+"\n",
		xmlEscape(sourceLocale), xmlEscape(targetLocale)); err != nil {
		return err
	}
	if _, err := io.WriteString(w.Output, "<header/>\n<body>\n"); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("txml writer: expected Block resource")
	}

	sourceText := block.SourceText()
	targetText := ""

	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		targetText = block.TargetText(w.Locale)
	}

	segType := block.Properties["segtype"]
	if segType == "" {
		segType = "block"
	}

	if _, err := fmt.Fprintf(w.Output, `<segment segtype="%s">`+"\n", xmlEscape(segType)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "<source>%s</source>\n", xmlEscape(sourceText)); err != nil {
		return err
	}
	if targetText != "" {
		if _, err := fmt.Fprintf(w.Output, "<target>%s</target>\n", xmlEscape(targetText)); err != nil {
			return err
		}
	} else if w.cfg.AllowEmptyOutputTarget {
		if _, err := io.WriteString(w.Output, "<target/>\n"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w.Output, "</segment>\n"); err != nil {
		return err
	}

	return nil
}

// xmlEscape escapes XML special characters.
func xmlEscape(s string) string {
	var buf []byte
	for i := range len(s) {
		switch s[i] {
		case '&':
			buf = append(buf, []byte("&amp;")...)
		case '<':
			buf = append(buf, []byte("&lt;")...)
		case '>':
			buf = append(buf, []byte("&gt;")...)
		case '"':
			buf = append(buf, []byte("&quot;")...)
		default:
			buf = append(buf, s[i])
		}
	}
	return string(buf)
}
