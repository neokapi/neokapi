package formats

import (
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/archive"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/formats/doxygen"
	dtdfmt "github.com/neokapi/neokapi/core/formats/dtd"
	"github.com/neokapi/neokapi/core/formats/fixedwidth"
	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/icml"
	"github.com/neokapi/neokapi/core/formats/idml"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/formats/mosestext"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/formats/paraplaintext"
	"github.com/neokapi/neokapi/core/formats/phpcontent"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/formats/pdf"
	"github.com/neokapi/neokapi/core/formats/properties"
	regexfmt "github.com/neokapi/neokapi/core/formats/regex"
	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/formats/splicedlines"
	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/formats/tex"
	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/formats/transtable"
	tsfmt "github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/formats/ttml"
	"github.com/neokapi/neokapi/core/formats/versifiedtext"
	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/registry"
)

// RegisterAll registers all built-in data formats with the given registry.
func RegisterAll(reg *registry.FormatRegistry) {
	// Plain Text
	reg.RegisterReader("plaintext", func() format.DataFormatReader { return plaintext.NewReader() })
	reg.RegisterWriter("plaintext", func() format.DataFormatWriter { return plaintext.NewWriter() })

	// HTML
	reg.RegisterReader("html", func() format.DataFormatReader { return html.NewReader() })
	reg.RegisterWriter("html", func() format.DataFormatWriter { return html.NewWriter() })

	// XML
	reg.RegisterReader("xml", func() format.DataFormatReader { return xmlfmt.NewReader() })
	reg.RegisterWriter("xml", func() format.DataFormatWriter { return xmlfmt.NewWriter() })

	// XLIFF 1.2
	reg.RegisterReader("xliff", func() format.DataFormatReader { return xliff.NewReader() })
	reg.RegisterWriter("xliff", func() format.DataFormatWriter { return xliff.NewWriter() })

	// XLIFF 2.0
	reg.RegisterReader("xliff2", func() format.DataFormatReader { return xliff2.NewReader() })
	reg.RegisterWriter("xliff2", func() format.DataFormatWriter { return xliff2.NewWriter() })

	// YAML
	reg.RegisterReader("yaml", func() format.DataFormatReader { return yaml.NewReader() })
	reg.RegisterWriter("yaml", func() format.DataFormatWriter { return yaml.NewWriter() })

	// JSON
	reg.RegisterReader("json", func() format.DataFormatReader { return json.NewReader() })
	reg.RegisterWriter("json", func() format.DataFormatWriter { return json.NewWriter() })

	// PO (GNU gettext)
	reg.RegisterReader("po", func() format.DataFormatReader { return po.NewReader() })
	reg.RegisterWriter("po", func() format.DataFormatWriter { return po.NewWriter() })

	// Java Properties
	reg.RegisterReader("properties", func() format.DataFormatReader { return properties.NewReader() })
	reg.RegisterWriter("properties", func() format.DataFormatWriter { return properties.NewWriter() })

	// Markdown
	reg.RegisterReader("markdown", func() format.DataFormatReader { return markdown.NewReader() })
	reg.RegisterWriter("markdown", func() format.DataFormatWriter { return markdown.NewWriter() })

	// CSV
	reg.RegisterReader("csv", func() format.DataFormatReader { return csvfmt.NewReader() })
	reg.RegisterWriter("csv", func() format.DataFormatWriter { return csvfmt.NewWriter() })

	// TSV (Tab-Separated Values)
	reg.RegisterReader("tsv", func() format.DataFormatReader { return csvfmt.NewTSVReader() })
	reg.RegisterWriter("tsv", func() format.DataFormatWriter { return csvfmt.NewTSVWriter() })

	// Moses Text
	reg.RegisterReader("mosestext", func() format.DataFormatReader { return mosestext.NewReader() })
	reg.RegisterWriter("mosestext", func() format.DataFormatWriter { return mosestext.NewWriter() })

	// SRT Subtitles
	reg.RegisterReader("srt", func() format.DataFormatReader { return srt.NewReader() })
	reg.RegisterWriter("srt", func() format.DataFormatWriter { return srt.NewWriter() })

	// TTML Subtitles
	reg.RegisterReader("ttml", func() format.DataFormatReader { return ttml.NewReader() })
	reg.RegisterWriter("ttml", func() format.DataFormatWriter { return ttml.NewWriter() })

	// WebVTT Subtitles
	reg.RegisterReader("vtt", func() format.DataFormatReader { return vtt.NewReader() })
	reg.RegisterWriter("vtt", func() format.DataFormatWriter { return vtt.NewWriter() })

	// TMX (Translation Memory eXchange)
	reg.RegisterReader("tmx", func() format.DataFormatReader { return tmx.NewReader() })
	reg.RegisterWriter("tmx", func() format.DataFormatWriter { return tmx.NewWriter() })

	// OpenXML (DOCX, PPTX, XLSX)
	reg.RegisterReader("openxml", func() format.DataFormatReader { return openxml.NewReader() })
	reg.RegisterWriter("openxml", func() format.DataFormatWriter { return openxml.NewWriter() })

	// DTD
	reg.RegisterReader("dtd", func() format.DataFormatReader { return dtdfmt.NewReader() })
	reg.RegisterWriter("dtd", func() format.DataFormatWriter { return dtdfmt.NewWriter() })

	// Qt TS (Qt Linguist)
	reg.RegisterReader("ts", func() format.DataFormatReader { return tsfmt.NewReader() })
	reg.RegisterWriter("ts", func() format.DataFormatWriter { return tsfmt.NewWriter() })

	// Wiki (MediaWiki/DokuWiki)
	reg.RegisterReader("wiki", func() format.DataFormatReader { return wiki.NewReader() })
	reg.RegisterWriter("wiki", func() format.DataFormatWriter { return wiki.NewWriter() })

	// TeX/LaTeX
	reg.RegisterReader("tex", func() format.DataFormatReader { return tex.NewReader() })
	reg.RegisterWriter("tex", func() format.DataFormatWriter { return tex.NewWriter() })

	// Regex
	reg.RegisterReader("regex", func() format.DataFormatReader { return regexfmt.NewReader() })
	reg.RegisterWriter("regex", func() format.DataFormatWriter { return regexfmt.NewWriter() })

	// Doxygen
	reg.RegisterReader("doxygen", func() format.DataFormatReader { return doxygen.NewReader() })
	reg.RegisterWriter("doxygen", func() format.DataFormatWriter { return doxygen.NewWriter() })

	// ICU MessageFormat
	reg.RegisterReader("messageformat", func() format.DataFormatReader { return messageformat.NewReader() })
	reg.RegisterWriter("messageformat", func() format.DataFormatWriter { return messageformat.NewWriter() })

	// PHP Content
	reg.RegisterReader("phpcontent", func() format.DataFormatReader { return phpcontent.NewReader() })
	reg.RegisterWriter("phpcontent", func() format.DataFormatWriter { return phpcontent.NewWriter() })

	// ICML (InCopy Markup Language)
	reg.RegisterReader("icml", func() format.DataFormatReader { return icml.NewReader() })
	reg.RegisterWriter("icml", func() format.DataFormatWriter { return icml.NewWriter() })

	// IDML (InDesign Markup Language)
	reg.RegisterReader("idml", func() format.DataFormatReader { return idml.NewReader() })
	reg.RegisterWriter("idml", func() format.DataFormatWriter { return idml.NewWriter() })

	// Fixed-Width Table
	reg.RegisterReader("fixedwidth", func() format.DataFormatReader { return fixedwidth.NewReader() })
	reg.RegisterWriter("fixedwidth", func() format.DataFormatWriter { return fixedwidth.NewWriter() })

	// Translation Table
	reg.RegisterReader("transtable", func() format.DataFormatReader { return transtable.NewReader() })
	reg.RegisterWriter("transtable", func() format.DataFormatWriter { return transtable.NewWriter() })

	// Paragraph Plain Text
	reg.RegisterReader("paraplaintext", func() format.DataFormatReader { return paraplaintext.NewReader() })
	reg.RegisterWriter("paraplaintext", func() format.DataFormatWriter { return paraplaintext.NewWriter() })

	// Spliced Lines
	reg.RegisterReader("splicedlines", func() format.DataFormatReader { return splicedlines.NewReader() })
	reg.RegisterWriter("splicedlines", func() format.DataFormatWriter { return splicedlines.NewWriter() })

	// Versified Text
	reg.RegisterReader("versifiedtext", func() format.DataFormatReader { return versifiedtext.NewReader() })
	reg.RegisterWriter("versifiedtext", func() format.DataFormatWriter { return versifiedtext.NewWriter() })

	// R Vignette
	reg.RegisterReader("vignette", func() format.DataFormatReader { return vignette.NewReader() })
	reg.RegisterWriter("vignette", func() format.DataFormatWriter { return vignette.NewWriter() })

	// ODF (Open Document Format)
	reg.RegisterReader("odf", func() format.DataFormatReader { return odf.NewReader() })
	reg.RegisterWriter("odf", func() format.DataFormatWriter { return odf.NewWriter() })

	// Archive (ZIP)
	reg.RegisterReader("archive", func() format.DataFormatReader { return archive.NewReader() })
	reg.RegisterWriter("archive", func() format.DataFormatWriter { return archive.NewWriter() })

	// EPUB
	reg.RegisterReader("epub", func() format.DataFormatReader { return epub.NewReader() })
	reg.RegisterWriter("epub", func() format.DataFormatWriter { return epub.NewWriter() })

	// RTF (Rich Text Format)
	reg.RegisterReader("rtf", func() format.DataFormatReader { return rtf.NewReader() })
	reg.RegisterWriter("rtf", func() format.DataFormatWriter { return rtf.NewWriter() })

	// MIF (Adobe FrameMaker)
	reg.RegisterReader("mif", func() format.DataFormatReader { return mif.NewReader() })
	reg.RegisterWriter("mif", func() format.DataFormatWriter { return mif.NewWriter() })

	// TTX (Trados TagEditor)
	reg.RegisterReader("ttx", func() format.DataFormatReader { return ttx.NewReader() })
	reg.RegisterWriter("ttx", func() format.DataFormatWriter { return ttx.NewWriter() })

	// TXML (Trados XML)
	reg.RegisterReader("txml", func() format.DataFormatReader { return txml.NewReader() })
	reg.RegisterWriter("txml", func() format.DataFormatWriter { return txml.NewWriter() })

	// PDF (extraction only)
	reg.RegisterReader("pdf", func() format.DataFormatReader { return pdf.NewReader() })
	reg.RegisterWriter("pdf", func() format.DataFormatWriter { return pdf.NewWriter() })
}
