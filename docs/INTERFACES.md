# gokapi: Key Interface Definitions

This document defines the concrete Go interfaces and types that form the foundation of gokapi. These definitions serve as the contract for all implementations.

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
    PartLayerStart  PartType = iota // Start of a structural layer (document, section, embedded content)
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
// It carries a typed payload (the Resource).
type Part struct {
    Type     PartType
    Resource Resource
}

// PartResult pairs a Part with an optional error, used in channels
// to propagate errors alongside content.
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
// Layer is a top-level structural grouping: a document, a section, or embedded content.
// Layers can nest — embedded content (HTML inside JSON, CDATA in XML) becomes
// a child Layer with its own DataFormat.
//
// This replaces Okapi's StartDocument, EndDocument, StartSubDocument, EndSubDocument,
// StartSubFilter, and EndSubFilter with a single hierarchical concept.
type Layer struct {
    ID             string
    Name           string
    Format         string   // DataFormat ID (e.g., "html", "json", "xml"). Empty = same as parent.
    Locale         LocaleID
    Encoding       string
    MimeType       string
    LineBreak      string
    IsMultilingual bool
    ParentID       string   // ID of the parent Layer (empty for root)
    Properties     map[string]string
}

func (l *Layer) ResourceID() string { return l.ID }

// IsRoot returns true if this is a root (document-level) Layer.
func (l *Layer) IsRoot() bool { return l.ParentID == "" }

// IsEmbedded returns true if this Layer represents embedded content with a different format.
func (l *Layer) IsEmbedded() bool { return l.ParentID != "" && l.Format != "" }
```

### Block (translatable content)

```go
// Block is the primary translatable content unit (Okapi: TextUnit).
// Source and target segments live directly on the Block.
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

// SourceText returns the plain text of all source segments concatenated.
func (b *Block) SourceText() string {
    var buf strings.Builder
    for _, seg := range b.Source {
        buf.WriteString(seg.Content.Text())
    }
    return buf.String()
}

// FirstFragment returns the Fragment of the first source segment.
func (b *Block) FirstFragment() *Fragment {
    if len(b.Source) == 0 {
        return nil
    }
    return b.Source[0].Content
}

// SetSourceText replaces all source content with a single unsegmented Fragment.
func (b *Block) SetSourceText(text string) {
    b.Source = []*Segment{{ID: "s1", Content: NewFragment(text)}}
}

// HasTarget returns true if target segments exist for the given locale.
func (b *Block) HasTarget(locale LocaleID) bool {
    segs, ok := b.Targets[locale]
    return ok && len(segs) > 0
}

// TargetText returns the plain text of all target segments for the given locale.
func (b *Block) TargetText(locale LocaleID) string {
    segs, ok := b.Targets[locale]
    if !ok {
        return ""
    }
    var buf strings.Builder
    for _, seg := range segs {
        buf.WriteString(seg.Content.Text())
    }
    return buf.String()
}

// SetTargetText sets the target text for a locale as a single unsegmented Fragment.
func (b *Block) SetTargetText(locale LocaleID, text string) {
    if b.Targets == nil {
        b.Targets = make(map[LocaleID][]*Segment)
    }
    b.Targets[locale] = []*Segment{{ID: "s1", Content: NewFragment(text)}}
}

// Segment is a single segment within a Block's source or target content.
type Segment struct {
    ID      string
    Content *Fragment
}
```

### Fragment (text with inline spans)

```go
// Fragment holds text content with inline Spans (Okapi: TextFragment).
// Spans are represented as markers in the CodedText with metadata in the Spans slice.
type Fragment struct {
    CodedText string  // Text with span markers (special Unicode chars)
    Spans     []*Span // Inline markup elements
}

// NewFragment creates a Fragment from plain text (no spans).
func NewFragment(text string) *Fragment {
    return &Fragment{CodedText: text}
}

// Text returns the plain text with all span markers stripped.
func (f *Fragment) Text() string {
    // Strip span marker characters from CodedText
    // Implementation: filter out chars in the private use area
}

// HasSpans returns true if this Fragment contains any inline spans.
func (f *Fragment) HasSpans() bool {
    return len(f.Spans) > 0
}

// AppendText appends plain text to the Fragment.
func (f *Fragment) AppendText(text string) {
    f.CodedText += text
}

// AppendSpan appends an inline span and its marker to the Fragment.
func (f *Fragment) AppendSpan(span *Span) {
    // Add marker character to CodedText
    // Append span to Spans slice
}

// IsEmpty returns true if the Fragment has no content.
func (f *Fragment) IsEmpty() bool {
    return len(f.CodedText) == 0
}

// Length returns the length of the coded text (including markers).
func (f *Fragment) Length() int {
    return len(f.CodedText)
}
```

### Span (inline markup)

```go
// SpanType classifies inline markup elements.
type SpanType int

const (
    SpanOpening     SpanType = iota // Opening tag (e.g., <b>)
    SpanClosing                     // Closing tag (e.g., </b>)
    SpanPlaceholder                 // Self-closing/standalone (e.g., <br/>)
)

// Span represents an inline markup element within a Fragment (Okapi: Code).
type Span struct {
    SpanType  SpanType
    Type      string // Semantic type (e.g., "bold", "link", "image")
    ID        string
    Data      string // Original markup data (e.g., "<b>")
    OuterData string
    Deletable bool
    Cloneable bool
}
```

### Data, Media, Group markers

```go
// Data holds non-translatable document structure (Okapi: DocumentPart).
type Data struct {
    ID         string
    Name       string
    Skeleton   *Skeleton
    Properties map[string]string
}

func (d *Data) ResourceID() string { return d.ID }

// Media holds binary or media content (images, embedded objects).
type Media struct {
    ID        string
    MimeType  string
    Data      []byte
    URI       string // External reference if not inline
    AltText   string // Accessible alternative text
    Properties map[string]string
}

func (m *Media) ResourceID() string { return m.ID }

// GroupStart signals the beginning of a structural group within a Layer.
type GroupStart struct {
    ID   string
    Name string
    Type string
}

func (gs *GroupStart) ResourceID() string { return gs.ID }

// GroupEnd signals the end of a structural group.
type GroupEnd struct {
    ID string
}

func (ge *GroupEnd) ResourceID() string { return ge.ID }

// RawDocument represents an unprocessed input document.
type RawDocument struct {
    URI          string
    Encoding     string
    SourceLocale LocaleID
    TargetLocale LocaleID
    MimeType     string
    FormatID     string // e.g., "html", "xliff", "docx"
    Reader       io.ReadCloser
}

func (rd *RawDocument) ResourceID() string { return rd.URI }
```

### Skeleton (per-Block)

```go
// Skeleton preserves non-translatable document structure for reconstruction.
// Used by fragment-based formats (XML, XLIFF) where skeleton data is carried
// on individual Block/Data resources.
type Skeleton struct {
    Parts []SkeletonPart
}

// SkeletonPart is either a literal text fragment or a reference to a Block/Data.
type SkeletonPart interface {
    isSkeletonPart()
}

type SkeletonText struct {
    Text string
}

func (st *SkeletonText) isSkeletonPart() {}

type SkeletonRef struct {
    ResourceID string
    Property   string // Which property to reference (e.g., "target", "source")
}

func (sr *SkeletonRef) isSkeletonPart() {}
```

### SkeletonStore (document-level streaming)

```go
// SkeletonStore streams document skeleton data through temporary file storage.
// The reader writes text/ref entries as it parses; the writer reads them
// sequentially to reconstruct the document with byte-exact fidelity.
//
// Binary format: [type:1byte] [length:4bytes big-endian] [data:N bytes]
//   type 0 = text (non-translatable raw bytes)
//   type 1 = ref  (block ID as UTF-8)
//
// See docs/notes/skeleton-store.md for full details.
type SkeletonStore struct {
    file   *os.File
    writer *bufio.Writer
    reader *bufio.Reader
}

func NewSkeletonStore() (*SkeletonStore, error)    // creates temp file
func (s *SkeletonStore) WriteText(data []byte) error // non-translatable bytes (skips empty)
func (s *SkeletonStore) WriteRef(blockID string) error // block placeholder
func (s *SkeletonStore) Flush() error                  // finish writing, prepare for reading
func (s *SkeletonStore) Next() (SkeletonEntry, error)  // read next entry (io.EOF when done)
func (s *SkeletonStore) Close() error                  // cleanup temp file

type SkeletonEntryType byte

const (
    SkeletonText SkeletonEntryType = 0
    SkeletonRef  SkeletonEntryType = 1
)

type SkeletonEntry struct {
    Type SkeletonEntryType
    Data []byte // raw bytes (text) or block ID (ref)
}

// SkeletonStoreEmitter is implemented by readers that write skeleton data
// during extraction.
type SkeletonStoreEmitter interface {
    SetSkeletonStore(store *SkeletonStore)
}

// SkeletonStoreConsumer is implemented by writers that read skeleton data
// during reconstruction.
type SkeletonStoreConsumer interface {
    SetSkeletonStore(store *SkeletonStore)
}
```

Flow executor wiring (in `cli/flow.go`, `cli/toolrun.go`, `kapi/cmd/kapi/mcp_tools.go`):

```go
// Wire skeleton store if both reader and writer support it.
// Must be wired BEFORE reader.Read() — the reader writes entries during reading.
if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
    if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
        store, err := format.NewSkeletonStore()
        if err == nil {
            defer store.Close()
            emitter.SetSkeletonStore(store)
            consumer.SetSkeletonStore(store)
        }
    }
}
```

### Supporting types

```go
// LocaleID represents a BCP 47 language tag.
type LocaleID string

const (
    LocaleEnglish  LocaleID = "en"
    LocaleFrench   LocaleID = "fr"
    LocaleGerman   LocaleID = "de"
    LocaleJapanese LocaleID = "ja"
    LocaleSpanish  LocaleID = "es"
    LocaleChinese  LocaleID = "zh"
)

// Annotation is an extensible metadata attachment on Blocks.
type Annotation interface {
    AnnotationType() string
}

// AltTranslation holds an alternative translation with metadata.
type AltTranslation struct {
    Source    *Fragment
    Target   *Fragment
    Locale   LocaleID
    Origin   string  // Where this translation came from (TM, MT, etc.)
    Score    float64 // Match quality (0.0 - 1.0)
    MatchType string // "exact", "fuzzy", "mt", "ai"
}

func (at *AltTranslation) AnnotationType() string { return "alt-translations" }
```

---

## Data Format Interfaces

```go
package format

// DataFormatReader reads a document and produces a stream of Parts (Okapi: IFilter).
type DataFormatReader interface {
    // Name returns the unique identifier for this format (e.g., "html", "xliff").
    Name() string

    // DisplayName returns a human-readable name (e.g., "HTML Filter").
    DisplayName() string

    // Signature returns detection metadata: MIME types, extensions, and content signatures.
    Signature() FormatSignature

    // Open opens a RawDocument for reading. Call Read() to stream Parts.
    Open(ctx context.Context, doc *model.RawDocument) error

    // Read returns a channel of PartResults. The channel is closed when
    // the document is fully read or an error occurs. Context cancellation
    // stops reading.
    Read(ctx context.Context) <-chan model.PartResult

    // Close releases resources.
    Close() error

    // Config returns the current configuration.
    Config() DataFormatConfig

    // SetConfig applies a new configuration.
    SetConfig(cfg DataFormatConfig) error
}

// FormatSignature describes how to detect a data format.
type FormatSignature struct {
    MIMETypes  []string            // e.g., ["text/html", "application/xhtml+xml"]
    Extensions []string            // e.g., [".html", ".htm", ".xhtml"]
    MagicBytes [][]byte            // Byte prefixes to match (e.g., []byte("<!DOCTYPE"))
    Sniff      func([]byte) bool   // Custom content sniffing function (optional)
}

// FormatDetector determines the data format of a document using multiple strategies.
type FormatDetector struct {
    registry *FormatRegistry
}

// Detect tries all strategies: explicit MIME → extension → content sniffing.
func (d *FormatDetector) Detect(path string, reader io.ReadSeeker, mimeType string) (string, error)

// DetectByMIME maps a MIME type to a registered format name.
func (d *FormatDetector) DetectByMIME(mimeType string) (string, error)

// DetectByExtension maps a file extension to a registered format name.
func (d *FormatDetector) DetectByExtension(ext string) (string, error)

// DetectByContent reads the first N bytes and matches against registered signatures.
func (d *FormatDetector) DetectByContent(reader io.ReadSeeker) (string, error)

// DataFormatWriter reconstructs a document from Parts (Okapi: IFilterWriter).
type DataFormatWriter interface {
    // Name returns the format name matching the reader.
    Name() string

    // SetOutput configures the output destination.
    SetOutput(path string) error

    // SetOutputWriter configures an io.Writer as output.
    SetOutputWriter(w io.Writer) error

    // SetLocale sets the target locale for writing.
    SetLocale(locale model.LocaleID)

    // SetEncoding sets the output encoding.
    SetEncoding(encoding string)

    // Write consumes Parts from a channel and writes the reconstructed document.
    // Returns when the channel is closed or context is canceled.
    Write(ctx context.Context, parts <-chan *model.Part) error

    // Close flushes and closes the output.
    Close() error
}

// DataFormatConfig holds configuration for a data format.
type DataFormatConfig interface {
    // FormatName returns the format this config applies to.
    FormatName() string

    // Reset restores default values.
    Reset()

    // Validate checks configuration validity.
    Validate() error
}
```

### BaseFormatReader (embedding target)

```go
// BaseFormatReader provides shared behavior for format reader implementations.
// Embed this in concrete readers.
type BaseFormatReader struct {
    name        string
    displayName string
    mimeType    string
    extensions  []string
    config      DataFormatConfig
    doc         *model.RawDocument
}

func (b *BaseFormatReader) Name() string           { return b.name }
func (b *BaseFormatReader) DisplayName() string     { return b.displayName }
func (b *BaseFormatReader) MimeType() string        { return b.mimeType }
func (b *BaseFormatReader) Extensions() []string    { return b.extensions }
func (b *BaseFormatReader) Config() DataFormatConfig { return b.config }
func (b *BaseFormatReader) SetConfig(cfg DataFormatConfig) error {
    if err := cfg.Validate(); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }
    b.config = cfg
    return nil
}
```

---

## Tool Interfaces

```go
package tool

// Tool processes Parts in a Flow (Okapi: IPipelineStep).
type Tool interface {
    // Name returns the tool's unique identifier.
    Name() string

    // Description returns a human-readable description.
    Description() string

    // Process reads Parts from the input channel, processes them,
    // and writes results to the output channel. Both channels carry
    // model.Part values. Process blocks until input is exhausted or
    // context is canceled.
    Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error

    // Config returns the current configuration.
    Config() ToolConfig

    // SetConfig applies a new configuration.
    SetConfig(cfg ToolConfig) error
}

// ToolConfig holds configuration for a Tool.
type ToolConfig interface {
    // ToolName returns the tool this config applies to.
    ToolName() string

    // Reset restores default values.
    Reset()

    // Validate checks configuration validity.
    Validate() error
}
```

### BaseTool (embedding target with event dispatch)

```go
// BaseTool provides default pass-through behavior and event dispatch.
// Embed in concrete tools and override Handle* methods as needed.
type BaseTool struct {
    name        string
    description string
    config      ToolConfig
}

func (b *BaseTool) Name() string        { return b.name }
func (b *BaseTool) Description() string { return b.description }
func (b *BaseTool) Config() ToolConfig  { return b.config }
func (b *BaseTool) SetConfig(cfg ToolConfig) error {
    if err := cfg.Validate(); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }
    b.config = cfg
    return nil
}

// Process dispatches each Part to the appropriate Handle* method.
// Override this only if you need full control over the processing loop.
func (b *BaseTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-in:
            if !ok {
                return nil // input channel closed
            }
            result, err := b.dispatch(part)
            if err != nil {
                return err
            }
            select {
            case out <- result:
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
}

func (b *BaseTool) dispatch(part *model.Part) (*model.Part, error) {
    switch part.Type {
    case model.PartBlock:
        return b.HandleBlock(part)
    case model.PartData:
        return b.HandleData(part)
    case model.PartMedia:
        return b.HandleMedia(part)
    case model.PartLayerStart:
        return b.HandleLayerStart(part)
    case model.PartLayerEnd:
        return b.HandleLayerEnd(part)
    case model.PartGroupStart:
        return b.HandleGroupStart(part)
    case model.PartGroupEnd:
        return b.HandleGroupEnd(part)
    default:
        return part, nil // pass through unknown types
    }
}

// Default handlers — all pass through. Override in concrete tools.
func (b *BaseTool) HandleBlock(part *model.Part) (*model.Part, error)      { return part, nil }
func (b *BaseTool) HandleData(part *model.Part) (*model.Part, error)       { return part, nil }
func (b *BaseTool) HandleMedia(part *model.Part) (*model.Part, error)      { return part, nil }
func (b *BaseTool) HandleLayerStart(part *model.Part) (*model.Part, error) { return part, nil }
func (b *BaseTool) HandleLayerEnd(part *model.Part) (*model.Part, error)   { return part, nil }
func (b *BaseTool) HandleGroupStart(part *model.Part) (*model.Part, error) { return part, nil }
func (b *BaseTool) HandleGroupEnd(part *model.Part) (*model.Part, error)   { return part, nil }
```

### Concrete Tool example

```go
// SegmentationTool applies SRX segmentation rules to Blocks.
type SegmentationTool struct {
    BaseTool
    rules *srx.Rules
}

func NewSegmentationTool() *SegmentationTool {
    return &SegmentationTool{
        BaseTool: BaseTool{
            name:        "segmentation",
            description: "Applies SRX segmentation rules to translatable blocks",
        },
    }
}

func (s *SegmentationTool) HandleBlock(part *model.Part) (*model.Part, error) {
    block := part.Resource.(*model.Block)
    if !block.Translatable {
        return part, nil
    }
    // Apply SRX rules to segment the source content into multiple segments
    block.Source = s.segmentContent(block.Source)
    return part, nil
}
```

---

## Flow Interfaces

```go
package flow

// ToolFactory creates a fresh Tool instance. Used for parallel execution
// where each document needs its own tool chain to avoid shared state.
type ToolFactory func() (tool.Tool, error)

// Flow represents a configured sequence of Tools that Parts stream through.
type Flow struct {
    Name          string
    Tools         []tool.Tool    // for single-doc / sequential (backward compat)
    ToolFactories []ToolFactory  // for parallel: creates fresh tool chain per document
}

// FlowExecutor orchestrates the execution of a Flow across batch items.
type FlowExecutor interface {
    // Execute runs the Flow over the given batch items.
    Execute(ctx context.Context, f *Flow, items []*FlowItem) error
}

// FlowItem represents a single document to process in a batch.
type FlowItem struct {
    Input          *model.RawDocument
    OutputPath     string
    OutputEncoding string
    TargetLocale   model.LocaleID
}

// Collector accumulates results from processed documents.
// Implementations must be safe for concurrent use.
type Collector interface {
    Collect(ctx context.Context, item *FlowItem, parts []*model.Part) error
    Result() (CollectorResult, error)
}

type CollectorResult struct {
    Name string
    Data interface{}
}

// ExecutorConfig holds configuration for the DefaultFlowExecutor.
type ExecutorConfig struct {
    MaxConcurrency int         // 0 = runtime.NumCPU(); 1 = sequential
    ChannelSize    int         // default 64
    FailFast       bool        // default true
    Collectors     []Collector
}

// ExecutorOption is a functional option for configuring a DefaultFlowExecutor.
type ExecutorOption func(*ExecutorConfig)

func WithMaxConcurrency(n int) ExecutorOption { ... }
func WithChannelSize(n int) ExecutorOption    { ... }
func WithFailFast(b bool) ExecutorOption      { ... }
func WithCollectors(c ...Collector) ExecutorOption { ... }

// DefaultFlowExecutor runs tools concurrently using goroutines and channels.
// With MaxConcurrency > 1, documents are processed in parallel using
// a semaphore-bounded fan-out pattern with errgroup.
type DefaultFlowExecutor struct {
    config ExecutorConfig
}

func NewFlowExecutor(opts ...ExecutorOption) *DefaultFlowExecutor {
    // defaults: MaxConcurrency=1, ChannelSize=64, FailFast=true
}

// Execute processes FlowItems through the tool chain.
// When MaxConcurrency > 1 (or 0 for NumCPU), items are processed
// in parallel using a semaphore-bounded fan-out pattern.
func (e *DefaultFlowExecutor) Execute(ctx context.Context, f *Flow, items []*FlowItem) error
```

### Flow Builder

```go
// FlowBuilder provides a fluent API for constructing Flows.
type FlowBuilder struct {
    name  string
    tools []tool.Tool
    items []*FlowItem
}

func NewFlow(name string) *FlowBuilder {
    return &FlowBuilder{name: name}
}

func (fb *FlowBuilder) AddTool(t tool.Tool) *FlowBuilder {
    fb.tools = append(fb.tools, t)
    return fb
}

func (fb *FlowBuilder) AddToolFactory(f ToolFactory) *FlowBuilder {
    fb.toolFactories = append(fb.toolFactories, f)
    return fb
}

func (fb *FlowBuilder) AddItem(input *model.RawDocument, outputPath string, targetLocale model.LocaleID) *FlowBuilder {
    fb.items = append(fb.items, &FlowItem{
        Input:        input,
        OutputPath:   outputPath,
        TargetLocale: targetLocale,
    })
    return fb
}

func (fb *FlowBuilder) Build() *Flow {
    return &Flow{
        Name:  fb.name,
        Tools: fb.tools,
    }
}

// Usage (sequential, single document):
// f := flow.NewFlow("translate-html").
//     AddTool(tool.NewSegmentationTool()).
//     AddTool(tool.NewLeveragingTool(tm)).
//     AddTool(tool.NewAITranslationTool(llmClient)).
//     Build()
//
// executor := flow.NewFlowExecutor()
// executor.Execute(ctx, f, items)
//
// Usage (parallel, multiple documents with collector):
// wc := tools.NewWordCountCollector()
// f := flow.NewFlow("word-count").
//     AddToolFactory(func() (tool.Tool, error) {
//         return tools.NewWordCountTool(&tools.WordCountConfig{...}), nil
//     }).Build()
//
// executor := flow.NewFlowExecutor(
//     flow.WithMaxConcurrency(8),
//     flow.WithCollectors(wc),
// )
// executor.Execute(ctx, f, items)
// result, _ := wc.Result()
```

---

## Configuration

```go
package config

import "github.com/spf13/viper"

// AppConfig holds application-level configuration loaded via Viper.
type AppConfig struct {
    v *viper.Viper
}

// NewAppConfig creates a config reader that searches for kapi.yaml
// in standard locations.
func NewAppConfig() *AppConfig {
    v := viper.New()
    v.SetConfigName("kapi")
    v.SetConfigType("yaml")
    v.AddConfigPath(".")
    v.AddConfigPath("$HOME/.config/kapi")
    v.AddConfigPath("/etc/kapi")
    v.SetEnvPrefix("KAPI")
    v.AutomaticEnv()
    return &AppConfig{v: v}
}

func (c *AppConfig) Load() error {
    return c.v.ReadInConfig()
}

// Example kapi.yaml:
//
// formats:
//   html:
//     encoding: UTF-8
//     preserveWhitespace: false
//   xliff:
//     version: "2.0"
//
// tools:
//   segmentation:
//     srxPath: "./rules/default.srx"
//   ai-translation:
//     provider: "anthropic"
//     model: "claude-sonnet-4-20250514"
//     apiKey: "${ANTHROPIC_API_KEY}"
//
// plugins:
//   directory: "./plugins"
//   registry: "https://plugins.gokapi.dev"
//
// flow:
//   channelBuffer: 64
//   workerPool: 4
```

---

## Plugin Protocols

### gRPC Service Definitions

```protobuf
syntax = "proto3";
package gokapi.plugin.v1;

// DataFormatReaderPlugin is served by format reader plugins.
service DataFormatReaderPlugin {
    // Open initializes the reader with a document.
    rpc Open(OpenRequest) returns (OpenResponse);

    // Read streams Parts from the document.
    rpc Read(ReadRequest) returns (stream PartMessage);

    // Close releases resources.
    rpc Close(CloseRequest) returns (CloseResponse);

    // Info returns metadata about the format.
    rpc Info(InfoRequest) returns (FormatInfo);
}

// DataFormatWriterPlugin is served by format writer plugins.
service DataFormatWriterPlugin {
    // Open initializes the writer with output configuration.
    rpc Open(WriterOpenRequest) returns (WriterOpenResponse);

    // Write consumes a stream of Parts and writes the document.
    rpc Write(stream PartMessage) returns (WriteResponse);

    // Close flushes and closes.
    rpc Close(CloseRequest) returns (CloseResponse);
}

// ToolPlugin is served by tool plugins.
service ToolPlugin {
    // Process is a bidirectional stream: Parts flow in and out.
    rpc Process(stream PartMessage) returns (stream PartMessage);

    // Info returns metadata about the tool.
    rpc Info(InfoRequest) returns (ToolInfo);
}

// PartMessage is the wire format for a Part.
message PartMessage {
    int32 type = 1;
    bytes resource_json = 2;  // JSON-serialized resource
    string error = 3;         // Non-empty if this is an error
}

message FormatInfo {
    string name = 1;
    string display_name = 2;
    string mime_type = 3;
    repeated string extensions = 4;
    string version = 5;
}

message ToolInfo {
    string name = 1;
    string description = 2;
    string version = 3;
}

message OpenRequest {
    string uri = 1;
    string encoding = 2;
    string source_locale = 3;
    string target_locale = 4;
    bytes config_json = 5;
}

// ... remaining message types
```

### go-plugin Integration

```go
package plugin

import (
    "github.com/hashicorp/go-plugin"
)

// Handshake is shared between host and plugins.
var Handshake = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "GOKAPI_PLUGIN",
    MagicCookieValue: "gokapi-v1",
}

// PluginMap defines the plugin types gokapi supports.
var PluginMap = map[string]plugin.Plugin{
    "format_reader": &DataFormatReaderGRPCPlugin{},
    "format_writer": &DataFormatWriterGRPCPlugin{},
    "tool":          &ToolGRPCPlugin{},
}

// DataFormatReaderGRPCPlugin implements go-plugin's Plugin and GRPCPlugin interfaces.
type DataFormatReaderGRPCPlugin struct {
    plugin.Plugin
    Impl format.DataFormatReader // Set for server side
}

func (p *DataFormatReaderGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
    proto.RegisterDataFormatReaderPluginServer(s, &formatReaderGRPCServer{impl: p.Impl})
    return nil
}

func (p *DataFormatReaderGRPCPlugin) GRPCClient(broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
    return &formatReaderGRPCClient{client: proto.NewDataFormatReaderPluginClient(c)}, nil
}
```

### Java Bridge Plugin

```go
// JavaBridgeReader wraps an Okapi Java filter as a gokapi DataFormatReader plugin.
// It runs a JVM subprocess communicating via gRPC.
//
// The Java side implements the DataFormatReaderPlugin gRPC service,
// delegating to the original Okapi IFilter implementation.
//
// Usage: register as a go-plugin executable:
//   gokapi-okapi-bridge --filter=net.sf.okapi.filters.openxml.OpenXMLFilter
type JavaBridgeReader struct {
    filterClass string
    jvmPath     string
    client      *plugin.Client
    reader      format.DataFormatReader
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

type FormatReaderFactory func() format.DataFormatReader
type FormatWriterFactory func() format.DataFormatWriter

func NewFormatRegistry() *FormatRegistry {
    return &FormatRegistry{
        readers: make(map[string]FormatReaderFactory),
        writers: make(map[string]FormatWriterFactory),
        configs: make(map[string]format.DataFormatConfig),
    }
}

// RegisterReader registers a DataFormatReader factory.
func (r *FormatRegistry) RegisterReader(name string, factory FormatReaderFactory) {
    r.readers[name] = factory
}

// RegisterWriter registers a DataFormatWriter factory.
func (r *FormatRegistry) RegisterWriter(name string, factory FormatWriterFactory) {
    r.writers[name] = factory
}

// NewReader creates a new reader instance for the given format name.
func (r *FormatRegistry) NewReader(name string) (format.DataFormatReader, error) {
    factory, ok := r.readers[name]
    if !ok {
        return nil, fmt.Errorf("unknown format: %s", name)
    }
    return factory(), nil
}

// Detector returns a FormatDetector backed by this registry.
func (r *FormatRegistry) Detector() *format.FormatDetector {
    return &format.FormatDetector{Registry: r}
}

// ToolRegistry manages available Tools.
type ToolRegistry struct {
    tools map[string]ToolFactory
}

type ToolFactory func() tool.Tool

func NewToolRegistry() *ToolRegistry {
    return &ToolRegistry{tools: make(map[string]ToolFactory)}
}

func (r *ToolRegistry) Register(name string, factory ToolFactory) {
    r.tools[name] = factory
}

func (r *ToolRegistry) NewTool(name string) (tool.Tool, error) {
    factory, ok := r.tools[name]
    if !ok {
        return nil, fmt.Errorf("unknown tool: %s", name)
    }
    return factory(), nil
}

// PluginManager discovers and loads external plugins from disk or remote registries.
type PluginManager struct {
    pluginDir string
    registry  string // Remote registry URL
    loaded    map[string]*plugin.Client
}

func NewPluginManager(pluginDir string, registryURL string) *PluginManager {
    return &PluginManager{
        pluginDir: pluginDir,
        registry:  registryURL,
        loaded:    make(map[string]*plugin.Client),
    }
}

// DiscoverPlugins scans the plugin directory for executables and loads them.
func (pm *PluginManager) DiscoverPlugins(formatReg *FormatRegistry, toolReg *ToolRegistry) error {
    // Scan pluginDir for executables
    // For each: connect via go-plugin, query Info(), register in appropriate registry
}

// FetchPlugin downloads a plugin from the remote registry.
func (pm *PluginManager) FetchPlugin(name string, version string) error {
    // Download from pm.registry, verify checksum, place in pluginDir
}
```
