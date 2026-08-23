package formats

import "github.com/neokapi/neokapi/core/registry"

// Built-in format identifiers. These constants are the canonical names used
// for format registration and lookup. Using these instead of raw strings
// provides compile-time safety.
const (
	Plaintext     registry.FormatID = "plaintext"
	HTML          registry.FormatID = "html"
	XML           registry.FormatID = "xml"
	XLIFF         registry.FormatID = "xliff"
	XLIFF2        registry.FormatID = "xliff2"
	YAML          registry.FormatID = "yaml"
	JSON          registry.FormatID = "json"
	PO            registry.FormatID = "po"
	Properties    registry.FormatID = "properties"
	Markdown      registry.FormatID = "markdown"
	CSV           registry.FormatID = "csv"
	TSV           registry.FormatID = "tsv"
	SRT           registry.FormatID = "srt"
	VTT           registry.FormatID = "vtt"
	TMX           registry.FormatID = "tmx"
	OpenXML       registry.FormatID = "openxml"
	TS            registry.FormatID = "ts"
	MessageFormat registry.FormatID = "messageformat"
	ODF           registry.FormatID = "odf"
	EPUB          registry.FormatID = "epub"
	XCStrings     registry.FormatID = "xcstrings"
	ARB           registry.FormatID = "arb"
	RESX          registry.FormatID = "resx"
	AndroidXML    registry.FormatID = "androidxml"
	AppleStrings  registry.FormatID = "applestrings"
	I18next       registry.FormatID = "i18next"
	DesignTokens  registry.FormatID = "designtokens"
	MDX           registry.FormatID = "mdx"
	ASCIIDoc      registry.FormatID = "asciidoc"
	// SourceCode is provided out-of-core by kapi-sourcecode. It is named
	// `sourcecode` rather than `source` because `source` already names the
	// source-vs-target axis (terms_source, source locale, source text).
	SourceCode registry.FormatID = "sourcecode"
)
