package vignette

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for R Vignette files (.Rnw/.Rmd).
// Code chunks and YAML front matter are non-translatable (Data).
// Text outside code chunks is translatable (Blocks).
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new R Vignette reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "vignette",
			FormatDisplayName: "R Vignette",
			FormatMimeType:    "text/x-r-markdown",
			FormatExtensions:  []string{".Rmd", ".Rnw"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/x-r-markdown"},
		Extensions: []string{".Rmd", ".Rnw"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("vignette: nil document or reader")
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

type parseState int

const (
	stateText parseState = iota
	stateYAML
	stateRmdCode  // ```{r} ... ```
	stateRnwCode  // <<>>= ... @
)

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "vignette",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/x-r-markdown",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	scanner := bufio.NewScanner(r.Doc.Reader)
	blockID := 0
	dataID := 0
	state := stateText
	lineNum := 0
	yamlStartSeen := false

	var textLines []string

	flushText := func() bool {
		if len(textLines) == 0 {
			return true
		}
		joined := strings.Join(textLines, "\n")
		textLines = nil

		if strings.TrimSpace(joined) == "" {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "whitespace",
			}
			return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		}

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), joined)
		block.Name = fmt.Sprintf("text%d", blockID)
		return r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		switch state {
		case stateText:
			// Check for YAML front matter start (only at line 1)
			if lineNum == 1 && strings.TrimSpace(line) == "---" {
				if !flushText() {
					return
				}
				state = stateYAML
				yamlStartSeen = true
				dataID++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataID),
					Name: "yaml-start",
					Properties: map[string]string{
						"type": "yaml-frontmatter",
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			// Check for Rmd code chunk start: ```{r ...}
			if isRmdCodeStart(line) {
				if !flushText() {
					return
				}
				state = stateRmdCode
				dataID++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataID),
					Name: "code-chunk-start",
					Properties: map[string]string{
						"type": "rmd-code",
						"line": line,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			// Check for Rnw code chunk start: <<...>>=
			if isRnwCodeStart(line) {
				if !flushText() {
					return
				}
				state = stateRnwCode
				dataID++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataID),
					Name: "code-chunk-start",
					Properties: map[string]string{
						"type": "rnw-code",
						"line": line,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			textLines = append(textLines, line)

		case stateYAML:
			if strings.TrimSpace(line) == "---" && yamlStartSeen {
				dataID++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataID),
					Name: "yaml-end",
					Properties: map[string]string{
						"type": "yaml-frontmatter",
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				state = stateText
				continue
			}
			// YAML content is non-translatable
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("yaml-line.%d", lineNum),
				Properties: map[string]string{
					"type": "yaml-content",
					"line": line,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}

		case stateRmdCode:
			if strings.TrimSpace(line) == "```" {
				dataID++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataID),
					Name: "code-chunk-end",
					Properties: map[string]string{
						"type": "rmd-code",
						"line": line,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				state = stateText
				continue
			}
			// Code content is non-translatable
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("code.%d", lineNum),
				Properties: map[string]string{
					"type": "code",
					"line": line,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}

		case stateRnwCode:
			if strings.TrimSpace(line) == "@" {
				dataID++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataID),
					Name: "code-chunk-end",
					Properties: map[string]string{
						"type": "rnw-code",
						"line": line,
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				state = stateText
				continue
			}
			// Code content is non-translatable
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("code.%d", lineNum),
				Properties: map[string]string{
					"type": "code",
					"line": line,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}

	// Flush any remaining text
	if !flushText() {
		return
	}

	if err := scanner.Err(); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("vignette: reading: %w", err)}
		return
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// isRmdCodeStart checks if a line starts an R Markdown code chunk.
// Matches: ```{r}, ```{r label}, ```{r, echo=FALSE}, etc.
func isRmdCodeStart(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```{")
}

// isRnwCodeStart checks if a line starts an Rnw (Sweave) code chunk.
// Matches: <<>>=, <<label>>=, <<label, echo=FALSE>>=, etc.
func isRnwCodeStart(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "<<") && strings.HasSuffix(trimmed, ">>=")
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
