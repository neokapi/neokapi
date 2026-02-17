// Package shared defines serializable types for the gokapi plugin RPC protocol.
// These types mirror the core model types but are JSON-serializable and free
// of interfaces, channels, and io.Readers, making them safe for net/rpc transport.
package shared

// SpanDTO is the wire representation of model.Span.
type SpanDTO struct {
	SpanType  int    `json:"span_type"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	Data      string `json:"data"`
	OuterData string `json:"outer_data"`
	Deletable bool   `json:"deletable"`
	Cloneable bool   `json:"cloneable"`
}

// FragmentDTO is the wire representation of model.Fragment.
type FragmentDTO struct {
	CodedText string    `json:"coded_text"`
	Spans     []SpanDTO `json:"spans,omitempty"`
}

// SegmentDTO is the wire representation of model.Segment.
type SegmentDTO struct {
	ID      string      `json:"id"`
	Content FragmentDTO `json:"content"`
}

// TargetDTO maps a locale to its target segments.
type TargetDTO struct {
	Locale   string       `json:"locale"`
	Segments []SegmentDTO `json:"segments"`
}

// BlockDTO is the wire representation of model.Block.
type BlockDTO struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	MimeType     string            `json:"mime_type"`
	Translatable bool              `json:"translatable"`
	Source       []SegmentDTO      `json:"source"`
	Targets      []TargetDTO       `json:"targets,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
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
}

// DataDTO is the wire representation of model.Data.
type DataDTO struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties,omitempty"`
}

// GroupStartDTO is the wire representation of model.GroupStart.
type GroupStartDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
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
	URI          string `json:"uri"`
	SourceLocale string `json:"source_locale"`
	Encoding     string `json:"encoding"`
	Content      []byte `json:"content"`
	MimeType     string `json:"mime_type"`
	FormatID     string `json:"format_id"`
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
