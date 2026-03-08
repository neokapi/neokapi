package mif

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for MIF (Maker Interchange Format) files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new MIF reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mif",
			FormatDisplayName: "Adobe FrameMaker MIF",
			FormatMimeType:    "application/x-mif",
			FormatExtensions:  []string{".mif"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-mif", "application/vnd.mif"},
		Extensions: []string{".mif"},
		Sniff: func(data []byte) bool {
			return len(data) >= 9 && string(data[:9]) == "<MIFFile "
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("mif: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// mifStatement represents a parsed MIF statement.
type mifStatement struct {
	tag      string
	value    string
	children []*mifStatement
	raw      string // Original raw text for non-translatable parts.
}

// skipTags are top-level MIF tags whose content is non-translatable.
var skipTags = map[string]bool{
	"Units":           true,
	"ColorCatalog":    true,
	"ConditionCatalog": true,
	"BoolCondCatalog": true,
	"CombinedFontCatalog": true,
	"FontCatalog":     true,
	"RulingCatalog":   true,
	"TblCatalog":      true,
	"Views":           true,
	"VariableFormats": true,
	"XRefFormats":     true,
	"Document":        true,
	"BookComponent":   true,
	"InitialAutoNums": true,
	"Dictionary":      true,
	"PgfCatalog":      true,
	"ElementDefCatalog": true,
	"FmtChangeListCatalog": true,
	"DefAttrValuesCatalog": true,
	"AttrCondExprCatalog": true,
	"MasterPage":      true,
	"ReferencePage":   true,
	"Page":            true,
	"AFrames":         true,
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "mif",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-mif",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitErr(ctx, ch, fmt.Errorf("mif: read error: %w", err))
		return
	}

	stmts := parseMIF(string(data))
	r.emitStatements(ctx, ch, stmts)

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) emitStatements(ctx context.Context, ch chan<- model.PartResult, stmts []*mifStatement) {
	blockCounter := 0
	dataCounter := 0

	for _, stmt := range stmts {
		if skipTags[stmt.tag] {
			// Emit as non-translatable data.
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: fmt.Sprintf("mif.%s", stmt.tag),
				Properties: map[string]string{
					"tag": stmt.tag,
					"raw": stmt.raw,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		if stmt.tag == "MIFFile" {
			// Emit version line as data.
			dataCounter++
			d := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "mif.version",
				Properties: map[string]string{
					"tag":     "MIFFile",
					"version": stmt.value,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
				return
			}
			continue
		}

		if stmt.tag == "TextFlow" || stmt.tag == "Tbls" || stmt.tag == "Notes" {
			// Process translatable content inside these containers.
			blockCounter, dataCounter = r.processContainer(ctx, ch, stmt, blockCounter, dataCounter)
			continue
		}

		// Default: emit as data.
		dataCounter++
		d := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: fmt.Sprintf("mif.%s", stmt.tag),
			Properties: map[string]string{
				"tag": stmt.tag,
				"raw": stmt.raw,
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: d}) {
			return
		}
	}
}

// processContainer recursively processes a MIF container for translatable strings.
func (r *Reader) processContainer(ctx context.Context, ch chan<- model.PartResult, stmt *mifStatement, blockCounter, dataCounter int) (int, int) {
	for _, child := range stmt.children {
		if child.tag == "Para" {
			text := extractParaText(child)
			if strings.TrimSpace(text) != "" {
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
				block.Name = fmt.Sprintf("para.%d", blockCounter)

				// Extract paragraph tag if present.
				for _, gc := range child.children {
					if gc.tag == "PgfTag" {
						block.Properties["pgf_tag"] = gc.value
						break
					}
				}

				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return blockCounter, dataCounter
				}
			}
		} else if child.tag == "TextFlow" || child.tag == "Notes" || child.tag == "Tbls" || child.tag == "Cell" || child.tag == "CellContent" || child.tag == "Row" || child.tag == "TblBody" || child.tag == "Tbl" {
			blockCounter, dataCounter = r.processContainer(ctx, ch, child, blockCounter, dataCounter)
		}
	}
	return blockCounter, dataCounter
}

// extractParaText extracts translatable text from a Para statement.
func extractParaText(para *mifStatement) string {
	var texts []string
	for _, child := range para.children {
		if child.tag == "ParaLine" {
			for _, lc := range child.children {
				switch lc.tag {
				case "String":
					texts = append(texts, lc.value)
				case "Char":
					switch lc.value {
					case "HardReturn":
						texts = append(texts, "\n")
					case "Tab":
						texts = append(texts, "\t")
					case "HardSpace":
						texts = append(texts, "\u00A0")
					case "SoftHyphen":
						texts = append(texts, "\u00AD")
					case "EnSpace":
						texts = append(texts, "\u2002")
					case "EmSpace":
						texts = append(texts, "\u2003")
					case "ThinSpace":
						texts = append(texts, "\u2009")
					}
				}
			}
		}
	}
	return strings.Join(texts, "")
}

// parseMIF parses a MIF document into a list of top-level statements.
func parseMIF(content string) []*mifStatement {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var stmts []*mifStatement
	var stack []*mifStatement
	var rawBuilder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if trimmed == ">" {
			// End of current statement.
			if len(stack) > 0 {
				current := stack[len(stack)-1]
				current.raw += line + "\n"
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					parent := stack[len(stack)-1]
					parent.children = append(parent.children, current)
					parent.raw += current.raw
				} else {
					stmts = append(stmts, current)
				}
			}
			continue
		}

		if strings.HasPrefix(trimmed, "<") {
			// Start of a new statement.
			tag, value := parseTagLine(trimmed)
			stmt := &mifStatement{
				tag:   tag,
				value: value,
				raw:   line + "\n",
			}

			// Check if this is a single-line statement (ends with >).
			if strings.HasSuffix(trimmed, ">") && !strings.HasSuffix(trimmed, " >") {
				// Single-line statement like <MIFFile 2015> with no space before >.
				// Actually, need to check more carefully.
				inner := trimmed[1 : len(trimmed)-1] // remove < and >
				parts := strings.SplitN(inner, " ", 2)
				stmt.tag = parts[0]
				if len(parts) > 1 {
					stmt.value = unquoteMIF(parts[1])
				}
				if len(stack) > 0 {
					parent := stack[len(stack)-1]
					parent.children = append(parent.children, stmt)
					parent.raw += line + "\n"
				} else {
					stmts = append(stmts, stmt)
				}
				continue
			}

			stack = append(stack, stmt)
			continue
		}

		// Non-statement line (comment or other content).
		if len(stack) > 0 {
			stack[len(stack)-1].raw += line + "\n"
		} else {
			rawBuilder.WriteString(line + "\n")
		}
	}

	// Flush any unclosed statements.
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, current)
			parent.raw += current.raw
		} else {
			stmts = append(stmts, current)
		}
	}

	return stmts
}

// parseTagLine parses a MIF line like "<TagName value" or "<TagName".
func parseTagLine(line string) (string, string) {
	// Remove leading '<'.
	line = strings.TrimPrefix(line, "<")
	// Remove trailing '>' if present (single-line statement).
	line = strings.TrimSuffix(line, ">")
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, " ", 2)
	tag := parts[0]
	var value string
	if len(parts) > 1 {
		value = unquoteMIF(strings.TrimSpace(parts[1]))
	}
	return tag, value
}

// unquoteMIF removes MIF backtick-single-quote delimiters from a string value.
func unquoteMIF(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Reader) emitErr(ctx context.Context, ch chan<- model.PartResult, err error) {
	select {
	case ch <- model.PartResult{Error: err}:
	case <-ctx.Done():
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
