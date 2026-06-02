# neokapi: Key Interface Definitions

This document defines the concrete Go interfaces and types that form the foundation of neokapi. These definitions serve as the contract for all implementations.

## Table of Contents

- [Content Model](#content-model)
- [Data Format Interfaces](#data-format-interfaces)
- [Tool Interfaces](#tool-interfaces)
- [Flow Interfaces](#flow-interfaces)
- [Configuration](#configuration)
- [Plugins](#plugins)
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
// Block is the primary translatable content unit (Okapi: TextUnit). Source is
// a single flat run sequence; translations are first-class Target records keyed
// by VariantKey; every interpretation of the runs (segmentation, terms, entities,
// QA, alignment) is a stand-off Overlay. There is no structural Segment type.
type Block struct {
    ID           string
    Name         string
    Type         string
    MimeType     string
    Translatable bool
    Skeleton     *Skeleton
    Source       []Run
    Targets      map[VariantKey]*Target
    Overlays     []Overlay
    Properties   map[string]string
    Annotations  map[string]Annotation
}

func (b *Block) ResourceID() string { return b.ID }

// SourceText returns the plain text of the source run sequence.
func (b *Block) SourceText() string { /* flat-text projection of Source runs */ }

// SourceRuns returns the canonical source run sequence.
func (b *Block) SourceRuns() []Run { return b.Source }

// SetSourceRuns replaces the source run sequence.
func (b *Block) SetSourceRuns(runs []Run) { b.Source = runs }

// SetSourceText replaces the source with a single plain-text Run.
func (b *Block) SetSourceText(text string) { /* b.Source = []Run{{Text: &TextRun{Text: text}}} */ }

// HasTarget returns true if a locale-keyed variant exists.
func (b *Block) HasTarget(locale LocaleID) bool { /* checks Targets[Variant(locale)] */ }

// TargetLocales returns the sorted list of locales that have a Target.
func (b *Block) TargetLocales() []LocaleID { /* sorted slice from Targets keys */ }

// Target returns the locale-only variant, or nil.
func (b *Block) Target(locale LocaleID) *Target { return b.Targets[Variant(locale)] }

// TargetRuns returns the target run sequence for a locale.
func (b *Block) TargetRuns(locale LocaleID) []Run { /* from Target(locale) */ }

// TargetText returns the plain text of the target for a locale.
func (b *Block) TargetText(locale LocaleID) string { /* flat-text projection */ }

// SetTargetRuns sets the target run sequence for a locale.
func (b *Block) SetTargetRuns(locale LocaleID, runs []Run) { /* upserts Targets[Variant(locale)] */ }

// SetTargetText sets the target as a single plain-text Run for a locale.
func (b *Block) SetTargetText(locale LocaleID, text string) { /* upserts Targets[Variant(locale)] */ }

// SetTargetVariant sets an arbitrary tone/channel variant Target.
func (b *Block) SetTargetVariant(key VariantKey, t *Target) { b.Targets[key] = t }

// SourceSegmentation returns the source segmentation Overlay, or nil if absent.
// Without one, the block is treated as a single implicit segment.
func (b *Block) SourceSegmentation() *Overlay { /* finds Overlay{Type: "segmentation", Variant: nil} */ }

// SourceSegmentCount returns the number of spans in the source segmentation
// overlay, or 1 for a non-empty block with no segmentation overlay.
func (b *Block) SourceSegmentCount() int { /* span count from SourceSegmentation */ }

// SourceSegmentRuns returns the run slice for the i-th source segment span.
func (b *Block) SourceSegmentRuns(i int) []Run { /* sub-slice from RunRange */ }

// SetSegmentation replaces the segmentation overlay for the given variant
// (nil = source side). Segmentation is stored as a stand-off Overlay —
// it does not alter the run sequence.
func (b *Block) SetSegmentation(variant *VariantKey, spans []Span) { /* upserts Overlays */ }

// VariantKey identifies a translation: locale plus optional tone and channel.
type VariantKey struct {
    Locale  LocaleID
    Tone    string // optional
    Channel string // optional
}

// Variant returns a locale-only VariantKey (no tone or channel).
func Variant(locale LocaleID) VariantKey { return VariantKey{Locale: locale} }

// Target is one translation: a flat run sequence with status and provenance.
type Target struct {
    Runs   []Run
    Status TargetStatus // e.g. "", "translated", "reviewed"
    Origin Origin       // tool/provider that produced it
    Score  float64
}

// Overlay is a typed stand-off layer over one side of a Block — the source
// (Variant nil) or a target variant — anchoring Spans to run-index ranges.
type Overlay struct {
    Type    OverlayType // "segmentation" | "term" | "entity" | "qa" | "alignment"
    Variant *VariantKey // nil = source side
    Spans   []Span
}

// Span is one entry in an Overlay: a run-anchored range with an optional id
// and type-specific props. RunRange is half-open [start, end) over the run
// slice, with intra-text-run rune offsets so boundaries survive inline-code
// insertions and edits.
type Span struct {
    ID    string
    Range RunRange
    Props map[string]string
}

type RunRange struct {
    StartRun, StartOffset, EndRun, EndOffset int
}
```

### Run (inline content)

A block's `Source` (and each `Target.Runs`) is a flat `[]Run`. Each `Run` is a
discriminated union — exactly one pointer field is set:

```go
// Run is one element of a flat inline-content sequence (core/model/run.go).
type Run struct {
    Text    *TextRun        // plain text chunk
    Ph      *PlaceholderRun // self-closing token: variable, <br>, icon
    PcOpen  *PcOpenRun      // opening half of a paired code (<b>, <a>, …)
    PcClose *PcCloseRun     // closing half of a paired code (</b>, </a>, …)
    Sub     *SubRun         // reference to a nested Block (subfilter output)
    Plural  *PluralRun      // ICU plural with per-form Runs
    Select  *SelectRun      // ICU select with per-case Runs
}

type TextRun struct {
    Text string
}

// PlaceholderRun is a self-closing inline code.
// PcOpenRun is identical in shape; PcCloseRun shares its ID with the
// matching PcOpen and inherits its behavior.
type PlaceholderRun struct {
    ID          string
    Type        string          // semantic type (e.g., "fmt:bold", "var")
    SubType     string
    Data        string          // original markup verbatim (e.g., "<br/>")
    Equiv       string          // plain-text equivalent (e.g., "\n")
    Disp        string          // editor display label
    Constraints *RunConstraints
}

type RunConstraints struct {
    Deletable   bool
    Cloneable   bool
    Reorderable bool
}
```

A `Run` serializes to JSON as an object with exactly one of the keys `text`,
`ph`, `pcOpen`, `pcClose`, `sub`, `plural`, or `select`.

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
    Source    []Run
    Target    []Run
    Locale    LocaleID
    Origin    string  // Where this translation came from (TM, MT, etc.)
    Score     float64 // Match quality (0.0 - 1.0)
    MatchType string  // "exact", "fuzzy", "mt", "ai"
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

// Detector determines the data format of a document using multiple strategies.
type Detector struct {
    registry *FormatRegistry
}

// Detect tries all strategies: explicit MIME → extension → content sniffing.
func (d *Detector) Detect(path string, reader io.ReadSeeker, mimeType string) (string, error)

// DetectByMIME maps a MIME type to a registered format name.
func (d *Detector) DetectByMIME(mimeType string) (string, error)

// DetectByExtension maps a file extension to a registered format name.
func (d *Detector) DetectByExtension(ext string) (string, error)

// DetectByContent reads the first N bytes and matches against registered signatures.
func (d *Detector) DetectByContent(reader io.ReadSeeker) (string, error)

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

### BaseTool (embedding target with part-type dispatch)

`BaseTool` implements `Process` once and dispatches each Part to the matching
handler. Embed it and set only the handlers you need; unset handlers pass the
Part through unchanged. For Blocks, set exactly **one** capability-typed
handler — the parameter type bounds what the tool may write (AD-006):

```go
// PartHandler handles a single non-block Part.
type PartHandler func(part *model.Part) (*model.Part, error)

// BaseTool provides default pass-through behavior and capability-typed dispatch.
// Embed in concrete tools and set only the handler fields you need.
type BaseTool struct {
    ToolName        string
    ToolDescription string
    Cfg             ToolConfig
    SchemaFn        func() *schema.ComponentSchema

    // Block handlers — set exactly ONE:
    //   Annotate  — read-only view (overlays, annotations, properties)
    //   Translate — writes target; source is read-only
    //   Transform — rewrites source (and may write target)
    Annotate  func(BlockView) error
    Translate func(TargetView) error
    Transform func(SourceView) error

    // Other part-type handlers — all optional; unset = pass through.
    HandleDataFn       PartHandler
    HandleMediaFn      PartHandler
    HandleLayerStartFn PartHandler
    HandleLayerEndFn   PartHandler
    HandleGroupStartFn PartHandler
    HandleGroupEndFn   PartHandler
}

// BlockView ⊂ TargetView ⊂ SourceView are the read/write surfaces a block
// handler sees. BlockView reads source/target and writes overlays, annotations,
// and properties; TargetView adds SetTarget*; SourceView adds SetSource*. A
// tool needing batching, 1→N fan-out, or stream control overrides Process
// instead.
```

| Handler | View | May write |
| --- | --- | --- |
| `Annotate(BlockView)` | source + target read-only | overlays, annotations, properties |
| `Translate(TargetView)` | source read-only | target content (+ the above) |
| `Transform(SourceView)` | source writable | source content (+ the above) |

### Concrete Tool examples

An annotation tool (read-only, produces a segmentation overlay):

```go
// segmentation tool — sets Annotate because it only reads source runs
// and writes a stand-off segmentation overlay; it does not alter source content.
t := &tool.BaseTool{
    ToolName:        "segmentation",
    ToolDescription: "Applies SRX segmentation rules to translatable blocks",
}
t.Annotate = func(v tool.BlockView) error {
    if !v.Translatable() {
        return nil
    }
    spans := srxRules.Segment(v.SourceRuns())
    v.SetSegmentation(nil, spans) // nil variant = source side
    return nil
}
```

A translation tool (writes target):

```go
// ai-translate — sets Translate because it writes Block.Targets; source is read-only.
t := &tool.BaseTool{ToolName: "ai-translate"}
t.Translate = func(v tool.TargetView) error {
    translated, err := llm.Translate(ctx, v.SourceText(), targetLocale)
    if err != nil {
        return err
    }
    v.SetTargetText(targetLocale, translated)
    return nil
}
```

A source-transform tool (rewrites source):

```go
// redaction — sets Transform because it rewrites Block.Source.
// Source-transform tools run in a flow's leading source-transform stage,
// before any stand-off overlays are attached.
t := &tool.BaseTool{ToolName: "redaction"}
t.Transform = func(v tool.SourceView) error {
    redacted, vault := redact(v.SourceRuns())
    v.SetSourceRuns(redacted)
    v.SetAnnotation("redaction-vault", vault)
    return nil
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

// Executor orchestrates the execution of a Flow across batch items.
type Executor interface {
    // Execute runs the Flow over the given batch items.
    Execute(ctx context.Context, f *Flow, items []*Item) error
}

// Item represents a single document to process in a batch.
type Item struct {
    Input          *model.RawDocument
    OutputPath     string
    OutputEncoding string
    TargetLocale   model.LocaleID
}

// Collector accumulates results from processed documents.
// Implementations must be safe for concurrent use.
type Collector interface {
    Collect(ctx context.Context, item *Item, parts []*model.Part) error
    Result() (CollectorResult, error)
}

type CollectorResult struct {
    Name string
    Data interface{}
}

// ExecutorConfig holds configuration for the DefaultExecutor.
type ExecutorConfig struct {
    MaxConcurrency int         // 0 = runtime.NumCPU(); 1 = sequential
    ChannelSize    int         // default 64
    FailFast       bool        // default true
    Collectors     []Collector
}

// ExecutorOption is a functional option for configuring a DefaultExecutor.
type ExecutorOption func(*ExecutorConfig)

func WithMaxConcurrency(n int) ExecutorOption { ... }
func WithChannelSize(n int) ExecutorOption    { ... }
func WithFailFast(b bool) ExecutorOption      { ... }
func WithCollectors(c ...Collector) ExecutorOption { ... }

// DefaultExecutor runs tools concurrently using goroutines and channels.
// With MaxConcurrency > 1, documents are processed in parallel using
// a semaphore-bounded fan-out pattern with errgroup.
type DefaultExecutor struct {
    config ExecutorConfig
}

func NewExecutor(opts ...ExecutorOption) *DefaultExecutor {
    // defaults: MaxConcurrency=1, ChannelSize=64, FailFast=true
}

// Execute processes Items through the tool chain.
// When MaxConcurrency > 1 (or 0 for NumCPU), items are processed
// in parallel using a semaphore-bounded fan-out pattern.
func (e *DefaultExecutor) Execute(ctx context.Context, f *Flow, items []*Item) error
```

### Flow Builder

```go
// Builder provides a fluent API for constructing Flows.
type Builder struct {
    name  string
    tools []tool.Tool
    items []*Item
}

func NewFlow(name string) *Builder {
    return &Builder{name: name}
}

func (fb *Builder) AddTool(t tool.Tool) *Builder {
    fb.tools = append(fb.tools, t)
    return fb
}

func (fb *Builder) AddToolFactory(f ToolFactory) *Builder {
    fb.toolFactories = append(fb.toolFactories, f)
    return fb
}

func (fb *Builder) AddItem(input *model.RawDocument, outputPath string, targetLocale model.LocaleID) *Builder {
    fb.items = append(fb.items, &Item{
        Input:        input,
        OutputPath:   outputPath,
        TargetLocale: targetLocale,
    })
    return fb
}

func (fb *Builder) Build() *Flow {
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
// executor := flow.NewExecutor()
// executor.Execute(ctx, f, items)
//
// Usage (parallel, multiple documents with collector):
// wc := tools.NewWordCountCollector()
// f := flow.NewFlow("word-count").
//     AddToolFactory(func() (tool.Tool, error) {
//         return tools.NewWordCountTool(&tools.WordCountConfig{...}), nil
//     }).Build()
//
// executor := flow.NewExecutor(
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
//   registry: "https://plugins.neokapi.dev"
//
// flow:
//   channelBuffer: 64
//   workerPool: 4
```

---

## Plugins

Plugins are out-of-process binaries, not in-process Go interfaces. A plugin
ships as its own binary plus a `manifest.json` and is installed into a plugin
directory; `kapi` itself links no vendor-plugin code. There is no go-plugin
handshake, no `MagicCookieKey`, and no in-process `DataFormatReaderPlugin` /
`DataFormatWriterPlugin` / `ToolPlugin` gRPC services — those belonged to a
retired layer. Two documents own the plugin contract:

- The **in-process registry contract** — how a plugin binary wires its
  commands, MCP tools, formats, and recipe schema into the shared `cli.App`
  via `init()` registration — is described in the
  [plugin model](../../web/docs/docs/contribute/notes-internal/plugin-model.md)
  note.
- The **runtime transport** — manifest discovery, dispatch, and the A/B/C
  transport modes — is described in
  [AD-007: Plugin system](../../web/docs/docs/contribute/architecture/007-plugin-system.md).

### Discovery and dispatch (`cli/pluginhost`)

The host-side runtime lives in the shared CLI module under `cli/pluginhost`.
`Discover` reads `manifest.json` from each plugin root (no subprocess is
launched to enumerate), and `NewHost` folds the surviving plugins into
dispatch tables for commands, MCP tools, formats, and recipe-schema
extensions. A `manifest.json` declares the plugin name, version, binary, an
optional `daemon` block, and a `capabilities` block.

### Transport modes

A plugin is invoked through one of three modes, chosen per capability:

- **Mode A — one-shot command exec.** The plugin binary is run once per
  command invocation (e.g. `kapi push` dispatched to `kapi-bowrain`).
- **Mode B — MCP-over-stdio session.** The plugin serves MCP tools over a
  stdio session.
- **Mode C — gRPC daemon.** Long-lived format/tool/source-connector plugins
  (including the Okapi Java bridge) are served by a `DaemonPool` that lazily
  spawns one daemon subprocess per plugin and connects over a Unix-socket gRPC
  `BridgeService`. The service (`core/plugin/proto/v2/neokapi_bridge.proto`,
  package `neokapi.bridge.v2`) exposes three RPCs:

```protobuf
service BridgeService {
  // Process performs a complete document processing cycle (bidirectional stream).
  rpc Process(stream ProcessRequest) returns (stream ProcessResponse);

  // ProcessStep runs a single Okapi pipeline step over a stream of parts.
  rpc ProcessStep(stream StepRequest) returns (stream StepResponse);

  // Shutdown gracefully shuts down the bridge server.
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

The host translates between neokapi Parts and Okapi Events via
`core/plugin/protoconvert`. The Okapi Java bridge implementation lives in the
separate [okapi-bridge](https://github.com/neokapi/okapi-bridge) repository;
its wire protocol, batching, and daemon lifecycle are documented in the
[bridge protocol](../../web/docs/docs/contribute/notes-internal/plugin-bridge-protocol.md)
note.

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

// Detector returns a Detector backed by this registry.
func (r *FormatRegistry) Detector() *format.Detector {
    return &format.Detector{Registry: r}
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
```

> The registries above are sketches; the live API (`core/registry`) uses typed
> IDs (`FormatID`, `ToolID`) and carries schema/metadata. There is **no**
> `PluginManager` in the registry — external plugins are out-of-process binaries
> discovered and dispatched by `cli/pluginhost` (see [Plugins](#plugins) above),
> not loaded into the registry. Plugin-contributed formats and tools are folded
> into the dispatch tables by `pluginhost.NewHost`, not registered through a
> registry-level plugin manager.
