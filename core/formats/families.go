package formats

import "github.com/neokapi/neokapi/core/registry"

// builtInFamilies assigns every built-in format to the content shape it carries.
//
// The taxonomy and the intended membership are core/formats/constructs.yaml's
// `families:` block, which per-format vocabulary.yaml files already score
// themselves against. This table is the runtime half of the same statement, so
// a surface asks the registry what shape a document has rather than keeping its
// own list of format ids. families_test.go holds the two to each other: every
// registered format appears here exactly once, every family is one of the eight
// declared classes, and a format whose vocabulary.yaml names a family gets the
// same answer from both.
//
// Three assignments that constructs.yaml's examples do not name:
//
//   - doclang and docling carry document structure (headings, tables, reading
//     order) in an XML and a JSON envelope, so they sit with the marked-up
//     documents rather than with the key/value catalogs their file extension
//     shares.
//   - image, audio, video and archive travel as bytes: the reader emits a Media
//     part (plus whatever a plugin transcribes or OCRs from it), and the writer
//     emits the localized asset. binary-readonly is the class for a carrier
//     whose text is not the thing being written back.
//   - exec resolves as a name so a recipe and the desktop's format picker can
//     mention it; the reader refuses on Open. It carries no structure of its
//     own, which is what plain-text says.
var builtInFamilies = map[registry.FormatID]registry.FormatFamily{
	// Marked-up text documents.
	"asciidoc": registry.FamilyRichMarkup,
	"doclang":  registry.FamilyRichMarkup,
	"docling":  registry.FamilyRichMarkup,
	"epub":     registry.FamilyRichMarkup,
	"html":     registry.FamilyRichMarkup,
	"kbf":      registry.FamilyRichMarkup,
	"markdown": registry.FamilyRichMarkup,
	"mdx":      registry.FamilyRichMarkup,

	// Word-processor and DTP documents.
	"odf":     registry.FamilyOfficeDoc,
	"openxml": registry.FamilyOfficeDoc,

	// Source/target interchange.
	"tmx":    registry.FamilyBilingualInterchange,
	"ts":     registry.FamilyBilingualInterchange,
	"xliff":  registry.FamilyBilingualInterchange,
	"xliff2": registry.FamilyBilingualInterchange,

	// String-resource catalogs keyed by identifier.
	"androidxml":    registry.FamilyCatalogKeyValue,
	"applestrings":  registry.FamilyCatalogKeyValue,
	"arb":           registry.FamilyCatalogKeyValue,
	"designtokens":  registry.FamilyCatalogKeyValue,
	"i18next":       registry.FamilyCatalogKeyValue,
	"json":          registry.FamilyCatalogKeyValue,
	"messageformat": registry.FamilyCatalogKeyValue,
	"mo":            registry.FamilyCatalogKeyValue,
	"po":            registry.FamilyCatalogKeyValue,
	"properties":    registry.FamilyCatalogKeyValue,
	"resx":          registry.FamilyCatalogKeyValue,
	"xcstrings":     registry.FamilyCatalogKeyValue,
	"yaml":          registry.FamilyCatalogKeyValue,

	// Timed text.
	"srt": registry.FamilySubtitleTimedText,
	"vtt": registry.FamilySubtitleTimedText,

	// Unmarked text.
	"exec":      registry.FamilyPlainText,
	"plaintext": registry.FamilyPlainText,

	// Structured data and configuration carriers.
	"csv": registry.FamilyDataConfig,
	"tsv": registry.FamilyDataConfig,
	"xml": registry.FamilyDataConfig,

	// Byte carriers.
	"archive": registry.FamilyBinaryReadOnly,
	"audio":   registry.FamilyBinaryReadOnly,
	"image":   registry.FamilyBinaryReadOnly,
	"video":   registry.FamilyBinaryReadOnly,
}

// RegisterFamilies records each built-in format's content shape on the registry.
// RegisterAll calls it once every reader and writer is in place.
func RegisterFamilies(reg *registry.FormatRegistry) {
	for id, family := range builtInFamilies {
		reg.SetFormatFamily(id, family)
	}
}
