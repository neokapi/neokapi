package ts

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

// Writer implements DataFormatWriter for Qt TS files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	headerProps   map[string]string
	groups        []*contextGroup
	currentCtx    *contextGroup
	allBlocks     []*model.Block // all blocks in order, for skeleton lookup
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// contextGroup holds blocks for one <context> element.
type contextGroup struct {
	name   string
	blocks []*model.Block
}

// NewWriter creates a new Qt TS writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "ts",
		},
		headerProps: make(map[string]string),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes Qt TS XML.
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
			w.collectPart(part)
		}
	}
}

func (w *Writer) collectPart(part *model.Part) {
	switch part.Type {
	case model.PartData:
		if data, ok := part.Resource.(*model.Data); ok {
			if data.Name == "ts-header" {
				w.headerProps = data.Properties
			}
		}
	case model.PartGroupStart:
		if gs, ok := part.Resource.(*model.GroupStart); ok {
			w.currentCtx = &contextGroup{name: gs.Name}
			w.groups = append(w.groups, w.currentCtx)
		}
	case model.PartGroupEnd:
		w.currentCtx = nil
	case model.PartBlock:
		if block, ok := part.Resource.(*model.Block); ok {
			w.allBlocks = append(w.allBlocks, block)
			if w.currentCtx != nil {
				w.currentCtx.blocks = append(w.currentCtx.blocks, block)
			} else {
				// Block without a context group -- create an implicit one
				ctxName := block.Properties["context"]
				if ctxName == "" {
					ctxName = "default"
				}
				// Find or create context
				var grp *contextGroup
				for _, g := range w.groups {
					if g.name == ctxName {
						grp = g
						break
					}
				}
				if grp == nil {
					grp = &contextGroup{name: ctxName}
					w.groups = append(w.groups, grp)
				}
				grp.blocks = append(grp.blocks, block)
			}
		}
	}
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("ts writer: flush skeleton: %w", err)
	}

	// Determine target locale
	language := w.headerProps["language"]
	targetLocale := model.LocaleID(language)
	if w.Locale != "" {
		targetLocale = w.Locale
	}

	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("ts writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			// Ref ID is "blockIdx:elemType" where blockIdx is 0-based
			refID := string(entry.Data)
			parts := strings.SplitN(refID, ":", 2)
			if len(parts) != 2 {
				continue
			}
			blockIdx, err := strconv.Atoi(parts[0])
			if err != nil || blockIdx < 0 || blockIdx >= len(w.allBlocks) {
				continue
			}
			block := w.allBlocks[blockIdx]
			elemType := parts[1]

			var text string
			switch elemType {
			case "source":
				text = w.fragmentToXML(block.FirstFragment())
			case "translation":
				if block.HasTarget(targetLocale) {
					targetSegs := block.Targets[targetLocale]
					if len(targetSegs) > 0 && targetSegs[0].Content != nil {
						text = w.fragmentToXML(targetSegs[0].Content)
					}
				}
			}

			if _, err := io.WriteString(w.Output, text); err != nil {
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

	version := w.headerProps["version"]
	if version == "" {
		version = "2.1"
	}
	language := w.headerProps["language"]
	srcLanguage := w.headerProps["sourcelanguage"]

	// Determine target locale from writer setting or header
	targetLocale := model.LocaleID(language)
	if w.Locale != "" {
		targetLocale = w.Locale
	}

	// Write XML header
	if _, err := io.WriteString(w.Output, xml.Header); err != nil {
		return err
	}
	if _, err := io.WriteString(w.Output, "<!DOCTYPE TS>\n"); err != nil {
		return err
	}

	// Build TS opening tag
	tsAttrs := fmt.Sprintf(`<TS version="%s"`, xmlEscape(version))
	if language != "" {
		tsAttrs += fmt.Sprintf(` language="%s"`, xmlEscape(language))
	}
	if srcLanguage != "" {
		tsAttrs += fmt.Sprintf(` sourcelanguage="%s"`, xmlEscape(srcLanguage))
	}
	tsAttrs += ">\n"
	if _, err := io.WriteString(w.Output, tsAttrs); err != nil {
		return err
	}

	for _, grp := range w.groups {
		if _, err := io.WriteString(w.Output, "<context>\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w.Output, "    <name>%s</name>\n", xmlEscape(grp.name)); err != nil {
			return err
		}

		for _, block := range grp.blocks {
			if err := w.writeMessage(block, targetLocale); err != nil {
				return err
			}
		}

		if _, err := io.WriteString(w.Output, "</context>\n"); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w.Output, "</TS>\n"); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeMessage(block *model.Block, targetLocale model.LocaleID) error {
	// Build <message> opening tag
	msgAttrs := "    <message"
	if block.ID != "" && !strings.HasPrefix(block.ID, "tu") {
		msgAttrs += fmt.Sprintf(` id="%s"`, xmlEscape(block.ID))
	}
	if block.Properties["numerus"] == "yes" {
		msgAttrs += ` numerus="yes"`
	}
	msgAttrs += ">\n"
	if _, err := io.WriteString(w.Output, msgAttrs); err != nil {
		return err
	}

	// Write source
	sourceText := w.fragmentToXML(block.FirstFragment())
	if _, err := fmt.Fprintf(w.Output, "        <source>%s</source>\n", sourceText); err != nil {
		return err
	}

	// Write comment
	if comment := block.Properties["comment"]; comment != "" {
		if _, err := fmt.Fprintf(w.Output, "        <comment>%s</comment>\n", xmlEscape(comment)); err != nil {
			return err
		}
	}

	// Write extracomment
	if extraComment := block.Properties["extracomment"]; extraComment != "" {
		if _, err := fmt.Fprintf(w.Output, "        <extracomment>%s</extracomment>\n", xmlEscape(extraComment)); err != nil {
			return err
		}
	}

	// Write translatorcomment
	if transComment := block.Properties["translatorcomment"]; transComment != "" {
		if _, err := fmt.Fprintf(w.Output, "        <translatorcomment>%s</translatorcomment>\n", xmlEscape(transComment)); err != nil {
			return err
		}
	}

	// Write translation
	transType := block.Properties["type"]

	if block.Properties["numerus"] == "yes" {
		// Write numerus forms
		transOpen := "        <translation"
		if transType != "" {
			transOpen += fmt.Sprintf(` type="%s"`, xmlEscape(transType))
		}
		transOpen += ">\n"
		if _, err := io.WriteString(w.Output, transOpen); err != nil {
			return err
		}

		// Collect numerus forms from properties
		for i := 0; ; i++ {
			key := fmt.Sprintf("numerusform:%d", i)
			form, ok := block.Properties[key]
			if !ok {
				break
			}
			if _, err := fmt.Fprintf(w.Output, "            <numerusform>%s</numerusform>\n", xmlEscape(form)); err != nil {
				return err
			}
		}

		if _, err := io.WriteString(w.Output, "        </translation>\n"); err != nil {
			return err
		}
	} else {
		// Write regular translation
		transOpen := "        <translation"
		if transType != "" {
			transOpen += fmt.Sprintf(` type="%s"`, xmlEscape(transType))
		}

		if block.HasTarget(targetLocale) {
			targetSegs := block.Targets[targetLocale]
			if len(targetSegs) > 0 && targetSegs[0].Content != nil {
				targetXML := w.fragmentToXML(targetSegs[0].Content)
				transOpen += fmt.Sprintf(">%s</translation>\n", targetXML)
			} else {
				transOpen += "></translation>\n"
			}
		} else {
			transOpen += "></translation>\n"
		}
		if _, err := io.WriteString(w.Output, transOpen); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w.Output, "    </message>\n"); err != nil {
		return err
	}

	return nil
}

// fragmentToXML converts a Fragment back to XML string, preserving byte elements.
func (w *Writer) fragmentToXML(frag *model.Fragment) string {
	if frag == nil {
		return ""
	}
	if !frag.HasSpans() {
		return xmlEscape(frag.Text())
	}

	var buf strings.Builder
	spanIdx := 0
	for _, r := range frag.CodedText {
		if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
			if spanIdx < len(frag.Spans) {
				span := frag.Spans[spanIdx]
				spanIdx++
				if span.Type == "byte" {
					// Write back the <byte> element
					buf.WriteString(span.Data)
				} else {
					buf.WriteString(span.Data)
				}
			}
		} else {
			buf.WriteString(xmlEscapeRune(r))
		}
	}
	return buf.String()
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var buf strings.Builder
	for _, r := range s {
		buf.WriteString(xmlEscapeRune(r))
	}
	return buf.String()
}

// xmlEscapeRune escapes a single rune for XML.
func xmlEscapeRune(r rune) string {
	switch r {
	case '&':
		return "&amp;"
	case '<':
		return "&lt;"
	case '>':
		return "&gt;"
	case '"':
		return "&quot;"
	default:
		return string(r)
	}
}
