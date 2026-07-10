package brandscan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/formats/html"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/model"
)

// MaxFileBytes caps an uploaded brand file at 10 MiB.
const MaxFileBytes = 10 << 20

// deferredExtractorMsg is returned for formats on the roadmap that need a
// dedicated text-extraction dependency (a later slice); they are not silently
// treated as unknown types.
const deferredExtractorMsg = "unsupported (deferred: needs pdf/pptx text extractor)"

// ExtractFile converts an uploaded brand file into plain text by running it
// through the matching framework format reader and concatenating the source
// text of its translatable blocks. The format is picked by file extension
// first, then by content type. Files larger than MaxFileBytes, unknown types,
// and the deferred pdf/pptx types are rejected with an error.
func ExtractFile(data []byte, filename, contentType string) (string, error) {
	if int64(len(data)) > MaxFileBytes {
		return "", fmt.Errorf("brandscan: file %q too large: %d bytes (max %d)", filename, len(data), MaxFileBytes)
	}
	newReader, err := readerForFile(filename, contentType)
	if err != nil {
		return "", err
	}
	uri := filename
	if uri == "" {
		uri = "brandscan-upload"
	}
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	text, err := readerText(context.Background(), newReader(), doc)
	if err != nil {
		return "", fmt.Errorf("brandscan: extract %q: %w", filename, err)
	}
	return text, nil
}

// readersByExt maps allowlisted file extensions to reader factories.
var readersByExt = map[string]func() format.DataFormatReader{
	".md":       func() format.DataFormatReader { return markdown.NewReader() },
	".markdown": func() format.DataFormatReader { return markdown.NewReader() },
	".html":     newHTMLReader,
	".htm":      newHTMLReader,
	".xhtml":    newHTMLReader,
	".txt":      func() format.DataFormatReader { return plaintext.NewReader() },
	".text":     func() format.DataFormatReader { return plaintext.NewReader() },
	".csv":      func() format.DataFormatReader { return csvfmt.NewReader() },
	".tsv":      func() format.DataFormatReader { return csvfmt.NewTSVReader() },
	".docx":     func() format.DataFormatReader { return openxml.NewReader() },
	".xlsx":     func() format.DataFormatReader { return openxml.NewReader() },
	".odt":      func() format.DataFormatReader { return odf.NewReader() },
	".ods":      func() format.DataFormatReader { return odf.NewReader() },
	".odp":      func() format.DataFormatReader { return odf.NewReader() },
	".epub":     func() format.DataFormatReader { return epub.NewReader() },
	".xml":      func() format.DataFormatReader { return xmlfmt.NewReader() },
	".json":     func() format.DataFormatReader { return jsonfmt.NewReader() },
	".yaml":     func() format.DataFormatReader { return yamlfmt.NewReader() },
	".yml":      func() format.DataFormatReader { return yamlfmt.NewReader() },
}

// deferredExts are recognized but need a text-extraction dependency that is
// an explicit later slice (PDF, PowerPoint presentations).
var deferredExts = map[string]bool{
	".pdf":  true,
	".ppt":  true,
	".pptx": true,
	".pptm": true,
	".ppsx": true,
	".potx": true,
}

// readersByMediaType maps allowlisted content types to reader factories,
// used when the filename carries no (known) extension.
var readersByMediaType = map[string]func() format.DataFormatReader{
	"text/html":                 newHTMLReader,
	"application/xhtml+xml":     newHTMLReader,
	"text/markdown":             func() format.DataFormatReader { return markdown.NewReader() },
	"text/plain":                func() format.DataFormatReader { return plaintext.NewReader() },
	"text/csv":                  func() format.DataFormatReader { return csvfmt.NewReader() },
	"text/tab-separated-values": func() format.DataFormatReader { return csvfmt.NewTSVReader() },
	"application/json":          func() format.DataFormatReader { return jsonfmt.NewReader() },
	"text/xml":                  func() format.DataFormatReader { return xmlfmt.NewReader() },
	"application/xml":           func() format.DataFormatReader { return xmlfmt.NewReader() },
	"application/yaml":          func() format.DataFormatReader { return yamlfmt.NewReader() },
	"application/x-yaml":        func() format.DataFormatReader { return yamlfmt.NewReader() },
	"text/yaml":                 func() format.DataFormatReader { return yamlfmt.NewReader() },
	"application/epub+zip":      func() format.DataFormatReader { return epub.NewReader() },
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": func() format.DataFormatReader { return openxml.NewReader() },
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       func() format.DataFormatReader { return openxml.NewReader() },
	"application/vnd.oasis.opendocument.text":                                 func() format.DataFormatReader { return odf.NewReader() },
	"application/vnd.oasis.opendocument.spreadsheet":                          func() format.DataFormatReader { return odf.NewReader() },
	"application/vnd.oasis.opendocument.presentation":                         func() format.DataFormatReader { return odf.NewReader() },
}

// deferredMediaTypes mirror deferredExts for content-type based selection.
var deferredMediaTypes = map[string]bool{
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.ms-powerpoint":                                             true,
}

// readerForFile picks a reader factory by extension, then content type.
func readerForFile(filename, contentType string) (func() format.DataFormatReader, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" {
		if deferredExts[ext] {
			return nil, fmt.Errorf("brandscan: %q: %s", filename, deferredExtractorMsg)
		}
		if mk, ok := readersByExt[ext]; ok {
			return mk, nil
		}
	}
	if contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
			if deferredMediaTypes[mediaType] {
				return nil, fmt.Errorf("brandscan: %q: %s", filename, deferredExtractorMsg)
			}
			if mk, ok := readersByMediaType[mediaType]; ok {
				return mk, nil
			}
		}
	}
	return nil, fmt.Errorf("brandscan: unsupported file type for %q (extension %q, content type %q)", filename, ext, contentType)
}

// newHTMLReader returns an HTML reader configured for text harvesting:
// non-translatable contextual content (noscript fallbacks, JSON data islands)
// stays buried in skeleton, and executable <script>/<style> content is always
// opaque, so none of it leaks into the extracted text.
func newHTMLReader() format.DataFormatReader {
	r := html.NewReader()
	if cfg, ok := r.Config().(*html.Config); ok {
		cfg.SetExtractNonTranslatableContent(false)
	}
	return r
}

// htmlToText extracts the visible plain text of an HTML document.
func htmlToText(ctx context.Context, uri string, data []byte) (string, error) {
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	return readerText(ctx, newHTMLReader(), doc)
}

// readerText drives a format reader over doc and concatenates the plain
// source text of every translatable block, separated by blank lines.
func readerText(ctx context.Context, r format.DataFormatReader, doc *model.RawDocument) (string, error) {
	if err := r.Open(ctx, doc); err != nil {
		return "", err
	}
	defer r.Close()
	var sb strings.Builder
	for res := range r.Read(ctx) {
		if res.Error != nil {
			return "", res.Error
		}
		if res.Part == nil {
			continue
		}
		block, ok := res.Part.Resource.(*model.Block)
		if !ok || block == nil || !block.Translatable {
			continue
		}
		text := strings.TrimSpace(block.SourceText())
		if text == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(text)
	}
	return sb.String(), nil
}
