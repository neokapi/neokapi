package formats

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/formats/applestrings"
	"github.com/neokapi/neokapi/core/formats/arb"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/formats/designtokens"
	"github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/formats/docling"
	"github.com/neokapi/neokapi/core/formats/doxygen"
	dtdfmt "github.com/neokapi/neokapi/core/formats/dtd"
	"github.com/neokapi/neokapi/core/formats/epub"
	execfmt "github.com/neokapi/neokapi/core/formats/exec"
	"github.com/neokapi/neokapi/core/formats/fixedwidth"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/i18next"
	"github.com/neokapi/neokapi/core/formats/icml"
	"github.com/neokapi/neokapi/core/formats/idml"
	imagefmt "github.com/neokapi/neokapi/core/formats/image"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/jsx"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/mdx"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/formats/mosestext"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/formats/paraplaintext"
	"github.com/neokapi/neokapi/core/formats/phpcontent"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/formats/properties"
	regexfmt "github.com/neokapi/neokapi/core/formats/regex"
	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/formats/splicedlines"
	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/formats/tex"
	"github.com/neokapi/neokapi/core/formats/tmx"
	"github.com/neokapi/neokapi/core/formats/transtable"
	tsfmt "github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/formats/ttml"
	"github.com/neokapi/neokapi/core/formats/ttx"
	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/formats/versifiedtext"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/registry"
)

// RegisterOptions configures optional registries populated during RegisterAll.
type RegisterOptions struct {
	SchemaReg *schema.SchemaRegistry
	ConfigReg *config.Registry
}

// RegisterAll registers all built-in data formats with the given registry.
// No reader or writer instances are created during registration — all metadata
// (signatures, display names) is provided as static data.
//
// If opts is provided, schemas and config decoders are also registered in a
// single pass, eliminating the need for separate CollectNativeSchemas and
// CollectNativeDecoders calls.
func RegisterAll(reg *registry.FormatRegistry, opts ...RegisterOptions) {
	var o RegisterOptions
	if len(opts) > 0 {
		o = opts[0]
	}

	// Plain Text
	reg.RegisterReader("plaintext",
		func() format.DataFormatReader { return plaintext.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/plain"},
			Extensions: []string{".txt", ".text"},
		}, "Plain Text")
	reg.RegisterWriter("plaintext", func() format.DataFormatWriter { return plaintext.NewWriter() })
	registerSchemaAndDecoder(o, reg, "plaintext", func() format.DataFormatReader { return plaintext.NewReader() })

	// Image (PNG/JPEG) — a localizable raster asset. Always emits the image as a
	// Media part (whole-image localization); with ocr/layout enabled and the
	// kapi-vision plugin installed, also extracts text + structure.
	reg.RegisterReader("image",
		func() format.DataFormatReader { return imagefmt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"image/png", "image/jpeg"},
			Extensions: []string{".png", ".jpg", ".jpeg"},
			MagicBytes: [][]byte{
				{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
				{0xff, 0xd8, 0xff},
			},
		}, "Image")
	// The writer emits the (possibly localized) image bytes — the whole-image
	// localization sink, e.g. pseudo-localized variants.
	reg.RegisterWriter("image", func() format.DataFormatWriter { return imagefmt.NewWriter() })

	// HTML
	reg.RegisterReader("html",
		func() format.DataFormatReader { return html.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/html", "application/xhtml+xml"},
			Extensions: []string{".html", ".htm", ".xhtml"},
			MagicBytes: [][]byte{[]byte("<!DOCTYPE"), []byte("<!doctype"), []byte("<html"), []byte("<HTML")},
		}, "HTML")
	reg.RegisterWriter("html", func() format.DataFormatWriter { return html.NewWriter() })
	registerSchemaAndDecoder(o, reg, "html", func() format.DataFormatReader { return html.NewReader() })

	// XML
	reg.RegisterReader("xml",
		func() format.DataFormatReader { return xmlfmt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/xml", "application/xml"},
			Extensions: []string{".xml"},
			MagicBytes: [][]byte{[]byte("<?xml")},
		}, "XML")
	reg.RegisterWriter("xml", func() format.DataFormatWriter { return xmlfmt.NewWriter() })
	registerSchemaAndDecoder(o, reg, "xml", func() format.DataFormatReader { return xmlfmt.NewReader() })

	// DocLang (LF AI & Data open standard, v0.6). A DocLang file is named
	// "<name>.dclg.xml", but filepath.Ext only sees ".xml", so doclang co-claims
	// the ".xml" extension alongside the generic XML reader and disambiguates by
	// the precise "<doclang" content sniff. A below-default priority guarantees a
	// plain .xml never resolves to doclang when the sniff misses (the generic XML
	// reader wins the extension/MIME fallback).
	reg.RegisterReader("doclang",
		func() format.DataFormatReader { return doclang.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/doclang+xml"},
			Extensions: []string{".dclg.xml", ".xml"},
			Sniff:      func(data []byte) bool { return bytes.Contains(data, []byte("<doclang")) },
		}, "DocLang")
	reg.RegisterWriter("doclang", func() format.DataFormatWriter { return doclang.NewWriter() })
	reg.SetFormatPriority("doclang", format.DefaultBuiltInPriority-10)
	registerSchemaAndDecoder(o, reg, "doclang", func() format.DataFormatReader { return doclang.NewReader() })

	// DoclingDocument JSON — Docling's native lossless serialization. Read-only:
	// neokapi consumes it (re-emitting structure via DocLang or projecting to
	// Markdown/HTML). It co-claims the .json extension with the generic JSON
	// reader and disambiguates by a precise content sniff (schema_name +
	// DoclingDocument); a below-default priority guarantees a plain .json never
	// resolves to docling when the sniff misses (the JSON reader wins the
	// extension/MIME fallback).
	reg.RegisterReader("docling",
		func() format.DataFormatReader { return docling.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/json"},
			Extensions: []string{".json"},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte(`"schema_name"`)) &&
					bytes.Contains(data, []byte("DoclingDocument"))
			},
		}, "DoclingDocument JSON")
	reg.SetFormatPriority("docling", format.DefaultBuiltInPriority-10)

	// .NET RESX / .resw (Microsoft ResX 2.0). The Sniff keys on the
	// resmimetype resheader so RESX files routed without the .resx/.resw
	// extension are not claimed by the generic XML reader.
	reg.RegisterReader("resx",
		func() format.DataFormatReader { return resx.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/microsoft-resx"},
			Extensions: []string{".resx", ".resw"},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte("text/microsoft-resx"))
			},
		}, ".NET RESX")
	reg.RegisterWriter("resx", func() format.DataFormatWriter { return resx.NewWriter() })
	registerSchemaAndDecoder(o, reg, "resx", func() format.DataFormatReader { return resx.NewReader() })

	// XLIFF 1.2
	reg.RegisterReader("xliff",
		func() format.DataFormatReader { return xliff.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/xliff+xml", "application/x-xliff+xml"},
			Extensions: []string{".xlf", ".xliff"},
			Sniff: func(data []byte) bool {
				s := string(data)
				return strings.Contains(s, "<xliff") && strings.Contains(s, "urn:oasis:names:tc:xliff:document:1")
			},
		}, "XLIFF 1.2")
	reg.RegisterWriter("xliff", func() format.DataFormatWriter { return xliff.NewWriter() })
	registerSchemaAndDecoder(o, reg, "xliff", func() format.DataFormatReader { return xliff.NewReader() })

	// XLIFF 2.x (2.0 / 2.1 / 2.2 — accepted as a compatible family)
	reg.RegisterReader("xliff2",
		func() format.DataFormatReader { return xliff2.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/xliff+xml"},
			Extensions: []string{".xlf", ".xliff"},
			Sniff: func(data []byte) bool {
				s := string(data)
				if !strings.Contains(s, "<xliff") {
					return false
				}
				// Any OASIS 2.x document namespace, or any version="2.X" attr.
				return strings.Contains(s, "urn:oasis:names:tc:xliff:document:2") ||
					strings.Contains(s, `version="2.0"`) ||
					strings.Contains(s, `version="2.1"`) ||
					strings.Contains(s, `version="2.2"`)
			},
		}, "XLIFF 2.x")
	reg.RegisterWriter("xliff2", func() format.DataFormatWriter { return xliff2.NewWriter() })
	registerSchemaAndDecoder(o, reg, "xliff2", func() format.DataFormatReader { return xliff2.NewReader() })

	// YAML
	reg.RegisterReader("yaml",
		func() format.DataFormatReader { return yaml.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/yaml", "text/yaml", "application/x-yaml"},
			Extensions: []string{".yaml", ".yml"},
		}, "YAML")
	reg.RegisterWriter("yaml", func() format.DataFormatWriter { return yaml.NewWriter() })
	registerSchemaAndDecoder(o, reg, "yaml", func() format.DataFormatReader { return yaml.NewReader() })

	// JSON
	reg.RegisterReader("json",
		func() format.DataFormatReader { return json.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/json"},
			Extensions: []string{".json"},
		}, "JSON")
	reg.RegisterWriter("json", func() format.DataFormatWriter { return json.NewWriter() })
	registerSchemaAndDecoder(o, reg, "json", func() format.DataFormatReader { return json.NewReader() })

	// Apple String Catalog (.xcstrings) — Xcode 15+ JSON localization catalog.
	// Detection is primarily by the unique .xcstrings extension; the Sniff
	// disambiguates catalog content piped without an extension and avoids
	// stealing generic .json files (which lack both markers).
	reg.RegisterReader("xcstrings",
		func() format.DataFormatReader { return xcstrings.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/json"},
			Extensions: []string{".xcstrings"},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte(`"sourceLanguage"`)) &&
					bytes.Contains(data, []byte(`"strings"`))
			},
		}, "Apple String Catalog")
	reg.RegisterWriter("xcstrings", func() format.DataFormatWriter { return xcstrings.NewWriter() })
	registerSchemaAndDecoder(o, reg, "xcstrings", func() format.DataFormatReader { return xcstrings.NewReader() })

	// Flutter Application Resource Bundle (.arb) — Flutter/Dart gen-l10n JSON
	// localization. Detection is by the unique .arb extension only; the
	// shared application/json MIME is intentionally NOT advertised so MIME
	// detection still resolves to the generic json format.
	reg.RegisterReader("arb",
		func() format.DataFormatReader { return arb.NewReader() },
		format.FormatSignature{
			Extensions: []string{".arb"},
		}, "Flutter ARB")
	reg.RegisterWriter("arb", func() format.DataFormatWriter { return arb.NewWriter() })
	registerSchemaAndDecoder(o, reg, "arb", func() format.DataFormatReader { return arb.NewReader() })

	// Apple Strings (.strings) + Stringsdict (.stringsdict) — legacy Apple
	// localization; one package handles both file types. Detected by their
	// unique extensions (the regex format relinquished .strings).
	reg.RegisterReader("applestrings",
		func() format.DataFormatReader { return applestrings.NewReader() },
		format.FormatSignature{
			Extensions: []string{".strings", ".stringsdict"},
		}, "Apple Strings")
	reg.RegisterWriter("applestrings", func() format.DataFormatWriter { return applestrings.NewWriter() })
	registerSchemaAndDecoder(o, reg, "applestrings", func() format.DataFormatReader { return applestrings.NewReader() })

	// i18next / react-i18next JSON. Selected explicitly (-f i18next): claims no
	// extension or MIME because i18next files use the .json extension and
	// application/json MIME owned by the json format and cannot be reliably
	// auto-distinguished. Delegates to the json reader/writer with the i18next
	// preset plus plural/context annotation.
	reg.RegisterReader("i18next",
		func() format.DataFormatReader { return i18next.NewReader() },
		format.FormatSignature{}, "i18next JSON")
	reg.RegisterWriter("i18next", func() format.DataFormatWriter { return i18next.NewWriter() })
	registerSchemaAndDecoder(o, reg, "i18next", func() format.DataFormatReader { return i18next.NewReader() })

	// Android String Resources (res/values/strings.xml). The .xml extension and
	// XML MIME are owned by the generic xml format, so detection is Sniff-only:
	// the file must have a <resources> root carrying at least one <string>,
	// <string-array>, or <plurals>.
	reg.RegisterReader("androidxml",
		func() format.DataFormatReader { return androidxml.NewReader() },
		format.FormatSignature{
			Sniff: androidxml.Sniff,
		}, "Android String Resources")
	reg.RegisterWriter("androidxml", func() format.DataFormatWriter { return androidxml.NewWriter() })
	registerSchemaAndDecoder(o, reg, "androidxml", func() format.DataFormatReader { return androidxml.NewReader() })

	// W3C DTCG Design Tokens (.tokens / .tokens.json). Claims the unique
	// .tokens extension and Sniffs DTCG content ($value + $type); does NOT
	// claim .json or application/json (owned by the json format). Delegates to
	// the json reader/writer, extracting only $description documentation.
	reg.RegisterReader("designtokens",
		func() format.DataFormatReader { return designtokens.NewReader() },
		format.FormatSignature{
			Extensions: []string{".tokens"},
			Sniff:      designtokens.Sniff,
		}, "Design Tokens (DTCG)")
	reg.RegisterWriter("designtokens", func() format.DataFormatWriter { return designtokens.NewWriter() })
	registerSchemaAndDecoder(o, reg, "designtokens", func() format.DataFormatReader { return designtokens.NewReader() })

	// PO (GNU gettext)
	reg.RegisterReader("po",
		func() format.DataFormatReader { return po.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-gettext-translation"},
			Extensions: []string{".po", ".pot"},
		}, "PO (Gettext)")
	reg.RegisterWriter("po", func() format.DataFormatWriter { return po.NewWriter() })
	registerSchemaAndDecoder(o, reg, "po", func() format.DataFormatReader { return po.NewReader() })

	// MO (GNU gettext, binary — compiled runtime catalog). A stub reader
	// is registered purely so DetectByExtension(".mo") resolves to this
	// format and `-o file.mo` picks the MO writer. The stub errors on
	// Open — runtime consumers load MO via github.com/leonelquinteros/gotext,
	// never through the pipeline.
	reg.RegisterReader("mo",
		func() format.DataFormatReader { return mo.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-gettext-translation"},
			Extensions: []string{".mo"},
		}, "MO (Gettext, binary)")
	reg.RegisterWriter("mo", func() format.DataFormatWriter { return mo.NewWriter() })
	if o.ConfigReg != nil {
		o.ConfigReg.Register(config.FormatConfigKind("mo"), config.SpecDecoderFunc(func(spec map[string]any) (any, error) {
			c := &mo.Config{}
			c.Reset()
			if err := c.ApplyMap(spec); err != nil {
				return nil, err
			}
			return c, nil
		}))
	}

	// Java Properties
	reg.RegisterReader("properties",
		func() format.DataFormatReader { return properties.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-java-properties"},
			Extensions: []string{".properties"},
		}, "Java Properties")
	reg.RegisterWriter("properties", func() format.DataFormatWriter { return properties.NewWriter() })
	registerSchemaAndDecoder(o, reg, "properties", func() format.DataFormatReader { return properties.NewReader() })

	// Markdown
	reg.RegisterReader("markdown",
		func() format.DataFormatReader { return markdown.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/markdown", "text/x-markdown"},
			Extensions: []string{".md", ".markdown"},
		}, "Markdown")
	reg.RegisterWriter("markdown", func() format.DataFormatWriter { return markdown.NewWriter() })
	registerSchemaAndDecoder(o, reg, "markdown", func() format.DataFormatReader { return markdown.NewReader() })

	// MDX (Markdown + JSX/ESM). Unique .mdx extension — no collision. Reuses
	// the markdown reader for prose; ESM/JSX/expressions/tables are preserved
	// byte-faithfully and never translated.
	reg.RegisterReader("mdx",
		func() format.DataFormatReader { return mdx.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/mdx"},
			Extensions: []string{".mdx"},
		}, "MDX")
	reg.RegisterWriter("mdx", func() format.DataFormatWriter { return mdx.NewWriter() })
	registerSchemaAndDecoder(o, reg, "mdx", func() format.DataFormatReader { return mdx.NewReader() })

	// CSV
	reg.RegisterReader("csv",
		func() format.DataFormatReader { return csvfmt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/csv"},
			Extensions: []string{".csv"},
		}, "CSV")
	reg.RegisterWriter("csv", func() format.DataFormatWriter { return csvfmt.NewWriter() })
	registerSchemaAndDecoder(o, reg, "csv", func() format.DataFormatReader { return csvfmt.NewReader() })

	// TSV (Tab-Separated Values)
	reg.RegisterReader("tsv",
		func() format.DataFormatReader { return csvfmt.NewTSVReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/tab-separated-values"},
			Extensions: []string{".tsv"},
		}, "TSV")
	reg.RegisterWriter("tsv", func() format.DataFormatWriter { return csvfmt.NewTSVWriter() })

	// Moses Text
	reg.RegisterReader("mosestext",
		func() format.DataFormatReader { return mosestext.NewReader() },
		format.FormatSignature{
			MIMETypes: []string{"text/x-mosestext"},
		}, "Moses Text")
	reg.RegisterWriter("mosestext", func() format.DataFormatWriter { return mosestext.NewWriter() })

	// SRT Subtitles
	reg.RegisterReader("srt",
		func() format.DataFormatReader { return srt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-subrip", "text/srt"},
			Extensions: []string{".srt"},
		}, "SRT Subtitles")
	reg.RegisterWriter("srt", func() format.DataFormatWriter { return srt.NewWriter() })
	registerSchemaAndDecoder(o, reg, "srt", func() format.DataFormatReader { return srt.NewReader() })

	// TTML Subtitles
	reg.RegisterReader("ttml",
		func() format.DataFormatReader { return ttml.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/ttml+xml"},
			Extensions: []string{".ttml", ".dfxp"},
		}, "TTML Subtitles")
	reg.RegisterWriter("ttml", func() format.DataFormatWriter { return ttml.NewWriter() })
	registerSchemaAndDecoder(o, reg, "ttml", func() format.DataFormatReader { return ttml.NewReader() })

	// WebVTT Subtitles
	reg.RegisterReader("vtt",
		func() format.DataFormatReader { return vtt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/vtt"},
			Extensions: []string{".vtt"},
			MagicBytes: [][]byte{[]byte("WEBVTT")},
		}, "WebVTT")
	reg.RegisterWriter("vtt", func() format.DataFormatWriter { return vtt.NewWriter() })
	registerSchemaAndDecoder(o, reg, "vtt", func() format.DataFormatReader { return vtt.NewReader() })

	// TMX (Translation Memory eXchange)
	reg.RegisterReader("tmx",
		func() format.DataFormatReader { return tmx.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-tmx+xml"},
			Extensions: []string{".tmx"},
		}, "TMX")
	reg.RegisterWriter("tmx", func() format.DataFormatWriter { return tmx.NewWriter() })
	registerSchemaAndDecoder(o, reg, "tmx", func() format.DataFormatReader { return tmx.NewReader() })

	// OpenXML (DOCX, PPTX, XLSX)
	reg.RegisterReader("openxml",
		func() format.DataFormatReader { return openxml.NewReader() },
		format.FormatSignature{
			MIMETypes: []string{
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			},
			Extensions: []string{".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".pptx", ".pptm", ".ppsx", ".potx"},
			MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}},
		}, "Office Open XML")
	reg.RegisterWriter("openxml", func() format.DataFormatWriter { return openxml.NewWriter() })
	registerSchemaAndDecoder(o, reg, "openxml", func() format.DataFormatReader { return openxml.NewReader() })

	// DTD
	reg.RegisterReader("dtd",
		func() format.DataFormatReader { return dtdfmt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/xml-dtd"},
			Extensions: []string{".dtd"},
		}, "DTD")
	reg.RegisterWriter("dtd", func() format.DataFormatWriter { return dtdfmt.NewWriter() })

	// Qt TS
	reg.RegisterReader("ts",
		func() format.DataFormatReader { return tsfmt.NewReader() },
		format.FormatSignature{
			MIMETypes: []string{"application/x-ts", "application/x-linguist"},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte("<TS")) && bytes.Contains(data, []byte("</TS>"))
			},
		}, "Qt TS")
	reg.RegisterWriter("ts", func() format.DataFormatWriter { return tsfmt.NewWriter() })

	// Wiki (MediaWiki/DokuWiki)
	reg.RegisterReader("wiki",
		func() format.DataFormatReader { return wiki.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-wiki"},
			Extensions: []string{".wiki", ".mediawiki"},
		}, "Wiki")
	reg.RegisterWriter("wiki", func() format.DataFormatWriter { return wiki.NewWriter() })
	registerSchemaAndDecoder(o, reg, "wiki", func() format.DataFormatReader { return wiki.NewReader() })

	// TeX/LaTeX
	reg.RegisterReader("tex",
		func() format.DataFormatReader { return tex.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-tex", "text/x-tex"},
			Extensions: []string{".tex", ".latex"},
		}, "TeX/LaTeX")
	reg.RegisterWriter("tex", func() format.DataFormatWriter { return tex.NewWriter() })

	// Regex
	reg.RegisterReader("regex",
		func() format.DataFormatReader { return regexfmt.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-regex"},
			Extensions: []string{".ini", ".info", ".rls"},
		}, "Regex Extraction")
	reg.RegisterWriter("regex", func() format.DataFormatWriter { return regexfmt.NewWriter() })

	// Doxygen
	reg.RegisterReader("doxygen",
		func() format.DataFormatReader { return doxygen.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-doxygen-txt"},
			Extensions: []string{".c", ".cpp", ".h", ".java", ".m", ".py"},
		}, "Doxygen Comments")
	reg.RegisterWriter("doxygen", func() format.DataFormatWriter { return doxygen.NewWriter() })
	registerSchemaAndDecoder(o, reg, "doxygen", func() format.DataFormatReader { return doxygen.NewReader() })

	// ICU MessageFormat
	reg.RegisterReader("messageformat",
		func() format.DataFormatReader { return messageformat.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-messageformat"},
			Extensions: []string{".mf", ".messageformat"},
		}, "ICU MessageFormat")
	reg.RegisterWriter("messageformat", func() format.DataFormatWriter { return messageformat.NewWriter() })

	// PHP Content
	reg.RegisterReader("phpcontent",
		func() format.DataFormatReader { return phpcontent.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-php"},
			Extensions: []string{".php", ".phpcnt"},
		}, "PHP Content")
	reg.RegisterWriter("phpcontent", func() format.DataFormatWriter { return phpcontent.NewWriter() })
	registerSchemaAndDecoder(o, reg, "phpcontent", func() format.DataFormatReader { return phpcontent.NewReader() })

	// ICML (InCopy Markup Language)
	reg.RegisterReader("icml",
		func() format.DataFormatReader { return icml.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-icml+xml"},
			Extensions: []string{".icml", ".wcml"},
		}, "ICML (Adobe InCopy)")
	reg.RegisterWriter("icml", func() format.DataFormatWriter { return icml.NewWriter() })

	// IDML (InDesign Markup Language)
	reg.RegisterReader("idml",
		func() format.DataFormatReader { return idml.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/vnd.adobe.indesign-idml-package"},
			Extensions: []string{".idml"},
			MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}},
		}, "Adobe InDesign Markup Language")
	reg.RegisterWriter("idml", func() format.DataFormatWriter { return idml.NewWriter() })

	// Fixed-Width Table
	reg.RegisterReader("fixedwidth",
		func() format.DataFormatReader { return fixedwidth.NewReader() },
		format.FormatSignature{
			Extensions: []string{".dat", ".fixed"},
		}, "Fixed-Width")
	reg.RegisterWriter("fixedwidth", func() format.DataFormatWriter { return fixedwidth.NewWriter() })

	// Translation Table
	reg.RegisterReader("transtable",
		func() format.DataFormatReader { return transtable.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/tab-separated-values"},
			Extensions: []string{".tab", ".tsv"},
		}, "Translation Table")
	reg.RegisterWriter("transtable", func() format.DataFormatWriter { return transtable.NewWriter() })

	// Paragraph Plain Text
	reg.RegisterReader("paraplaintext",
		func() format.DataFormatReader { return paraplaintext.NewReader() },
		format.FormatSignature{}, "Paragraph Plain Text")
	reg.RegisterWriter("paraplaintext", func() format.DataFormatWriter { return paraplaintext.NewWriter() })

	// Spliced Lines
	reg.RegisterReader("splicedlines",
		func() format.DataFormatReader { return splicedlines.NewReader() },
		format.FormatSignature{}, "Spliced Lines")
	reg.RegisterWriter("splicedlines", func() format.DataFormatWriter { return splicedlines.NewWriter() })

	// Versified Text
	reg.RegisterReader("versifiedtext",
		func() format.DataFormatReader { return versifiedtext.NewReader() },
		format.FormatSignature{
			Extensions: []string{".ver"},
		}, "Versified Text")
	reg.RegisterWriter("versifiedtext", func() format.DataFormatWriter { return versifiedtext.NewWriter() })

	// Vignette CMS export/import XML (the `vgnexport` tool's output).
	// Detection is sniff-based because the file uses the generic .xml
	// extension and MIME — claiming text/xml unconditionally would
	// override the generic XML reader. The Sniff hook fires only when
	// the document carries the Vignette importexport namespace or an
	// importContentInstance element, leaving generic XML files routed
	// to the xml reader.
	reg.RegisterReader("vignette",
		func() format.DataFormatReader { return vignette.NewReader() },
		format.FormatSignature{
			Sniff: func(data []byte) bool {
				s := string(data)
				return strings.Contains(s, "vignette.com/xmlschemas/importexport") ||
					strings.Contains(s, "<importContentInstance")
			},
		}, "Vignette CMS Export")
	reg.RegisterWriter("vignette", func() format.DataFormatWriter { return vignette.NewWriter() })

	// ODF (Open Document Format)
	reg.RegisterReader("odf",
		func() format.DataFormatReader { return odf.NewReader() },
		format.FormatSignature{
			MIMETypes: []string{
				"application/vnd.oasis.opendocument.text",
				"application/vnd.oasis.opendocument.spreadsheet",
				"application/vnd.oasis.opendocument.presentation",
			},
			Extensions: []string{".odt", ".ods", ".odp", ".odg", ".odf"},
			MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}},
		}, "Open Document Format")
	reg.RegisterWriter("odf", func() format.DataFormatWriter { return odf.NewWriter() })

	// EPUB
	reg.RegisterReader("epub",
		func() format.DataFormatReader { return epub.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/epub+zip"},
			Extensions: []string{".epub"},
			MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte("application/epub+zip"))
			},
		}, "EPUB E-Book")
	reg.RegisterWriter("epub", func() format.DataFormatWriter { return epub.NewWriter() })

	// RTF (Rich Text Format)
	reg.RegisterReader("rtf",
		func() format.DataFormatReader { return rtf.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/rtf", "text/rtf"},
			Extensions: []string{".rtf"},
			MagicBytes: [][]byte{[]byte("{\\rtf")},
		}, "Rich Text Format")
	reg.RegisterWriter("rtf", func() format.DataFormatWriter { return rtf.NewWriter() })
	registerSchemaAndDecoder(o, reg, "rtf", func() format.DataFormatReader { return rtf.NewReader() })

	// MIF (Adobe FrameMaker)
	reg.RegisterReader("mif",
		func() format.DataFormatReader { return mif.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-mif", "application/vnd.mif"},
			Extensions: []string{".mif"},
			Sniff: func(data []byte) bool {
				return len(data) >= 9 && string(data[:9]) == "<MIFFile "
			},
		}, "Adobe FrameMaker MIF")
	reg.RegisterWriter("mif", func() format.DataFormatWriter { return mif.NewWriter() })
	registerSchemaAndDecoder(o, reg, "mif", func() format.DataFormatReader { return mif.NewReader() })

	// TTX (Trados TagEditor)
	reg.RegisterReader("ttx",
		func() format.DataFormatReader { return ttx.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-ttx+xml"},
			Extensions: []string{".ttx"},
			Sniff: func(data []byte) bool {
				s := string(data)
				return strings.Contains(s, "<TRADOStag")
			},
		}, "Trados TagEditor TTX")
	reg.RegisterWriter("ttx", func() format.DataFormatWriter { return ttx.NewWriter() })
	registerSchemaAndDecoder(o, reg, "ttx", func() format.DataFormatReader { return ttx.NewReader() })

	// TXML (Trados XML)
	reg.RegisterReader("txml",
		func() format.DataFormatReader { return txml.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/x-txml+xml"},
			Extensions: []string{".txml"},
			Sniff: func(data []byte) bool {
				s := string(data)
				return strings.Contains(s, "<txml")
			},
		}, "Trados XML")
	reg.RegisterWriter("txml", func() format.DataFormatWriter { return txml.NewWriter() })
	registerSchemaAndDecoder(o, reg, "txml", func() format.DataFormatReader { return txml.NewReader() })

	// Exec — declarative subprocess extractor. Registered here so
	// kapi-desktop's FormatSelect (and other UI surfaces) can list
	// it; actual execution is orchestrated by `kapi extract -p`,
	// which reads FormatSpec.Config.command from the .kapi and
	// invokes the subprocess once per collection. The registry
	// entry is a stub — opening a raw file with this reader
	// returns an instructive error.
	reg.RegisterReader(execfmt.FormatName,
		func() format.DataFormatReader { return execfmt.NewReader() },
		format.FormatSignature{},
		"Exec (subprocess extractor)")

	// KLF — Kapi Localization Format. Registered under the canonical
	// id "klf" (jsx.FormatName); the legacy id "jsx" stays a name-only
	// back-compat alias so `--format jsx` keeps resolving. The alias
	// carries no detection signature and no FormatInfo, so detection
	// and `kapi formats` always surface "klf".
	reg.RegisterReader(registry.FormatID(jsx.FormatName),
		func() format.DataFormatReader { return jsx.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/vnd.neokapi.klf+json"},
			Extensions: []string{".klf"},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte(`"kapi-localization-format"`))
			},
		}, "Kapi Localization Format (KLF)")
	reg.RegisterWriter(registry.FormatID(jsx.FormatName), func() format.DataFormatWriter { return jsx.NewWriter() })
	reg.RegisterAlias(registry.FormatID(jsx.FormatAlias), registry.FormatID(jsx.FormatName))

	// PDF is read-only and provided out-of-core: on native builds by the
	// kapi-pdfium plugin (cgo + PDFium, crash-isolated in a subprocess), and on
	// browser/js builds by the in-process PDFium-wasm bridge. registerPDF is the
	// build-tagged seam — a no-op on native (the plugin registers at runtime),
	// the wasm reader on js. See core/formats/register_pdf_*.go.
	registerPDF(reg)
}

// registerSchemaAndDecoder registers a format's schema and config decoder
// if the format implements SchemaProvider. This creates one reader instance
// per format that has a schema — only called for formats that implement it.
func registerSchemaAndDecoder(o RegisterOptions, reg *registry.FormatRegistry, name registry.FormatID, factory func() format.DataFormatReader) {
	if o.SchemaReg == nil && o.ConfigReg == nil {
		return
	}

	reader := factory()
	cfg := reader.Config()
	if cfg == nil {
		return
	}

	if o.SchemaReg != nil {
		if sp, ok := cfg.(format.SchemaProvider); ok {
			o.SchemaReg.RegisterSchema(string(name), sp.Schema())
		}
	}

	if o.ConfigReg != nil {
		kind := config.FormatConfigKind(string(name))
		if ckp, ok := cfg.(format.ConfigKindProvider); ok {
			kind = ckp.ConfigKind()
		}

		formatName := name
		o.ConfigReg.Register(kind, config.SpecDecoderFunc(func(spec map[string]any) (any, error) {
			f := reg.ReaderFactory(formatName)
			if f == nil {
				return nil, fmt.Errorf("format %q not found", formatName)
			}
			rdr := f()
			c := rdr.Config()
			if c == nil {
				return spec, nil
			}
			c.Reset()
			if err := c.ApplyMap(spec); err != nil {
				return nil, err
			}
			return c, nil
		}))
	}
}
