package contextscan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// MaxFileBytes caps an uploaded brand file at 10 MiB.
const MaxFileBytes = 10 << 20

// ExtractFile converts an uploaded brand file into plain text by running it
// through the matching framework format reader and concatenating the source
// text of its translatable blocks. The reader is resolved through the shared
// format registry — by extension first, then content type — so contextscan reads
// exactly the formats the rest of kapi reads, including any a plugin adds.
// Files larger than MaxFileBytes and types the registry has no reader for are
// rejected with an error.
func ExtractFile(reg *registry.FormatRegistry, data []byte, filename, contentType string) (string, error) {
	if int64(len(data)) > MaxFileBytes {
		return "", fmt.Errorf("contextscan: file %q too large: %d bytes (max %d)", filename, len(data), MaxFileBytes)
	}
	reader, err := readerForFile(reg, filename, contentType)
	if err != nil {
		return "", err
	}
	uri := filename
	if uri == "" {
		uri = "contextscan-upload"
	}
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	text, err := readerText(context.Background(), reader, doc)
	if err != nil {
		return "", fmt.Errorf("contextscan: extract %q: %w", filename, err)
	}
	return text, nil
}

// CheckFileSupported reports whether ExtractFile can handle a file of this
// name/content type, without reading any bytes or building a reader. It returns
// nil when the registry resolves a format and the same descriptive error
// ExtractFile's detection would otherwise, so upload endpoints can validate
// (and explain skips) before storing anything.
func CheckFileSupported(reg *registry.FormatRegistry, filename, contentType string) error {
	_, err := detectFormat(reg, filename, contentType)
	return err
}

// detectFormat resolves a registered format id from the registry by extension
// first, then content type — the same precedence the file connector uses.
func detectFormat(reg *registry.FormatRegistry, filename, contentType string) (string, error) {
	detector := reg.Detector()
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" {
		if name, err := detector.DetectByExtension(ext); err == nil {
			return name, nil
		}
	}
	if contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
			if name, err := detector.DetectByMIME(mediaType); err == nil {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("contextscan: unsupported file type for %q (extension %q, content type %q)", filename, ext, contentType)
}

// readerForFile resolves the format through the registry and builds a reader
// configured for text harvesting.
func readerForFile(reg *registry.FormatRegistry, filename, contentType string) (format.DataFormatReader, error) {
	fmtName, err := detectFormat(reg, filename, contentType)
	if err != nil {
		return nil, err
	}
	reader, err := reg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return nil, fmt.Errorf("contextscan: no reader for %q (%q): %w", fmtName, filename, err)
	}
	configureForHarvest(reader)
	return reader, nil
}

// configureForHarvest applies the format-specific harvesting settings contextscan
// needs. For HTML that means non-translatable contextual content (noscript
// fallbacks, JSON data islands) stays in skeleton and never joins the harvested
// text; readers without such settings are left untouched.
func configureForHarvest(r format.DataFormatReader) {
	if cfg, ok := r.Config().(*html.Config); ok {
		cfg.SetExtractNonTranslatableContent(false)
	}
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
