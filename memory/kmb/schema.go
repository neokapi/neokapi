package kmb

import "github.com/neokapi/neokapi/core/model"

// SchemaVersion is the kmb wire format version this package emits. Follows
// the same MAJOR.MINOR forward-compatibility contract as core/kbf: a consumer
// must reject an unknown major and should accept unknown minors of a known
// major.
const SchemaVersion = "1.0"

// Kind is the magic string on the root of a kmb document.
const Kind = "kapi-memory"

// File is the top-level shape of a kmb document: an envelope plus the full
// set of content-memory entries and the import sessions their origins
// reference.
type File struct {
	SchemaVersion  string          `json:"schemaVersion"`
	Kind           string          `json:"kind"`
	Created        string          `json:"created,omitempty"`
	Generator      *GeneratorInfo  `json:"generator,omitempty"`
	Entries        []Entry         `json:"entries"`
	ImportSessions []ImportSession `json:"importSessions,omitempty"`
}

// GeneratorInfo identifies the tool that produced the file.
type GeneratorInfo struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// Entry is the wire form of memory.Entry. It is a multilingual entry:
// peer variants keyed by locale, with no authoritative "source" at this layer.
// Variant content reuses the canonical model.Run serialization (the same runs
// core/kbf emits), so inline codes, placeholders, and plural/select constructs
// survive byte-for-byte.
type Entry struct {
	ID          string                 `json:"id"`
	ProjectID   string                 `json:"projectId,omitempty"`
	HintSrcLang string                 `json:"hintSrcLang,omitempty"`
	Variants    map[string][]model.Run `json:"variants"`
	Entities    []EntityMapping        `json:"entities,omitempty"`
	Properties  map[string]string      `json:"properties,omitempty"`
	Origins     []Origin               `json:"origins,omitempty"`
	Note        string                 `json:"note,omitempty"`
	// Point is where the answer was approved, and Unit is the block it was
	// approved for. Both are what make the bundle the truth rather than an
	// approximation of it: without them a wipe-and-reseed returns every entry
	// as approved nowhere, for no block, which reads to the matcher as a corpus
	// of ad-hoc additions and quietly disables the disambiguation both exist to
	// provide.
	Point   string `json:"point,omitempty"`
	Unit    string `json:"unit,omitempty"`
	Created string `json:"created,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// EntityMapping is the wire form of memory.EntityMapping: a named entity
// tracked across all variants. ConceptID optionally cross-links to a terms store
// concept — the enrichment TMX cannot represent.
type EntityMapping struct {
	PlaceholderID string                 `json:"placeholderId"`
	Type          string                 `json:"type,omitempty"`
	Values        map[string]EntityValue `json:"values,omitempty"`
	ConceptID     string                 `json:"conceptId,omitempty"`
}

// EntityValue is a per-locale entity value and its position within the variant.
type EntityValue struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Origin records where an entry came from. AddedAt is RFC 3339.
type Origin struct {
	Source    string `json:"source,omitempty"`
	Key       string `json:"key,omitempty"`
	Reference string `json:"reference,omitempty"`
	AddedAt   string `json:"addedAt,omitempty"`
	AddedBy   string `json:"addedBy,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	// ContextFP is the governing context this answer stands under: the voice
	// guidance and term rules in force when it was produced or approved, as the
	// producer stamps it on a target and the decision record keeps it beside
	// the decision (state.UnitState.GoverningFingerprint). The absorber copies
	// it from whichever of the two carries it, so an answer read out of a
	// format with no slot for provenance still arrives with one when the
	// project's record has it.
	//
	// It travels with the bundle because the bundle is the truth for the
	// answer, and a re-seed carries it back onto the decision record for the
	// unit it answers, so a checkout whose record never held it gains it from
	// the bundle. Empty when the answer was produced under no governing
	// context, or before anything recorded one: such an answer can be used and
	// cannot be judged against the context in force.
	ContextFP string `json:"contextFp,omitempty"`
}

// ImportSession is the wire form of memory.ImportSession — per-file metadata
// captured once at import time, referenced by Origin.SessionID. ImportedAt is
// RFC 3339.
type ImportSession struct {
	ID               string            `json:"id"`
	FileKey          string            `json:"fileKey,omitempty"`
	FileHash         string            `json:"fileHash,omitempty"`
	FileSizeBytes    int64             `json:"fileSizeBytes,omitempty"`
	ImportedAt       string            `json:"importedAt,omitempty"`
	ImportedBy       string            `json:"importedBy,omitempty"`
	ToolName         string            `json:"toolName,omitempty"`
	ToolVersion      string            `json:"toolVersion,omitempty"`
	SegType          string            `json:"segType,omitempty"`
	AdminLang        string            `json:"adminLang,omitempty"`
	SrcLang          string            `json:"srcLang,omitempty"`
	DataType         string            `json:"dataType,omitempty"`
	OriginalFormat   string            `json:"originalFormat,omitempty"`
	OriginalEncoding string            `json:"originalEncoding,omitempty"`
	EntryCount       int               `json:"entryCount,omitempty"`
	Properties       map[string]string `json:"properties,omitempty"`
}
