package klf

import "github.com/neokapi/neokapi/core/model"

// SchemaVersion is the klf wire format version this package emits.
// Consumers MUST reject unknown major versions and SHOULD accept
// unknown minor versions of their major (forward-compat contract in
// RFC 0001 §Versioning).
const SchemaVersion = "1.0"

// Kind is the magic string on the root of a .klf JSON document.
const Kind = "kapi-localization-format"

// The Run-based content model is canonical in core/model. klf
// re-exports the types so downstream consumers using `klf.Run`,
// `klf.TextRun`, etc. continue to compile unchanged while
// readers/writers and tools converge on the single type in model.

type (
	PluralForm      = model.PluralForm
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

	Placeholder       = model.Placeholder
	BlockProperties   = model.BlockProperties
	BlockPreviewHints = model.BlockPreviewHints
)

// Re-exported constants so callers referencing klf.PluralOne etc.
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
)

// Block is the unit of translation tracking on the wire. Structurally
// identical to what an extractor produces; the in-memory model.Block
// is the fuller runtime type carrying skeleton, annotations, etc.
type Block struct {
	ID           string             `json:"id"`
	Hash         string             `json:"hash"`
	Translatable bool               `json:"translatable"`
	Type         BlockType          `json:"type"`
	Source       []Run              `json:"source"`
	Targets      map[LocaleID][]Run `json:"targets,omitempty"`
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
	Path         string       `json:"path"`
	SourceHash   string       `json:"sourceHash,omitempty"`
	Skeleton     *Skeleton    `json:"skeleton,omitempty"`
	Blocks       []Block      `json:"blocks"`
}

// GeneratorInfo identifies the extractor that produced a .klf.
type GeneratorInfo struct {
	ID           string   `json:"id"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// ProjectInfo identifies the project a .klf belongs to.
type ProjectInfo struct {
	ID           string   `json:"id"`
	SourceLocale LocaleID `json:"sourceLocale"`
}

// Vocabulary lists vocabulary files this .klf depends on.
type Vocabulary struct {
	Extends []string `json:"extends,omitempty"`
}

// File is the top-level shape of a .klf JSON document.
type File struct {
	SchemaVersion string        `json:"schemaVersion"`
	Kind          string        `json:"kind"`
	Created       string        `json:"created,omitempty"`
	Generator     GeneratorInfo `json:"generator"`
	Project       ProjectInfo   `json:"project"`
	Vocabulary    *Vocabulary   `json:"vocabulary,omitempty"`
	Documents     []Document    `json:"documents"`
}
