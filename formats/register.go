package formats

import (
	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/registry"
	csvfmt "github.com/asgeirf/gokapi/formats/csv"
	"github.com/asgeirf/gokapi/formats/html"
	"github.com/asgeirf/gokapi/formats/json"
	"github.com/asgeirf/gokapi/formats/markdown"
	"github.com/asgeirf/gokapi/formats/plaintext"
	"github.com/asgeirf/gokapi/formats/po"
	"github.com/asgeirf/gokapi/formats/properties"
	"github.com/asgeirf/gokapi/formats/srt"
	"github.com/asgeirf/gokapi/formats/tmx"
	"github.com/asgeirf/gokapi/formats/vtt"
	"github.com/asgeirf/gokapi/formats/xliff"
	"github.com/asgeirf/gokapi/formats/xliff2"
	xmlfmt "github.com/asgeirf/gokapi/formats/xml"
	"github.com/asgeirf/gokapi/formats/yaml"
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

	// SRT Subtitles
	reg.RegisterReader("srt", func() format.DataFormatReader { return srt.NewReader() })
	reg.RegisterWriter("srt", func() format.DataFormatWriter { return srt.NewWriter() })

	// WebVTT Subtitles
	reg.RegisterReader("vtt", func() format.DataFormatReader { return vtt.NewReader() })
	reg.RegisterWriter("vtt", func() format.DataFormatWriter { return vtt.NewWriter() })

	// TMX (Translation Memory eXchange)
	reg.RegisterReader("tmx", func() format.DataFormatReader { return tmx.NewReader() })
	reg.RegisterWriter("tmx", func() format.DataFormatWriter { return tmx.NewWriter() })
}
