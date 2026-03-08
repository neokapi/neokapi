package formats

import (
	"github.com/gokapi/gokapi/core/format"
	csvfmt "github.com/gokapi/gokapi/core/formats/csv"
	"github.com/gokapi/gokapi/core/formats/doxygen"
	dtdfmt "github.com/gokapi/gokapi/core/formats/dtd"
	"github.com/gokapi/gokapi/core/formats/fixedwidth"
	"github.com/gokapi/gokapi/core/formats/html"
	"github.com/gokapi/gokapi/core/formats/icml"
	"github.com/gokapi/gokapi/core/formats/json"
	"github.com/gokapi/gokapi/core/formats/markdown"
	"github.com/gokapi/gokapi/core/formats/messageformat"
	"github.com/gokapi/gokapi/core/formats/mosestext"
	"github.com/gokapi/gokapi/core/formats/openxml"
	"github.com/gokapi/gokapi/core/formats/phpcontent"
	"github.com/gokapi/gokapi/core/formats/plaintext"
	"github.com/gokapi/gokapi/core/formats/po"
	"github.com/gokapi/gokapi/core/formats/properties"
	regexfmt "github.com/gokapi/gokapi/core/formats/regex"
	"github.com/gokapi/gokapi/core/formats/srt"
	"github.com/gokapi/gokapi/core/formats/tex"
	"github.com/gokapi/gokapi/core/formats/tmx"
	tsfmt "github.com/gokapi/gokapi/core/formats/ts"
	"github.com/gokapi/gokapi/core/formats/ttml"
	"github.com/gokapi/gokapi/core/formats/vtt"
	"github.com/gokapi/gokapi/core/formats/wiki"
	"github.com/gokapi/gokapi/core/formats/xliff"
	"github.com/gokapi/gokapi/core/formats/xliff2"
	xmlfmt "github.com/gokapi/gokapi/core/formats/xml"
	"github.com/gokapi/gokapi/core/formats/yaml"
	"github.com/gokapi/gokapi/core/registry"
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

	// Fixed-Width Columns
	reg.RegisterReader("fixedwidth", func() format.DataFormatReader { return fixedwidth.NewReader() })
	reg.RegisterWriter("fixedwidth", func() format.DataFormatWriter { return fixedwidth.NewWriter() })
}
