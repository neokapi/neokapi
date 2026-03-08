package icml

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Adobe InCopy ICML files.
type Writer struct {
	format.BaseFormatWriter
	docContent string // original document content for skeleton-based reconstruction
}

// NewWriter creates a new ICML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "icml",
		},
	}
}

// Write consumes Parts from a channel and writes reconstructed ICML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var blocks []*model.Block
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeOutput(blocks)
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocks = append(blocks, block)
				}
			case model.PartData:
				if data, ok := part.Resource.(*model.Data); ok {
					if data.Name == "icml-document" {
						w.docContent = data.Properties["content"]
					}
				}
			}
		}
	}
}

// writeOutput reconstructs the ICML document, replacing Content text
// with translated content where available.
func (w *Writer) writeOutput(blocks []*model.Block) error {
	if w.docContent == "" {
		return w.writeMinimalICML(blocks)
	}
	return w.writeFromSkeleton(blocks)
}

// writeFromSkeleton reconstructs the ICML from the original XML structure,
// replacing <Content> element text with translated content.
func (w *Writer) writeFromSkeleton(blocks []*model.Block) error {
	// Build a map from source text to translated text.
	translations := make(map[string]string)
	for _, block := range blocks {
		src := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			translations[src] = block.TargetText(w.Locale)
		}
	}

	if len(translations) == 0 {
		// No translations, write original content.
		_, err := io.WriteString(w.Output, w.docContent)
		return err
	}

	// Walk the XML and replace Content text.
	// We use an offset-based approach: find each <Content> text and replace it.
	type contentSpan struct {
		textStart int
		textEnd   int
		text      string
	}
	var spans []contentSpan

	decoder := xml.NewDecoder(strings.NewReader(w.docContent))
	inContent := false

	for {
		offset := decoder.InputOffset()
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "Content" {
				inContent = true
			}
		case xml.EndElement:
			if t.Name.Local == "Content" {
				inContent = false
			}
		case xml.CharData:
			if inContent {
				text := string(t)
				endOffset := decoder.InputOffset()
				// The char data ends at endOffset; it starts at offset
				// (the offset before reading this token).
				spans = append(spans, contentSpan{
					textStart: int(offset),
					textEnd:   int(endOffset),
					text:      text,
				})
			}
		}
		_ = offset
	}

	// Build the output by walking the original content and replacing spans.
	output := w.docContent
	// Process spans in reverse order to preserve offsets.
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		// Check if we have a translation for text that contains this span.
		// We need to match accumulated paragraph text to individual Content texts.
		if replacement, ok := translations[span.text]; ok {
			output = output[:span.textStart] + xmlEscape(replacement) + output[span.textEnd:]
		}
	}

	// If simple per-Content replacement didn't work (because blocks aggregate
	// multiple Content elements), try block-sequential replacement.
	if !w.hasAnyReplacement(output, translations) {
		output = w.replaceSequential(blocks)
	}

	_, err := io.WriteString(w.Output, output)
	return err
}

// hasAnyReplacement checks if any translation was actually applied.
func (w *Writer) hasAnyReplacement(output string, translations map[string]string) bool {
	for _, target := range translations {
		if strings.Contains(output, xmlEscape(target)) {
			return true
		}
	}
	return len(translations) == 0
}

// replaceSequential does a sequential Content-text replacement, matching blocks
// to Content elements in document order.
func (w *Writer) replaceSequential(blocks []*model.Block) string {
	decoder := xml.NewDecoder(strings.NewReader(w.docContent))
	inContent := false
	inStory := false
	contentIndex := 0
	nonTransDepth := 0

	type replacement struct {
		start int
		end   int
		text  string
	}
	var replacements []replacement

	// First, collect all Content char data positions in Story elements.
	type contentText struct {
		start int
		end   int
		text  string
	}
	var contentTexts []contentText

	for {
		offset := decoder.InputOffset()
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if name == "Story" {
				inStory = true
			}
			if nonTranslatableElements[name] {
				nonTransDepth++
			}
			if name == "Content" && inStory && nonTransDepth == 0 {
				inContent = true
			}
		case xml.EndElement:
			name := t.Name.Local
			if name == "Story" {
				inStory = false
			}
			if nonTranslatableElements[name] {
				nonTransDepth--
			}
			if name == "Content" {
				inContent = false
			}
		case xml.CharData:
			if inContent && nonTransDepth == 0 {
				endOffset := decoder.InputOffset()
				contentTexts = append(contentTexts, contentText{
					start: int(offset),
					end:   int(endOffset),
					text:  string(t),
				})
			}
		}
		_ = offset
	}

	// Now match blocks to content texts sequentially.
	// Each block's source text should be the concatenation of one or more
	// sequential Content texts.
	output := w.docContent
	blockIdx := 0
	contentIdx := 0

	for blockIdx < len(blocks) && contentIdx < len(contentTexts) {
		block := blocks[blockIdx]
		src := block.SourceText()

		// Try to find how many Content elements make up this block.
		accumulated := ""
		startContentIdx := contentIdx
		for contentIdx < len(contentTexts) {
			accumulated += contentTexts[contentIdx].text
			contentIdx++
			if accumulated == src {
				break
			}
			// If accumulated already exceeds source, we have a mismatch.
			if len(accumulated) > len(src) {
				// Reset and skip this block.
				contentIdx = startContentIdx + 1
				accumulated = ""
				break
			}
		}

		if accumulated == src {
			text := src
			if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
				text = block.TargetText(w.Locale)
			}
			if text != src {
				// Replace the span from the first Content to the last.
				rStart := contentTexts[startContentIdx].start
				rEnd := contentTexts[contentIdx-1].end
				replacements = append(replacements, replacement{
					start: rStart,
					end:   rEnd,
					text:  xmlEscape(text),
				})
			}
		}
		blockIdx++
	}

	// Apply replacements in reverse.
	for i := len(replacements) - 1; i >= 0; i-- {
		r := replacements[i]
		output = output[:r.start] + r.text + output[r.end:]
	}

	_ = contentIndex
	return output
}

// writeMinimalICML generates a minimal ICML document from blocks only.
func (w *Writer) writeMinimalICML(blocks []*model.Block) error {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	sb.WriteString(`<?aid style="50" type="snippet" readerVersion="6.0" featureSet="513" product="8.0(370)" ?>` + "\n")
	sb.WriteString(`<Document DOMVersion="8.0">` + "\n")
	sb.WriteString("  <Story>\n")

	for _, block := range blocks {
		text := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			text = block.TargetText(w.Locale)
		}

		style := block.Properties["paragraphStyle"]
		if style == "" {
			style = "ParagraphStyle/$ID/NormalParagraphStyle"
		}

		sb.WriteString(fmt.Sprintf(`    <ParagraphStyleRange AppliedParagraphStyle="%s">`+"\n", xmlEscape(style)))
		sb.WriteString(`      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">` + "\n")
		sb.WriteString(fmt.Sprintf("        <Content>%s</Content>\n", xmlEscape(text)))
		sb.WriteString("      </CharacterStyleRange>\n")
		sb.WriteString("    </ParagraphStyleRange>\n")
	}

	sb.WriteString("  </Story>\n")
	sb.WriteString("</Document>\n")

	_, err := io.WriteString(w.Output, sb.String())
	return err
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
