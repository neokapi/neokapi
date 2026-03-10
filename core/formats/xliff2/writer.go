package xliff2

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 2.0 files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	blocks        []*model.Block
	sourceLang    model.LocaleID
	targetLang    model.LocaleID
	fileID        string
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new XLIFF 2.0 writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff2",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes XLIFF 2.0 output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if w.skeletonStore != nil {
					return w.writeFromSkeleton()
				}
				return w.flush()
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					w.blocks = append(w.blocks, block)
				}
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok {
					w.sourceLang = layer.Locale
					w.fileID = layer.Name
					if tl, ok := layer.Properties["target-language"]; ok {
						w.targetLang = model.LocaleID(tl)
					}
				}
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("xliff2 writer: flush skeleton: %w", err)
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("xliff2 writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			// Ref ID is "blockIdx:elemType"
			refID := string(entry.Data)
			parts := strings.SplitN(refID, ":", 2)
			if len(parts) != 2 {
				continue
			}
			blockIdx, err := strconv.Atoi(parts[0])
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			block := w.blocks[blockIdx]
			elemType := parts[1]

			var text string
			switch elemType {
			case "source":
				text = block.SourceText()
			case "target":
				if block.HasTarget(targetLang) {
					text = block.TargetText(targetLang)
				} else {
					// Fallback to original source text
					text = block.SourceText()
				}
			}

			if _, err := io.WriteString(w.Output, xmlEscapeText(text)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	targetLang := w.targetLang
	if !w.Locale.IsEmpty() {
		targetLang = w.Locale
	}

	type xmlTarget struct {
		XMLName xml.Name `xml:"target"`
		Content string   `xml:",chardata"`
	}
	type xmlSource struct {
		XMLName xml.Name `xml:"source"`
		Content string   `xml:",chardata"`
	}
	type xmlSegment struct {
		XMLName xml.Name   `xml:"segment"`
		ID      string     `xml:"id,attr,omitempty"`
		Source  xmlSource  `xml:"source"`
		Target  *xmlTarget `xml:"target,omitempty"`
	}
	type xmlUnit struct {
		XMLName  xml.Name     `xml:"unit"`
		ID       string       `xml:"id,attr"`
		Name     string       `xml:"name,attr,omitempty"`
		Segments []xmlSegment `xml:"segment"`
	}
	type xmlFile struct {
		XMLName xml.Name  `xml:"file"`
		ID      string    `xml:"id,attr"`
		Units   []xmlUnit `xml:"unit"`
	}
	type xmlXliff struct {
		XMLName xml.Name  `xml:"xliff"`
		Version string    `xml:"version,attr"`
		Xmlns   string    `xml:"xmlns,attr"`
		SrcLang string    `xml:"srcLang,attr"`
		TrgLang string    `xml:"trgLang,attr,omitempty"`
		Files   []xmlFile `xml:"file"`
	}

	var units []xmlUnit
	for _, block := range w.blocks {
		var segments []xmlSegment
		for _, seg := range block.Source {
			s := xmlSegment{
				ID:     seg.ID,
				Source: xmlSource{Content: seg.Content.Text()},
			}
			// Check for target in the block's Targets map
			if trgSegs, ok := block.Targets[targetLang]; ok {
				for _, ts := range trgSegs {
					if ts.ID == seg.ID {
						s.Target = &xmlTarget{Content: ts.Content.Text()}
						break
					}
				}
			}
			segments = append(segments, s)
		}
		units = append(units, xmlUnit{
			ID:       block.ID,
			Name:     block.Name,
			Segments: segments,
		})
	}

	doc := xmlXliff{
		Version: "2.0",
		Xmlns:   "urn:oasis:names:tc:xliff:document:2.0",
		SrcLang: string(w.sourceLang),
		TrgLang: string(targetLang),
		Files: []xmlFile{
			{
				ID:    w.fileID,
				Units: units,
			},
		},
	}

	fmt.Fprint(w.Output, xml.Header)
	encoder := xml.NewEncoder(w.Output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("xliff2 writer: encoding: %w", err)
	}
	return encoder.Flush()
}
