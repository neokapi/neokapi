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
	"github.com/neokapi/neokapi/core/formats/archive"
	"github.com/neokapi/neokapi/core/formats/asciidoc"
	"github.com/neokapi/neokapi/core/formats/audio"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/formats/designtokens"
	"github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/formats/docling"
	"github.com/neokapi/neokapi/core/formats/epub"
	execfmt "github.com/neokapi/neokapi/core/formats/exec"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/i18next"
	imagefmt "github.com/neokapi/neokapi/core/formats/image"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/jsx"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/mdx"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/formats/tmx"
	tsfmt "github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/formats/video"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/kbf"
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

	// Image — a localizable raster asset. Always emits the image as a Media part
	// (whole-image localization); with ocr/layout enabled and the kapi-vision
	// plugin installed, also extracts text + structure. PNG/JPEG/GIF/BMP/TIFF are
	// matched by magic-byte prefix; WebP and the ISOBMFF still images HEIC/HEIF
	// and AVIF are matched by imagefmt.Sniff (their markers sit past offset 0 and
	// the RIFF/ftyp container prefixes are shared with audio/video).
	reg.RegisterReader("image",
		func() format.DataFormatReader { return imagefmt.NewReader() },
		format.FormatSignature{
			MIMETypes: []string{
				"image/png", "image/jpeg", "image/gif", "image/bmp", "image/tiff",
				"image/webp", "image/heic", "image/heif", "image/avif",
			},
			Extensions: []string{
				".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff",
				".webp", ".heic", ".heif", ".avif",
			},
			MagicBytes: [][]byte{
				{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
				{0xff, 0xd8, 0xff},
				[]byte("GIF87a"), []byte("GIF89a"),
				[]byte("BM"),
				{0x49, 0x49, 0x2a, 0x00},
				{0x4d, 0x4d, 0x00, 0x2a},
			},
			Sniff: imagefmt.Sniff,
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
	// `kapi convert ... --to yaml` produces a STRUCTURAL block-array (the catalog
	// yaml writer is the i18n round-trip format). yaml-catalog is the explicit
	// opt-in to the key→value catalog writer.
	reg.RegisterAlias("yaml-catalog", "yaml")

	// JSON
	reg.RegisterReader("json",
		func() format.DataFormatReader { return json.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/json"},
			Extensions: []string{".json"},
		}, "JSON")
	reg.RegisterWriter("json", func() format.DataFormatWriter { return json.NewWriter() })
	registerSchemaAndDecoder(o, reg, "json", func() format.DataFormatReader { return json.NewReader() })
	// `kapi convert ... --to json` produces a STRUCTURAL block-array (the catalog
	// json writer is the i18n round-trip format). json-catalog is the explicit
	// opt-in to the key→value catalog writer.
	reg.RegisterAlias("json-catalog", "json")

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

	// AsciiDoc (.adoc/.asciidoc/.adfm/.asc) — Eclipse AsciiDoc Language. A
	// lightweight prose markup format with no Okapi counterpart (harvest
	// cohort). Detection is by the unique extensions and the distinct
	// text/asciidoc MIME; it claims no shared MIME so it never steals plaintext.
	reg.RegisterReader("asciidoc",
		func() format.DataFormatReader { return asciidoc.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/asciidoc"},
			Extensions: []string{".adoc", ".asciidoc", ".adfm", ".asc"},
		}, "AsciiDoc")
	reg.RegisterWriter("asciidoc", func() format.DataFormatWriter { return asciidoc.NewWriter() })
	registerSchemaAndDecoder(o, reg, "asciidoc", func() format.DataFormatReader { return asciidoc.NewReader() })

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

	// Audio — transcribed to timing-anchored Blocks when the kapi-asr plugin is
	// available (AD-030); otherwise emitted as a Media asset. The writer emits the
	// (possibly per-locale replacement) audio bytes — the whole-audio
	// localization sink (a binary asset, see project.IsBinaryAssetFormat).
	reg.RegisterReader("audio",
		func() format.DataFormatReader { return audio.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"audio/wav", "audio/mpeg", "audio/mp4", "audio/flac", "audio/ogg"},
			Extensions: []string{".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus"},
			MagicBytes: [][]byte{[]byte("RIFF"), []byte("ID3"), []byte("OggS"), []byte("fLaC")},
		}, "Audio")
	reg.RegisterWriter("audio", func() format.DataFormatWriter { return audio.NewWriter() })

	// Video — demuxed (ffmpeg) into an audio track (→ kapi-asr) and sampled
	// frames (→ kapi-vision OCR), each a child Layer (AD-030). The writer emits
	// the (possibly per-locale replacement) video bytes — the whole-video
	// localization sink (a binary asset, see project.IsBinaryAssetFormat).
	reg.RegisterReader("video",
		func() format.DataFormatReader { return video.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"video/mp4", "video/quicktime", "video/x-matroska", "video/webm"},
			Extensions: []string{".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi"},
		}, "Video")
	reg.RegisterWriter("video", func() format.DataFormatWriter { return video.NewWriter() })

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

	// TeX/LaTeX

	// Regex

	// Doxygen

	// ICU MessageFormat
	reg.RegisterReader("messageformat",
		func() format.DataFormatReader { return messageformat.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"text/x-messageformat"},
			Extensions: []string{".mf", ".messageformat"},
		}, "ICU MessageFormat")
	reg.RegisterWriter("messageformat", func() format.DataFormatWriter { return messageformat.NewWriter() })

	// PHP Content

	// ICML (InCopy Markup Language)

	// IDML (InDesign Markup Language)

	// Fixed-Width Table

	// Translation Table

	// Paragraph Plain Text

	// Spliced Lines

	// Versified Text

	// Vignette CMS export/import XML (the `vgnexport` tool's output).
	// Detection is sniff-based because the file uses the generic .xml
	// extension and MIME — claiming text/xml unconditionally would
	// override the generic XML reader. The Sniff hook fires only when
	// the document carries the Vignette importexport namespace or an
	// importContentInstance element, leaving generic XML files routed
	// to the xml reader.

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

	// Archive (ZIP / TAR / TAR.GZ) — READ-ONLY. Surfaces the translatable
	// content of each recognised entry (JSON, Markdown, HTML, XML, .po, …) as a
	// child Layer for `kapi inspect` / analysis; binary, skeleton-bound, nested,
	// and unrecognised entries are listed as Data. There is no archive WRITER:
	// localizing a container is the container binding (AD-026 §6) — each entry is
	// run as its own file with a real reader/writer + skeleton round-trip and the
	// results are repacked over core/container. The reader captures the registry
	// (it is both the SubfilterResolver and the Detector). Detection is by the
	// unique .zip/.tar/.tgz/.tar.gz extensions; the shared ZIP/gzip magic is
	// ambiguous (OOXML/ODF/IDML/EPUB share the PK prefix), so a below-default
	// priority lets those specific formats win the content-sniff disambiguation —
	// a plain ZIP resolves here.
	reg.RegisterReader("archive",
		func() format.DataFormatReader { return archive.NewReader(reg) },
		format.FormatSignature{
			MIMETypes: []string{
				"application/zip", "application/x-tar",
				"application/gzip", "application/x-gzip",
			},
			Extensions: []string{".zip", ".tar", ".tgz", ".tar.gz"},
			MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}, {0x1f, 0x8b}},
		}, "Archive (ZIP/TAR)")
	reg.SetFormatPriority("archive", format.DefaultBuiltInPriority-10)

	// RTF (Rich Text Format)

	// MIF (Adobe FrameMaker)

	// TTX (Trados TagEditor)

	// TXML (Trados XML)

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

	// KBF — Kapi Bundle Format, registered under the id "kbf"
	// (jsx.FormatName).
	reg.RegisterReader(registry.FormatID(jsx.FormatName),
		func() format.DataFormatReader { return jsx.NewReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/vnd.neokapi.kbf+json"},
			Extensions: []string{".kbf.json"},
			Sniff: func(data []byte) bool {
				return bytes.Contains(data, []byte(`"`+kbf.Kind+`"`))
			},
		}, "Kapi Bundle Format (KBF)")
	reg.RegisterWriter(registry.FormatID(jsx.FormatName), func() format.DataFormatWriter { return jsx.NewWriter() })

	// PDF is read-only and provided out-of-core: on native builds by the
	// kapi-pdfium plugin (cgo + PDFium, crash-isolated in a subprocess), and on
	// browser/js builds by the in-process PDFium-wasm bridge. registerPDF is the
	// build-tagged seam — a no-op on native (the plugin registers at runtime),
	// the wasm reader on js. See core/formats/register_pdf_*.go.
	registerPDF(reg)

	// sourcecode is read-only and provided out-of-core by the kapi-sourcecode
	// plugin (cgo + tree-sitter grammars). Same seam as PDF above, minus the
	// browser path: the grammars cannot link into wasm at all.
	registerSourceCode(reg)

	// The content shape each format carries, once every id above exists.
	RegisterFamilies(reg)
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
