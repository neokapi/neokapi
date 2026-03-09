package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for EPUB e-book files.
type Writer struct {
	format.BaseFormatWriter
	originalContent []byte
}

// NewWriter creates a new EPUB writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "epub",
		},
	}
}

// SetOriginalContent provides the original EPUB bytes for roundtrip fidelity.
func (w *Writer) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write consumes Parts from a channel and writes a reconstructed EPUB.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var allParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeEPUB(allParts)
			}
			allParts = append(allParts, part)
		}
	}
}

func (w *Writer) writeEPUB(parts []*model.Part) error {
	if w.originalContent == nil {
		return fmt.Errorf("epub writer: original content required for roundtrip")
	}

	// Build map of entry -> translated blocks
	entryBlocks := make(map[string][]*model.Block)
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
		entryBlocks[entry] = append(entryBlocks[entry], block)
	}

	zr, err := zip.NewReader(bytes.NewReader(w.originalContent), int64(len(w.originalContent)))
	if err != nil {
		return fmt.Errorf("epub writer: reading original: %w", err)
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

		// Preserve compression settings via header copy
		header := file.FileHeader
		writer, err := zw.CreateHeader(&header)
		if err != nil {
			return err
		}

		if blocks, ok := entryBlocks[file.Name]; ok && len(blocks) > 0 {
			// Read original content
			rc, err := file.Open()
			if err != nil {
				return err
			}
			origContent, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}

			// Replace text in XHTML
			translated := replaceXHTMLText(origContent, blocks, w.Locale)
			if _, err := writer.Write(translated); err != nil {
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

// replaceXHTMLText replaces translatable text in XHTML content with translations.
func replaceXHTMLText(content []byte, blocks []*model.Block, locale model.LocaleID) []byte {
	// Build a map from source text to translated text
	replacements := make(map[string]string)
	for _, block := range blocks {
		sourceText := block.SourceText()
		targetText := sourceText
		if !locale.IsEmpty() && block.HasTarget(locale) {
			targetText = block.TargetText(locale)
		}
		replacements[sourceText] = targetText
	}

	// Parse and rebuild XHTML
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	var result bytes.Buffer
	encoder := xml.NewEncoder(&result)

	blockElements := map[string]bool{
		"p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "li": true,
		"dt": true, "dd": true, "th": true, "td": true,
		"figcaption": true, "caption": true, "summary": true,
		"blockquote": true, "title": true,
	}

	var textBuf strings.Builder
	inBlock := false
	depth := 0
	var pendingTokens []xml.Token

	flushBlock := func() {
		if textBuf.Len() > 0 {
			text := strings.TrimSpace(textBuf.String())
			if replacement, ok := replacements[text]; ok {
				// Replace all pending char data tokens with the replacement
				var newTokens []xml.Token
				replaced := false
				for _, tok := range pendingTokens {
					if _, isCharData := tok.(xml.CharData); isCharData && !replaced {
						newTokens = append(newTokens, xml.CharData(replacement))
						replaced = true
					} else if _, isCharData := tok.(xml.CharData); !isCharData {
						newTokens = append(newTokens, tok)
					}
				}
				for _, tok := range newTokens {
					_ = encoder.EncodeToken(tok)
				}
			} else {
				for _, tok := range pendingTokens {
					_ = encoder.EncodeToken(tok)
				}
			}
			textBuf.Reset()
			pendingTokens = nil
		} else {
			for _, tok := range pendingTokens {
				_ = encoder.EncodeToken(tok)
			}
			pendingTokens = nil
		}
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if blockElements[t.Name.Local] {
				if inBlock {
					flushBlock()
				}
				inBlock = true
				depth++
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else if inBlock {
				depth++
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				_ = encoder.EncodeToken(xml.CopyToken(t))
			}
		case xml.EndElement:
			if blockElements[t.Name.Local] {
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
				flushBlock()
				depth--
				if depth <= 0 {
					inBlock = false
					depth = 0
				}
			} else if inBlock {
				depth--
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				_ = encoder.EncodeToken(xml.CopyToken(t))
			}
		case xml.CharData:
			if inBlock {
				textBuf.Write(t)
				pendingTokens = append(pendingTokens, xml.CopyToken(t))
			} else {
				_ = encoder.EncodeToken(xml.CopyToken(t))
			}
		case xml.ProcInst:
			_ = encoder.EncodeToken(xml.CopyToken(t))
		case xml.Comment:
			_ = encoder.EncodeToken(xml.CopyToken(t))
		case xml.Directive:
			_ = encoder.EncodeToken(xml.CopyToken(t))
		}
	}

	flushBlock()
	encoder.Flush()

	return result.Bytes()
}
