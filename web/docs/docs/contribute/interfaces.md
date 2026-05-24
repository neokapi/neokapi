---
sidebar_position: 2
title: Interface Reference
description: The concrete Go interfaces and types that form neokapi's implementation contract — DataFormatReader, DataFormatWriter, Tool, Executor, LLMProvider, MTProvider — with signatures for writing formats, tools, and plugins.
keywords: [interface reference, DataFormatReader, DataFormatWriter, Tool, Executor, Go interfaces, neokapi]
---

# Interface Reference

This page collects the concrete Go interfaces and types that form neokapi's
implementation contract — the signatures you implement when writing a format, a
tool, or a plugin. For the _concepts_ behind these types (what a Part is, why the
content model is shaped this way), see the framework section:
[Content Model](/framework/content-model), [Tools](/framework/tools),
[Flows](/framework/flows), and [Pipeline](/framework/pipeline).

## Content model

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
[Implementing a Format](/contribute/formats#inline-code-handling) for a complete
guide to building and reconstructing inline codes.

### Data, Media, RawDocument

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

## Data format interfaces

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

## Tool interfaces

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

// ToolConfig holds configuration for a Tool.
type ToolConfig interface {
    ToolName() string
    Reset()
    Validate() error
}

// SchemaProvider is implemented by tools that declare a parameter schema,
// enabling schema-driven CLI flags, config panels, and validation.
type SchemaProvider interface {
    Schema() *schema.ComponentSchema
}
```

### BaseTool (embedding target with part-type dispatch)

```go
// PartHandler handles a single Part, returning the (possibly transformed) Part.
type PartHandler func(part *model.Part) (*model.Part, error)

// BaseTool implements Process once and dispatches each Part to the matching
// handler. Embed it and set only the handler fields you need; unset handlers
// pass the Part through unchanged.
type BaseTool struct {
    ToolName        string
    ToolDescription string
    Cfg             ToolConfig
    SchemaFn        func() *schema.ComponentSchema

    HandleBlockFn      PartHandler
    HandleDataFn       PartHandler
    HandleMediaFn      PartHandler
    HandleLayerStartFn PartHandler
    HandleLayerEndFn   PartHandler
    HandleGroupStartFn PartHandler
    HandleGroupEndFn   PartHandler
}
```

### SessionTool (random access to project block state)

```go
// SessionTool is an optional interface for tools that need random access to
// the project's block state alongside the streaming channel contract. The
// executor opens a blockstore session and passes it to SessionProcess.
// Tools implementing SessionTool MUST also implement Tool.
type SessionTool interface {
    Tool
    SessionProcess(
        ctx context.Context,
        sess blockstore.Session,
        in <-chan *model.Part,
        out chan<- *model.Part,
    ) error
}
```

See [Session Tool Authoring](/contribute/notes-internal/session-tool-authoring)
for the lifecycle and when to use this contract.

## Flow interfaces

```go
package flow

// ToolFactory creates a fresh Tool instance (parallel execution gives each
// document its own tool chain).
type ToolFactory func() (tool.Tool, error)

// Flow is a configured sequence of Tools that Parts stream through.
type Flow struct {
    Name          string
    Tools         []tool.Tool   // sequential / single-document execution
    ToolFactories []ToolFactory // parallel: fresh tool chain per document
}

// Item represents a single document to process in a batch.
type Item struct {
    Input          *model.RawDocument
    OutputPath     string
    OutputEncoding string
    TargetLocale   model.LocaleID
    OutputBlocks   []*model.Block // populated after execution
}

// Executor orchestrates execution of a Flow across batch items.
type Executor interface {
    Execute(ctx context.Context, f *Flow, items []*Item) error
}

// ExecutorConfig configures the DefaultExecutor.
type ExecutorConfig struct {
    MaxConcurrency int             // 0 = runtime.NumCPU(); 1 = sequential
    ChannelSize    int             // default 64
    FailFast       bool            // default true
    Collectors     []Collector
    Store          blockstore.Store // backs SessionTool dispatch
}
```

The `Builder` provides a fluent API for constructing flows:

```go
f, err := flow.NewFlow("translate").
    AddTool(tools.NewTMLeverageTool(tmCfg)).
    AddTool(aitools.NewAITranslateTool(translateCfg)).
    Build()

executor := flow.NewExecutor(
    flow.WithMaxConcurrency(0),
    flow.WithChannelSize(64),
    flow.WithFailFast(true),
)
err = executor.Execute(ctx, f, items)
```

## Registry

```go
package registry

// FormatRegistry manages available DataFormats and their configurations.
type FormatRegistry struct { /* readers, writers, configs */ }

func (r *FormatRegistry) RegisterReader(name string, factory FormatReaderFactory)
func (r *FormatRegistry) RegisterWriter(name string, factory FormatWriterFactory)
func (r *FormatRegistry) NewReader(name string) (format.DataFormatReader, error)
func (r *FormatRegistry) Detector() *format.Detector

// ToolRegistry manages available Tools.
type ToolRegistry struct { /* name → factory */ }

func (r *ToolRegistry) Register(name string, factory ToolFactory)
func (r *ToolRegistry) RegisterWithSchema(name string, factory ToolFactory, s *schema.ComponentSchema)
func (r *ToolRegistry) NewTool(name string) (tool.Tool, error)
```

## Plugin protocols

Out-of-process formats and tools are exposed over gRPC. The service definitions
mirror the in-process interfaces:

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

See [Plugin System](/contribute/plugins) and the
[Plugin Bridge Protocol](/contribute/notes-internal/plugin-bridge-protocol) for
the full protocol.
