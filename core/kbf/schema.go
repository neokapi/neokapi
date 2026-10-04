package kbf

import "github.com/neokapi/neokapi/core/model"

// Anchor constructors, re-exported with the vocabulary above.
var (
	BlockAnchor         = model.BlockAnchor
	RunAnchor           = model.RunAnchor
	FormAnchor          = model.FormAnchor
	SpanAnchor          = model.SpanAnchor
	RangeAnchor         = model.RangeAnchor
	RangeAnchorForBytes = model.RangeAnchorForBytes
)

// The anchor vocabulary, so a KBF caller names kinds and path steps without
// reaching past the format package for them.
const (
	AnchorBlock = model.AnchorBlock
	AnchorRun   = model.AnchorRun
	AnchorRange = model.AnchorRange
	AnchorForm  = model.AnchorForm

	StepIndex  = model.StepIndex
	StepPlural = model.StepPlural
	StepSelect = model.StepSelect
)

// SchemaVersion is the kbf wire format version this package emits.
// Consumers MUST reject unknown major versions and SHOULD accept
// unknown minor versions of their major (forward-compat contract in
// RFC 0001 §Versioning).
const SchemaVersion = "2.0"

// SchemaVersionV1 is the first schema, in which a block carried `source` runs
// beside `targets` keyed by locale. [Unmarshal] and [Decode] read a file in it
// as the editions it describes and stamp it [SchemaVersion], so a caller sees
// one shape whichever schema a file was written in.
const SchemaVersionV1 = "1.0"

// Kind is the magic string on the root of a .kbf.json document, and the only
// one [Marshal] writes.
const Kind = "kapi-bundle"

// KindI18nReact is the root kind stamped by @neokapi/i18n-react 1.2.3, the
// build on npm that `npm install -D @neokapi/i18n-react` served before the
// bundle format took its current name. A catalog it extracted is a current
// bundle in every other byte, and a reader that refuses the string refuses the
// file: `kapi pseudo-translate i18n/` on a tree that package wrote fails on
// the envelope check with the content sitting right there. Whoever installed
// 1.2.3 has such files on disk, so [Unmarshal] and the format sniffer take
// them and the writer restamps them as [Kind].
const KindI18nReact = "kapi-localization-format"

// ReadableKinds are the root kinds [Unmarshal] accepts, current spelling first.
var ReadableKinds = []string{Kind, KindI18nReact}

// The Run-based content model is canonical in core/model. kbf
// re-exports the types so downstream consumers using `kbf.Run`,
// `kbf.TextRun`, etc. continue to compile unchanged while
// readers/writers and tools converge on the single type in model.

type (
	PluralForm      = model.PluralForm
	Anchor          = model.Anchor
	AnchorKind      = model.AnchorKind
	RunPos          = model.RunPos
	RunPath         = model.RunPath
	RunPathStep     = model.RunPathStep
	RunPathStepKind = model.RunPathStepKind
	PlaceholderKind = model.PlaceholderKind
	LocaleID        = string
	RunConstraints  = model.RunConstraints

	TextRun        = model.TextRun
	PlaceholderRun = model.PlaceholderRun
	PcOpenRun      = model.PcOpenRun
	PcCloseRun     = model.PcCloseRun
	SubRun         = model.SubRun
	PluralRun      = model.PluralRun
	SelectRun      = model.SelectRun
	Run            = model.Run
	// Origin is the model's own provenance record, re-exported so a bundle
	// carries exactly what a producer stamped rather than a projection of it
	// that could drift.
	Origin = model.Origin

	Placeholder       = model.Placeholder
	BlockProperties   = model.BlockProperties
	BlockPreviewHints = model.BlockPreviewHints
)

// Re-exported constants so callers referencing kbf.PluralOne etc.
// don't need to know about the alias to core/model.
const (
	PluralZero  = model.PluralZero
	PluralOne   = model.PluralOne
	PluralTwo   = model.PluralTwo
	PluralFew   = model.PluralFew
	PluralMany  = model.PluralMany
	PluralOther = model.PluralOther

	PlaceholderVariable = model.PlaceholderVariable
	PlaceholderElement  = model.PlaceholderElement
	PlaceholderNode     = model.PlaceholderNode
	PlaceholderICUPivot = model.PlaceholderICUPivot
)

// BlockType is the coarse classification of a Block per RFC 0001.
// Kept distinct from the free-form model.Block.Type string so the
// RFC enum values don't clash with format-specific type tags.
type BlockType string

const (
	BlockTypeJSXElement   BlockType = BlockType(model.BlockContentJSXElement)
	BlockTypeJSXAttribute BlockType = BlockType(model.BlockContentJSXAttribute)
	BlockTypeJST          BlockType = BlockType(model.BlockContentJST)
)

// Block is the unit of translation tracking on the wire. Structurally
// identical to what an extractor produces; the in-memory model.Block
// is the fuller runtime type carrying skeleton, annotations, etc.
type Block struct {
	ID           string    `json:"id"`
	Hash         string    `json:"hash"`
	Translatable bool      `json:"translatable"`
	Type         BlockType `json:"type"`
	// Editions holds the block's content as peer editions, each under the
	// text form of its edition key: the edition the block was read in under
	// SourceEdition, every translation and other edition (a tone, a channel)
	// under its own key (edition.go). Each edition carries its status, its
	// provenance and the edition it was derived from beside its runs, so an
	// answer re-seeded from a bundle arrives with the context that governed
	// it, and a reader can judge it.
	Editions map[string]Edition `json:"editions"`
	// Unlabelled is a translation a reader filed under no language, such as an
	// xcstrings localization keyed by the empty string. The empty key names
	// the source, so a block carries it apart. Omitted when there is none.
	Unlabelled   *Edition           `json:"unlabelled,omitempty"`
	Placeholders []Placeholder      `json:"placeholders"`
	Properties   BlockProperties    `json:"properties"`
	Preview      *BlockPreviewHints `json:"preview,omitempty"`
}

// DocumentType discriminates the source format of a document.
type DocumentType string

const (
	DocumentTypeJSX DocumentType = "jsx"
)

// Skeleton is the reference to an opaque skeleton payload.
type Skeleton struct {
	Ref    string `json:"ref,omitempty"`
	Inline string `json:"inline,omitempty"`
}

// Document is one source file's worth of extracted content.
type Document struct {
	ID           string       `json:"id"`
	DocumentType DocumentType `json:"documentType"`
	// Path is the source file, relative to the root the extractor was given
	// (--source-root, defaulting to its working directory). It is both the
	// document's identity — it spells ID and every block id under it — and the
	// path a reader is shown, which is why the root is declared rather than
	// incidental: a path relative to wherever a build happened to run names a
	// file only to someone who knows where that was, and a surface holding the
	// catalog does not.
	Path       string    `json:"path"`
	SourceHash string    `json:"sourceHash,omitempty"`
	Skeleton   *Skeleton `json:"skeleton,omitempty"`
	Blocks     []Block   `json:"blocks"`
}

// GeneratorInfo identifies the extractor that produced a .kbf.json.
type GeneratorInfo struct {
	ID           string   `json:"id"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// ProjectInfo identifies the project a .kbf.json belongs to.
type ProjectInfo struct {
	ID string `json:"id"`
	// SourceLocale is the language of the edition every block was read in,
	// the one each holds under SourceEdition.
	SourceLocale LocaleID `json:"sourceLocale"`
}

// Vocabulary lists vocabulary files this .kbf.json depends on.
type Vocabulary struct {
	Extends []string `json:"extends,omitempty"`
}

// File is the top-level shape of a .kbf.json document.
type File struct {
	SchemaVersion string        `json:"schemaVersion"`
	Kind          string        `json:"kind"`
	Created       string        `json:"created,omitempty"`
	Generator     GeneratorInfo `json:"generator"`
	Project       ProjectInfo   `json:"project"`
	Vocabulary    *Vocabulary   `json:"vocabulary,omitempty"`
	Documents     []Document    `json:"documents"`
}
