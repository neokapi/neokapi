package xliff2

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for XLIFF 2.x files.
type Writer struct {
	format.BaseFormatWriter
	cfg            *Config
	skeletonStore  *format.SkeletonStore
	blocks         []*model.Block
	sourceLang     model.LocaleID
	targetLang     model.LocaleID
	fileID         string
	inputVersion   string
	inputExtraAttr []xml.Attr
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new XLIFF 2.x writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xliff2",
		},
		cfg: cfg,
	}
}

// Config returns the writer's configuration (mutable).
func (w *Writer) Config() *Config { return w.cfg }

// SetVersion overrides the emitted XLIFF 2.x version. Valid values are
// "2.0", "2.1", "2.2". Empty resets to auto (preserve input, else default).
// Returns an error if v is not a supported XLIFF 2.x version.
func (w *Writer) SetVersion(v string) error {
	if v != "" && !IsSupportedVersion(v) {
		return fmt.Errorf("xliff2: unsupported XLIFF 2.x version %q (expected one of %v)", v, SupportedXLIFFVersions)
	}
	w.cfg.Version = v
	return nil
}

// resolveVersion returns the version this writer should emit.
// Precedence: explicit Config.Version → input document's version → DefaultXLIFFVersion.
func (w *Writer) resolveVersion() string {
	if w.cfg != nil && w.cfg.Version != "" {
		return w.cfg.Version
	}
	if w.inputVersion != "" && IsSupportedVersion(w.inputVersion) {
		return w.inputVersion
	}
	return DefaultXLIFFVersion
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes XLIFF 2.x output.
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
					if v, ok := layer.Properties["xliff-version"]; ok {
						w.inputVersion = v
					}
					w.inputExtraAttr = extraAttrsFromLayer(layer)
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
		if errors.Is(err, io.EOF) {
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
			refID := string(entry.Data)
			// Ref ID is "blockIdx:elemType"
			idxStr, refSuffix, ok := strings.Cut(refID, ":")
			if !ok {
				continue
			}
			blockIdx, err := strconv.Atoi(idxStr)
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			block := w.blocks[blockIdx]
			elemType := refSuffix

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
		XMLName xml.Name   `xml:"xliff"`
		Attrs   []xml.Attr `xml:",any,attr"`
		Version string     `xml:"version,attr"`
		Xmlns   string     `xml:"xmlns,attr"`
		SrcLang string     `xml:"srcLang,attr"`
		TrgLang string     `xml:"trgLang,attr,omitempty"`
		Files   []xmlFile  `xml:"file"`
	}

	var units []xmlUnit
	for _, block := range w.blocks {
		var segments []xmlSegment
		for _, seg := range block.Source {
			s := xmlSegment{
				ID:     seg.ID,
				Source: xmlSource{Content: seg.Text()},
			}
			// Check for target in the block's Targets map
			if trgSegs, ok := block.Targets[targetLang]; ok {
				for _, ts := range trgSegs {
					if ts.ID == seg.ID {
						s.Target = &xmlTarget{Content: ts.Text()}
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

	version := w.resolveVersion()
	doc := xmlXliff{
		Version: version,
		Xmlns:   NamespaceForVersion(version),
		Attrs:   w.inputExtraAttr,
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
