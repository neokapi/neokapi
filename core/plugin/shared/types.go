// Package shared defines serializable types for the neokapi plugin RPC protocol.
// These types mirror the core model types but are JSON-serializable and free
// of interfaces, channels, and io.Readers, making them safe for wire transport.
//
// The v2 bridge uses gRPC with proto-generated types. These DTO types remain
// for backward compatibility with the net/rpc plugin system and are also used
// as an intermediate representation when converting between model and proto.
package shared

// AnnotationDTO is a typed annotation with a JSON-encoded payload.
type AnnotationDTO struct {
	Type string `json:"type"`
	Data []byte `json:"data"` // JSON-encoded type-specific payload
}

// RunConstraintsDTO mirrors model.RunConstraints on the wire.
type RunConstraintsDTO struct {
	Deletable   bool `json:"deletable,omitempty"`
	Cloneable   bool `json:"cloneable,omitempty"`
	Reorderable bool `json:"reorderable,omitempty"`
}

// TextRunDTO is the wire representation of a text run.
type TextRunDTO struct {
	Text string `json:"text"`
}

// PlaceholderRunDTO is the wire representation of a self-closing
// placeholder run (variable, conditional node, icon, etc.).
type PlaceholderRunDTO struct {
	ID          string             `json:"id"`
	Type        string             `json:"type"`
	SubType     string             `json:"subType,omitempty"`
	Data        string             `json:"data"`
	Equiv       string             `json:"equiv"`
	Disp        string             `json:"disp,omitempty"`
	Constraints *RunConstraintsDTO `json:"constraints,omitempty"`
}

// PcOpenRunDTO is the wire representation of the opening half of a
// paired inline code.
type PcOpenRunDTO struct {
	ID          string             `json:"id"`
	Type        string             `json:"type"`
	SubType     string             `json:"subType,omitempty"`
	Data        string             `json:"data"`
	Equiv       string             `json:"equiv"`
	Disp        string             `json:"disp,omitempty"`
	Constraints *RunConstraintsDTO `json:"constraints,omitempty"`
}

// PcCloseRunDTO is the wire representation of the closing half of a
// paired inline code.
type PcCloseRunDTO struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	SubType string `json:"subType,omitempty"`
	Data    string `json:"data"`
	Equiv   string `json:"equiv,omitempty"`
}

// SubRunDTO is the wire representation of a sub-filter reference.
type SubRunDTO struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Equiv string `json:"equiv"`
}

// PluralRunDTO is the wire representation of a structured plural
// construct. Keys of Forms are ICU plural forms.
type PluralRunDTO struct {
	Pivot string              `json:"pivot"`
	Forms map[string][]RunDTO `json:"forms"`
}

// SelectRunDTO is the wire representation of a structured select
// construct.
type SelectRunDTO struct {
	Pivot string              `json:"pivot"`
	Cases map[string][]RunDTO `json:"cases"`
}

// RunDTO is the wire representation of a model.Run. Exactly one of
// the pointer fields is non-nil per record.
type RunDTO struct {
	Text    *TextRunDTO        `json:"text,omitempty"`
	Ph      *PlaceholderRunDTO `json:"ph,omitempty"`
	PcOpen  *PcOpenRunDTO      `json:"pcOpen,omitempty"`
	PcClose *PcCloseRunDTO     `json:"pcClose,omitempty"`
	Sub     *SubRunDTO         `json:"sub,omitempty"`
	Plural  *PluralRunDTO      `json:"plural,omitempty"`
	Select  *SelectRunDTO      `json:"select,omitempty"`
}

// SegmentDTO is the wire representation of model.Segment.
type SegmentDTO struct {
	ID         string            `json:"id"`
	Runs       []RunDTO          `json:"runs,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// TargetDTO maps a locale to its target segments.
type TargetDTO struct {
	Locale   string       `json:"locale"`
	Segments []SegmentDTO `json:"segments"`
}

// SkeletonPartDTO is the wire representation of a skeleton part.
type SkeletonPartDTO struct {
	Text       string `json:"text,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Property   string `json:"property,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

// SkeletonDTO is the wire representation of model.Skeleton.
type SkeletonDTO struct {
	Strategy  int               `json:"strategy"`
	Parts     []SkeletonPartDTO `json:"parts,omitempty"`
	SourceURI string            `json:"source_uri,omitempty"`
}

// DisplayHintDTO is the wire representation of model.DisplayHint.
type DisplayHintDTO struct {
	MaxLength   int    `json:"max_length,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Context     string `json:"context,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

// BlockDTO is the wire representation of model.Block.
type BlockDTO struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name"`
	Type               string                   `json:"type"`
	MimeType           string                   `json:"mime_type"`
	Translatable       bool                     `json:"translatable"`
	Source             []SegmentDTO             `json:"source"`
	Targets            []TargetDTO              `json:"targets,omitempty"`
	Properties         map[string]string        `json:"properties,omitempty"`
	Annotations        map[string]AnnotationDTO `json:"annotations,omitempty"`
	DisplayHint        *DisplayHintDTO          `json:"display_hint,omitempty"`
	Skeleton           *SkeletonDTO             `json:"skeleton,omitempty"`
	PreserveWhitespace bool                     `json:"preserve_whitespace,omitempty"`
	IsReferent         bool                     `json:"is_referent,omitempty"`
}

// LayerDTO is the wire representation of model.Layer.
type LayerDTO struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Format         string            `json:"format"`
	Locale         string            `json:"locale"`
	Encoding       string            `json:"encoding"`
	MimeType       string            `json:"mime_type"`
	LineBreak      string            `json:"line_break"`
	IsMultilingual bool              `json:"is_multilingual"`
	ParentID       string            `json:"parent_id"`
	Properties     map[string]string `json:"properties,omitempty"`
	HasBOM         bool              `json:"has_bom,omitempty"`
}

// DataDTO is the wire representation of model.Data.
type DataDTO struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties,omitempty"`
	Skeleton   *SkeletonDTO      `json:"skeleton,omitempty"`
	IsReferent bool              `json:"is_referent,omitempty"`
}

// GroupStartDTO is the wire representation of model.GroupStart.
type GroupStartDTO struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Properties map[string]string `json:"properties,omitempty"`
}

// GroupEndDTO is the wire representation of model.GroupEnd.
type GroupEndDTO struct {
	ID string `json:"id"`
}

// MediaDTO is the wire representation of model.Media.
type MediaDTO struct {
	ID         string            `json:"id"`
	MimeType   string            `json:"mime_type"`
	Data       []byte            `json:"data,omitempty"`
	URI        string            `json:"uri"`
	AltText    string            `json:"alt_text"`
	Properties map[string]string `json:"properties,omitempty"`
}

// PartDTO is the wire representation of model.Part.
// Exactly one of the resource fields is populated based on PartType.
type PartDTO struct {
	PartType   int            `json:"part_type"`
	Block      *BlockDTO      `json:"block,omitempty"`
	Layer      *LayerDTO      `json:"layer,omitempty"`
	Data       *DataDTO       `json:"data,omitempty"`
	GroupStart *GroupStartDTO `json:"group_start,omitempty"`
	GroupEnd   *GroupEndDTO   `json:"group_end,omitempty"`
	Media      *MediaDTO      `json:"media,omitempty"`
}

// InfoResult holds plugin identification metadata returned by Info RPCs.
type InfoResult struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	MIMETypes   []string `json:"mime_types"`
	Extensions  []string `json:"extensions"`
}

// OpenArgs holds the arguments for the Open RPC.
type OpenArgs struct {
	URI            string `json:"uri"`
	SourceLanguage string `json:"source_language"`
	Encoding       string `json:"encoding"`
	Content        []byte `json:"content"`
	MimeType       string `json:"mime_type"`
	FormatID       string `json:"format_id"`
}

// ReadResult holds the response from the Read RPC.
type ReadResult struct {
	Parts []PartDTO `json:"parts"`
	Error string    `json:"error,omitempty"`
}

// WriteArgs holds the arguments for the Write RPC.
type WriteArgs struct {
	Parts    []PartDTO `json:"parts"`
	Locale   string    `json:"locale"`
	Encoding string    `json:"encoding"`
}

// WriteResult holds the response from the Write RPC.
type WriteResult struct {
	Output []byte `json:"output"`
	Error  string `json:"error,omitempty"`
}

// ToolInfoResult holds tool plugin identification metadata.
type ToolInfoResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

// ProcessArgs holds the arguments for the Process RPC.
type ProcessArgs struct {
	Parts []PartDTO `json:"parts"`
}

// ProcessResult holds the response from the Process RPC.
type ProcessResult struct {
	Parts []PartDTO `json:"parts"`
	Error string    `json:"error,omitempty"`
}
