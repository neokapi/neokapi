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
    // Explicit integer values preserve wire compatibility (JSON plugin DTOs,
    // protobuf PartMessage.part_type). Do NOT renumber existing constants.
    PartTypeUnknown PartType = 0  // zero value (uninitialized)
    PartLayerStart  PartType = 1  // Start of a structural layer
    PartLayerEnd    PartType = 2  // End of a structural layer
    PartGroupStart  PartType = 3  // Start of a structural group within a layer
    PartGroupEnd    PartType = 4  // End of a structural group
    PartBlock       PartType = 5  // Translatable content
    PartData        PartType = 6  // Non-translatable document structure
    PartMedia       PartType = 7  // Binary/media content
    // 8-11 reserved (formerly batch part types)
    PartRawDocument PartType = 12 // Unprocessed document
    PartCustom      PartType = 13 // Custom extension
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
func (b *Block) FirstSegment() *Segment { /* first source segment */ }
func (b *Block) SetSourceText(text string) { /* replace source */ }
func (b *Block) HasTarget(locale LocaleID) bool { /* check target exists */ }
func (b *Block) TargetText(locale LocaleID) string { /* concatenate target */ }
func (b *Block) SetTargetText(locale LocaleID, text string) { /* set target */ }
func (b *Block) SourceRuns() []Run { /* canonical inline content */ }
func (b *Block) SetSourceRuns(runs []Run) { /* replace source runs */ }
func (b *Block) TargetRuns(locale LocaleID) []Run { /* target inline content */ }
func (b *Block) SetTargetRuns(locale LocaleID, runs []Run) { /* set target runs */ }

// Segment is a single segment within a Block's source or target content.
// Runs is the canonical inline-content representation: a flat run sequence.
type Segment struct {
    ID          string
    Runs        []Run
    Properties  map[string]string
    Annotations map[string]Annotation
}

func (s *Segment) Text() string { /* plain-text flattening (inline codes drop) */ }
func (s *Segment) SetRuns(runs []Run) { /* replace the run sequence */ }
func (s *Segment) SetRunsText(text string) { /* a single TextRun */ }
func (s *Segment) HasInlineCodes() bool { /* any non-text run present */ }
```

### Run (inline content)

A segment's content is a flat `[]Run`. Each `Run` is a discriminated union —
exactly one pointer field is set — defined in `core/model/run.go`:

```go
// Run is one element of a segment's flat inline-content sequence.
type Run struct {
    Text    *TextRun        // plain text chunk
    Ph      *PlaceholderRun // self-closing token: variable, <br>, icon, redaction
    PcOpen  *PcOpenRun      // opening half of a paired code (<b>, <a>, …)
    PcClose *PcCloseRun     // closing half of a paired code (</b>, </a>, …)
    Sub     *SubRun         // reference to a nested Block (subfilter output)
    Plural  *PluralRun      // ICU plural with per-form Runs
    Select  *SelectRun      // ICU select with per-case Runs
}

// RunKind names a Run's discriminator (see Run.Kind()).
type RunKind string

const (
    RunKindText    RunKind = "text"
    RunKindPh      RunKind = "ph"
    RunKindPcOpen  RunKind = "pcOpen"
    RunKindPcClose RunKind = "pcClose"
    RunKindSub     RunKind = "sub"
    RunKindPlural  RunKind = "plural"
    RunKindSelect  RunKind = "select"
)

type TextRun struct {
    Text string
}

// PlaceholderRun is a self-closing inline code. PcOpenRun is identical in
// shape; PcCloseRun shares its ID with the matching PcOpen but omits Disp
// and Constraints (the close inherits the opener's behavior).
type PlaceholderRun struct {
    ID          string
    Type        string          // semantic type (e.g., "fmt:bold", "var")
    SubType     string
    Data        string          // original markup verbatim (e.g., "<br/>")
    Equiv       string          // plain-text equivalent (e.g., "\n")
    Disp        string          // editor display label (e.g., "[BR]")
    Constraints *RunConstraints // deletable / cloneable / reorderable
}

// RunConstraints is the per-run editing policy.
type RunConstraints struct {
    Deletable   bool // translator may remove this code
    Cloneable   bool // translator may duplicate this code
    Reorderable bool // this code may move relative to others
}
```

A `Run` serializes to JSON as an object with exactly one of the keys `text`,
`ph`, `pcOpen`, `pcClose`, `sub`, `plural`, or `select`. See
[Implementing a Format](/contribute/formats#inline-code-handling) for a complete
guide to building and reconstructing inline codes from runs.

> A coded-text exchange form (`Fragment` with a private-use-area-marked
> `CodedText` string and a parallel `[]Span`, mirroring Okapi's `TextFragment`)
> historically backed inline content. It has been removed; `[]Run` is the
> canonical representation.

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
    AddTool(aitools.NewAITranslateTool(llmProvider, translateCfg)).
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

// FormatID and ToolID are string-typed identifiers.
type FormatID string
type ToolID string

// FormatRegistry manages available DataFormats and their configurations.
type FormatRegistry struct { /* readers, writers, configs */ }

func (r *FormatRegistry) RegisterReader(name FormatID, factory FormatReaderFactory, sig format.FormatSignature, displayName string)
func (r *FormatRegistry) RegisterWriter(name FormatID, factory FormatWriterFactory)
func (r *FormatRegistry) NewReader(name FormatID) (format.DataFormatReader, error)
func (r *FormatRegistry) Detector() *format.Detector

// ToolRegistry manages available Tools.
type ToolRegistry struct { /* name → factory */ }

func (r *ToolRegistry) Register(name ToolID, factory ToolFactory)
func (r *ToolRegistry) RegisterWithSchema(name ToolID, factory ToolFactory, s *schema.ComponentSchema)
func (r *ToolRegistry) NewTool(name ToolID) (tool.Tool, error)
```

## Plugin protocols

Out-of-process formats, tools, and source connectors run as Mode-C plugin
daemons and are reached over a single gRPC `BridgeService`. A bidirectional
`Process` stream carries the whole document lifecycle — its mode (read-only,
read-write, or write-only) is selected by the header rather than by separate
RPCs:

```protobuf
service BridgeService {
    // Full document cycle. The ProcessHeader selects read-only,
    // read-write, or write-only mode.
    rpc Process(stream ProcessRequest) returns (stream ProcessResponse);
    // Run a single Okapi pipeline step over a stream of parts.
    rpc ProcessStep(stream StepRequest) returns (stream StepResponse);
    // Gracefully stop the daemon.
    rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

The `ProcessRequest` / `ProcessResponse` payloads stream `PartMessage`s that map
onto the in-process `Part` model via `core/plugin/protoconvert`. The full
service is defined in `core/plugin/proto/v2/neokapi_bridge.proto`.

See [Plugin System](/contribute/plugins),
[Okapi Bridge](/contribute/java-bridge), and the
[Okapi Bridge Protocol](/contribute/notes-internal/plugin-bridge-protocol) for
the full protocol.
