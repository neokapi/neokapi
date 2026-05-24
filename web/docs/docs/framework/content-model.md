---
sidebar_position: 2
title: Content Model & Interfaces
---

# Key Interface Definitions

This document defines the concrete Go interfaces and types that form the foundation of neokapi. These definitions serve as the contract for all implementations.

## Table of Contents

- [Content Model](#content-model)
- [Data Format Interfaces](#data-format-interfaces)
- [Tool Interfaces](#tool-interfaces)
- [Flow Interfaces](#flow-interfaces)
- [Configuration](#configuration)
- [Plugin Protocols](#plugin-protocols)
- [Registry](#registry)

---

## Content Model

### Part (the streaming unit)

```go
package model

// PartType identifies the kind of Part flowing through a Flow.
type PartType int

const (
    PartLayerStart  PartType = iota // Start of a structural layer
    PartLayerEnd                    // End of a structural layer
    PartGroupStart                  // Start of a structural group within a layer
    PartGroupEnd                    // End of a structural group
    PartBlock                       // Translatable content
    PartData                        // Non-translatable document structure
    PartMedia                       // Binary/media content
    _                               // reserved
    _                               // reserved
    _                               // reserved
    _                               // reserved
    PartRawDocument                 // Unprocessed document
    PartCustom                      // Custom extension
)

// Part is the fundamental unit of content flowing through a Flow.
type Part struct {
    Type     PartType
    Resource Resource
}

// PartResult pairs a Part with an optional error, used in channels.
type PartResult struct {
    Part  *Part
    Error error
}

// Resource is the interface satisfied by all content payloads.
type Resource interface {
    ResourceID() string
}
```

### Layer (structural grouping)

```go
// Layer is a top-level structural grouping: a document, a section,
// or embedded content. Layers can nest — embedded content becomes
// a child Layer with its own DataFormat.
type Layer struct {
    ID             string
    Name           string
    Format         string   // DataFormat ID (e.g., "html", "json")
    Locale         LocaleID
    Encoding       string
    MimeType       string
    LineBreak      string
    IsMultilingual bool
    ParentID       string   // ID of the parent Layer (empty for root)
    Properties     map[string]string
}

func (l *Layer) ResourceID() string { return l.ID }
func (l *Layer) IsRoot() bool { return l.ParentID == "" }
func (l *Layer) IsEmbedded() bool { return l.ParentID != "" && l.Format != "" }
```

### Block (translatable content)

```go
// Block is the primary translatable content unit (Okapi: TextUnit).
type Block struct {
    ID           string
    Name         string
    Type         string
    MimeType     string
    Translatable bool
    Skeleton     *Skeleton
    Source       []*Segment
    Targets      map[LocaleID][]*Segment
    Properties   map[string]string
    Annotations  map[string]Annotation
}

func (b *Block) ResourceID() string { return b.ID }
func (b *Block) SourceText() string { /* concatenate source segments */ }
func (b *Block) FirstFragment() *Fragment { /* first source fragment */ }
func (b *Block) SetSourceText(text string) { /* replace source */ }
func (b *Block) HasTarget(locale LocaleID) bool { /* check target exists */ }
func (b *Block) TargetText(locale LocaleID) string { /* concatenate target */ }
func (b *Block) SetTargetText(locale LocaleID, text string) { /* set target */ }

// Segment is a single segment within a Block's source or target content.
type Segment struct {
    ID      string
    Content *Fragment
}
```

### Fragment (text with inline spans)

```go
// Fragment holds text content with inline Spans (Okapi: TextFragment).
type Fragment struct {
    CodedText string  // Text with span markers (special Unicode chars)
    Spans     []*Span // Inline markup elements
}

func NewFragment(text string) *Fragment { /* plain text, no spans */ }
func (f *Fragment) Text() string { /* strip span markers */ }
func (f *Fragment) HasSpans() bool { return len(f.Spans) > 0 }
func (f *Fragment) AppendText(text string) { /* append plain text */ }
func (f *Fragment) AppendSpan(span *Span) { /* add span with marker */ }
func (f *Fragment) IsEmpty() bool { return len(f.CodedText) == 0 }
```

### Span (inline markup)

```go
type SpanType int

const (
    SpanOpening     SpanType = iota // Opening tag (e.g., <b>)
    SpanClosing                     // Closing tag (e.g., </b>)
    SpanPlaceholder                 // Self-closing/standalone (e.g., <br/>)
)

// Span represents an inline markup element within a Fragment.
type Span struct {
    SpanType    SpanType
    Type        string // Semantic type (e.g., "bold", "link", "image")
    ID          string
    Data        string // Original markup verbatim (e.g., "<b>", "<a href=\"/help\">")
    OuterData   string // Outer context when needed
    Deletable   bool   // Can a translator remove this code?
    Cloneable   bool   // Can a translator duplicate this code?
    OriginalID  string // Original ID before merging/splitting
    DisplayText string // Human-readable label for editors (e.g., "[B]")
    EquivText   string // Plain text equivalent (e.g., "\n" for <br>)
    CanReorder  bool   // Can this code be reordered in translation?
    Flags       int    // Bitfield: SpanFlagHasRef, SpanFlagAdded, SpanFlagMerged, SpanFlagMarkerMasking
    Annotations map[string]Annotation
}
```

Markers in `CodedText` map to spans positionally. See
[Implementing a Format](/contribute/formats#inline-code-handling) for
a complete guide to building and reconstructing inline codes.

### Data, Media, Group markers

```go
// Data holds non-translatable document structure.
type Data struct {
    ID         string
    Name       string
    Skeleton   *Skeleton
    Properties map[string]string
}

// Media holds binary or media content.
type Media struct {
    ID        string
    MimeType  string
    Data      []byte
    URI       string
    AltText   string
    Properties map[string]string
}

// RawDocument represents an unprocessed input document.
type RawDocument struct {
    URI          string
    Encoding     string
    SourceLocale LocaleID
    TargetLocale LocaleID
    MimeType     string
    FormatID     string
    Reader       io.ReadCloser
}
```

### Skeleton

```go
// Skeleton preserves non-translatable document structure for reconstruction.
type Skeleton struct {
    Strategy  SkeletonStrategy
    Parts     []SkeletonPart // Fragment-based strategy
    SourceURI string         // Re-parse strategy
}

type SkeletonStrategy int

const (
    SkeletonFragmentBased SkeletonStrategy = iota
    SkeletonReparse
)
```

---

## Data Format Interfaces

```go
package format

// DataFormatReader reads a document and produces a stream of Parts.
type DataFormatReader interface {
    Name() string
    DisplayName() string
    Signature() FormatSignature
    Open(ctx context.Context, doc *model.RawDocument) error
    Read(ctx context.Context) <-chan model.PartResult
    Close() error
    Config() DataFormatConfig
    SetConfig(cfg DataFormatConfig) error
}

// DataFormatWriter reconstructs a document from Parts.
type DataFormatWriter interface {
    Name() string
    SetOutput(path string) error
    SetOutputWriter(w io.Writer) error
    SetLocale(locale model.LocaleID)
    SetEncoding(encoding string)
    Write(ctx context.Context, parts <-chan *model.Part) error
    Close() error
}

// FormatSignature describes how to detect a data format.
type FormatSignature struct {
    MIMETypes  []string
    Extensions []string
    MagicBytes [][]byte
    Sniff      func([]byte) bool
}
```

---

## Tool Interfaces

```go
package tool

// Tool processes Parts in a Flow.
type Tool interface {
    Name() string
    Description() string
    Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
    Config() ToolConfig
    SetConfig(cfg ToolConfig) error
}
```

### BaseTool (embedding target with event dispatch)

```go
// BaseTool provides default pass-through behavior and event dispatch.
type BaseTool struct {
    name        string
    description string
    config      ToolConfig
}

// Process dispatches each Part to the appropriate Handle* method.
func (b *BaseTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
    // reads from in, dispatches to Handle* methods, writes to out
}

// Default handlers — all pass through. Override in concrete tools.
func (b *BaseTool) HandleBlock(part *model.Part) (*model.Part, error)      { return part, nil }
func (b *BaseTool) HandleData(part *model.Part) (*model.Part, error)       { return part, nil }
func (b *BaseTool) HandleMedia(part *model.Part) (*model.Part, error)      { return part, nil }
func (b *BaseTool) HandleLayerStart(part *model.Part) (*model.Part, error) { return part, nil }
func (b *BaseTool) HandleLayerEnd(part *model.Part) (*model.Part, error)   { return part, nil }
```

---

## Flow Interfaces

```go
package flow

// Flow represents a configured sequence of Tools.
type Flow struct {
    Name          string
    Tools         []tool.Tool
    ToolFactories []ToolFactory  // for parallel: fresh tool chain per document
}

// Executor orchestrates execution of a Flow across batch items.
type Executor interface {
    Execute(ctx context.Context, f *Flow, items []*Item) error
}

// Item represents a single document to process.
type Item struct {
    Input          *model.RawDocument
    OutputPath     string
    OutputEncoding string
    TargetLocale   model.LocaleID
}

// ExecutorConfig holds configuration for the DefaultExecutor.
type ExecutorConfig struct {
    MaxConcurrency int         // 0 = runtime.NumCPU(); 1 = sequential
    ChannelSize    int         // default 64
    FailFast       bool        // default true
    Collectors     []Collector
}

// Builder provides a fluent API for constructing Flows.
fb := flow.NewFlow("translate-html").
    AddTool(tool.NewSegmentationTool()).
    AddTool(tool.NewLeveragingTool(tm)).
    AddTool(tool.NewAITranslationTool(llmClient)).
    Build()
```

---

## Configuration

```go
package config

// AppConfig holds application-level configuration with layered lookup.
type AppConfig struct {
    v *viper.Viper
}

func NewAppConfig() *AppConfig {
    // Searches for kapi.yaml in ., ~/.config/kapi, /etc/kapi
    // Env prefix: KAPI_
}
```

---

## Plugin Protocols

### gRPC Service Definitions

```protobuf
service DataFormatReaderPlugin {
    rpc Open(OpenRequest) returns (OpenResponse);
    rpc Read(ReadRequest) returns (stream PartMessage);
    rpc Close(CloseRequest) returns (CloseResponse);
    rpc Info(InfoRequest) returns (FormatInfo);
}

service DataFormatWriterPlugin {
    rpc Open(WriterOpenRequest) returns (WriterOpenResponse);
    rpc Write(stream PartMessage) returns (WriteResponse);
    rpc Close(CloseRequest) returns (CloseResponse);
}

service ToolPlugin {
    rpc Process(stream PartMessage) returns (stream PartMessage);
    rpc Info(InfoRequest) returns (ToolInfo);
}
```

---

## Registry

```go
package registry

// FormatRegistry manages available DataFormats and their configurations.
type FormatRegistry struct {
    readers map[string]FormatReaderFactory
    writers map[string]FormatWriterFactory
    configs map[string]format.DataFormatConfig
}

func (r *FormatRegistry) RegisterReader(name string, factory FormatReaderFactory)
func (r *FormatRegistry) RegisterWriter(name string, factory FormatWriterFactory)
func (r *FormatRegistry) NewReader(name string) (format.DataFormatReader, error)
func (r *FormatRegistry) Detector() *format.Detector

// ToolRegistry manages available Tools.
type ToolRegistry struct {
    tools map[string]ToolFactory
}

func (r *ToolRegistry) Register(name string, factory ToolFactory)
func (r *ToolRegistry) NewTool(name string) (tool.Tool, error)
```
