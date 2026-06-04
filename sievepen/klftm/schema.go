package klftm

import "github.com/neokapi/neokapi/core/model"

// SchemaVersion is the klftm wire format version this package emits. Follows
// the same MAJOR.MINOR forward-compatibility contract as core/klf: a consumer
// must reject an unknown major and should accept unknown minors of a known
// major.
const SchemaVersion = "1.0"

// Kind is the magic string on the root of a klftm document.
const Kind = "kapi-tm-format"

// File is the top-level shape of a klftm document: an envelope plus the full
// set of translation-memory entries and the import sessions their origins
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

// Entry is the wire form of sievepen.TMEntry. It is a multilingual entry:
// peer variants keyed by locale, with no authoritative "source" at this layer.
// Variant content reuses the canonical model.Run serialization (the same runs
// core/klf emits), so inline codes, placeholders, and plural/select constructs
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
	Created     string                 `json:"created,omitempty"`
	Updated     string                 `json:"updated,omitempty"`
}

// EntityMapping is the wire form of sievepen.EntityMapping: a named entity
// tracked across all variants. ConceptID optionally cross-links to a termbase
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
}

// ImportSession is the wire form of sievepen.ImportSession — per-file metadata
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
