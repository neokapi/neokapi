# neokapi: Architecture

neokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/)
in Go. For the reasoning behind each major design choice, see the
[Architecture Decisions](ad/README.md).

## Architecture Diagram

```mermaid
graph TB
    subgraph Input
        RD[RawDocument]
    end

    subgraph "Data Format Layer"
        DFR[DataFormatReader]
        DFW[DataFormatWriter]
    end

    subgraph "Flow (Pipeline)"
        direction LR
        T1[Tool 1<br/>Segmentation]
        T2[Tool 2<br/>TM Leverage]
        T3[Tool 3<br/>AI Translation]
        T4[Tool N<br/>QA Check]
        T1 -->|"chan Part"| T2
        T2 -->|"chan Part"| T3
        T3 -->|"chan Part"| T4
    end

    subgraph Output
        OUT[Output Document]
    end

    RD -->|open| DFR
    DFR -->|"chan PartResult"| T1
    T4 -->|"chan Part"| DFW
    DFW --> OUT

    subgraph "Plugin System (go-plugin + gRPC)"
        P1[Native Go Format]
        P2[Okapi Bridge Format]
        P3[Native Go Tool]
        P4[AI Tool]
        P5[Remote Plugin]
    end

    subgraph "Format Registry"
        FR[FormatRegistry]
    end

    FR -.->|lookup| DFR
    FR -.->|lookup| DFW
    P1 -.-> FR
    P2 -.-> FR

    style T3 fill:#e1f5fe
    style P4 fill:#e1f5fe
```

Documents flow through a channel-based concurrent pipeline. Each tool runs in
its own goroutine. Buffered channels provide backpressure. See
[AD-004](ad/004-processing-engine.md).

## Package Layout

The project is a **multi-module monorepo** with two Go modules coordinated by
`go.work`: the **framework** (`github.com/neokapi/neokapi`) provides the
localization engine, and the **platform** (`github.com/neokapi/neokapi/bowrain`)
builds the full-stack application on top.

```
neokapi/                              ── Framework Module ──
├── go.mod                           # module github.com/neokapi/neokapi
├── go.work                          # workspace: use . and ./bowrain
│
├── model/                           # Part, Block, Layer, Run, Target, Overlay, Data, Media
├── format/                          # DataFormatReader/Writer interfaces, detection
├── tool/                            # Tool interface, BaseTool dispatch
├── flow/                            # Executor, Builder, FlowDefinition
├── registry/                        # FormatRegistry, ToolRegistry
├── encoding/                        # Text encoding utilities
├── locale/                          # BCP-47 locale handling
├── editor/                          # Block index serialization and preview generation
├── version/                         # Build version info
│
├── formats/                         # 15 built-in format implementations
│   ├── html/                        # Each has reader.go, writer.go, config.go
│   ├── xml/
│   ├── xliff/
│   ├── xliff2/
│   ├── json/
│   ├── yaml/
│   ├── po/
│   ├── properties/
│   ├── plaintext/
│   ├── markdown/
│   ├── csv/
│   ├── srt/
│   ├── vtt/
│   ├── tmx/
│   └── register.go                  # init() registration
│
├── ai/                              # AI/LLM integration
│   ├── provider/                    # LLMProvider: Anthropic, OpenAI, Ollama
│   ├── tools/                       # AI translate, QA, terminology, review
│   └── prompt/                      # Prompt templates
│
├── mt/                              # Machine translation
│   ├── provider/                    # DeepL, Google, Microsoft, ModernMT, MyMemory
│   └── tools/                       # MT translate tool
│
├── sievepen/                        # Translation memory (interface + in-memory)
├── termbase/                        # Terminology management (interface + in-memory)
├── tools/                           # Utility tools (wordcount, pseudo, segmentation, etc.)
│
├── plugin/                          # Plugin system
│   ├── host/                        # PluginManager, gRPC clients
│   ├── server/                      # gRPC server helpers (plugin side)
│   ├── bridge/                      # Okapi bridge: gRPC protocol, pool, format adapters
│   ├── loader/                      # Plugin discovery and loading
│   ├── registry/                    # Multi-version plugin registry
│   ├── shared/                      # DTO types shared between host and bridge
│   └── proto/                       # Protobuf service definitions
│
├── testutil/                        # Shared test helpers
│
├── bowrain/                         ── Platform Module ──
│   ├── go.mod                       # module github.com/neokapi/neokapi/bowrain
│   ├── config/                      # Viper-based AppConfig
│   ├── store/                       # ContentStore + PostgreSQL implementation
│   ├── auth/                        # OIDC, JWT, device flow authentication
│   ├── connector/                   # System connectors (CMS, file, git)
│   ├── project/                     # .kapi/ project model
│   ├── event/                       # Event bus, webhooks, automation
│   ├── service/                     # Auth, project, connector, flow services
│   ├── credentials/                 # Credential management
│   ├── server/                      # HTTP/gRPC server handlers
│   ├── storage/                     # Database migration utilities
│   ├── sievepen/                    # PostgreSQL TM implementation
│   ├── termbase/                    # PostgreSQL TermBase implementation
│   ├── proto/v1/                    # gRPC protobuf definitions
│   ├── cmd/
│   │   ├── kapi/                    # Cobra CLI
│   │   └── bowrain-server/          # Echo v4 REST API server
│   └── apps/
│       ├── bowrain/                 # Wails v3 desktop app (Go + React/TypeScript)
│       └── web/                     # SaaS web UI
│
├── docs/                            # Documentation, architecture decisions, notes
└── web/docs/                         # Docusaurus 3 documentation site
```

## Content Model

```mermaid
classDiagram
    class Part {
        +PartType Type
        +Resource Resource
    }

    class Resource {
        <<interface>>
        +ResourceID() string
    }

    class Layer {
        +string ID
        +string Name
        +string Format
        +Layer Parent
        +Skeleton Skeleton
    }

    class Block {
        +string ID
        +string Name
        +bool Translatable
        +[]Run Source
        +map~VariantKey,Target~ Targets
        +[]Overlay Overlays
        +Skeleton Skeleton
    }

    class Target {
        +[]Run Runs
        +TargetStatus Status
    }

    class Overlay {
        +OverlayType Type
        +[]Span Spans
    }

    class Run {
        +TextRun Text
        +PlaceholderRun Ph
        +PcOpenRun PcOpen
        +PcCloseRun PcClose
    }

    class Data {
        +string ID
        +Skeleton Skeleton
    }

    class Media {
        +string ID
        +string MimeType
        +[]byte Data
        +string URI
    }

    Part --> Resource
    Layer ..|> Resource
    Block ..|> Resource
    Data ..|> Resource
    Media ..|> Resource
    Layer --> Layer : child Layers (embedded content)
    Layer --> Block : contains
    Layer --> Data : contains
    Layer --> Media : contains
    Block --> Run : flat Source sequence
    Block --> Target : per variant
    Block --> Overlay : stand-off annotations
    Target --> Run : flat run sequence
```

Embedded content (HTML inside JSON, CDATA in XML) is modeled as nested
Layers, each with its own DataFormat. See
[AD-002](ad/002-content-model.md).

### Inline Content as Runs

A block's `Source` (and each `Target`) is a flat `[]Run` — a discriminated
union of typed `Run` values. Plain text is a `TextRun`; paired
inline markup becomes a `PcOpenRun` / `PcCloseRun` sharing an ID; standalone
placeholders (variables, `<br/>`, icons) are a `PlaceholderRun`. Inline
markup never lives inside the text string, so text operations cannot corrupt
it. There is no structural `Segment` type — segmentation is a stand-off
`Overlay` layered over the runs.

```
Source HTML: Click <b>here</b> for info

Block.Source: [
    {Text:    {Text: "Click "}},
    {PcOpen:  {ID: "1", Type: "fmt:bold", Data: "<b>"}},
    {Text:    {Text: "here"}},
    {PcClose: {ID: "1", Type: "fmt:bold", Data: "</b>"}},
    {Text:    {Text: " for info"}},
]
```

### Part Stream

```
DataFormatReader.Read(ctx) -> chan PartResult
    -> PartLayerStart  (format="json")
    -> PartBlock        (key: "title")
    -> PartLayerStart  (format="html")        <- embedded child
    -> PartBlock        ("Hello <b>world</b>") <- inside child
    -> PartLayerEnd    (format="html")
    -> PartBlock        (key: "footer")
    -> PartLayerEnd    (format="json")
    -> (channel closed)
```

## Terminology Mapping from Okapi

| Okapi (Java)               | neokapi (Go)               |
| -------------------------- | -------------------------- |
| Filter                     | DataFormat (Reader/Writer) |
| Step                       | Tool                       |
| Pipeline                   | Flow                       |
| PipelineDriver             | Executor                   |
| Event                      | Part                       |
| TextUnit                   | Block                      |
| TextFragment               | Run sequence (`[]Run`)     |
| Code                       | Run                        |
| Segment (structural)       | Span in a segmentation Overlay |
| StartSubDocument/SubFilter | Child Layer                |
| Tikal                      | kapi (CLI)                 |
| Rainbow                    | Bowrain (desktop app)      |

## Key Interfaces

```go
// Format layer
type DataFormatReader interface {
    Open(ctx context.Context, doc *RawDocument) error
    Read(ctx context.Context) <-chan PartResult
    Close() error
}

type DataFormatWriter interface {
    SetOutput(path string) error
    Write(ctx context.Context, parts <-chan *Part) error
}

// Tool layer
type Tool interface {
    Process(ctx context.Context, in <-chan *Part, out chan<- *Part) error
}

// Flow execution
type Executor interface {
    Execute(ctx context.Context, items []Item) error
}

// AI providers
type LLMProvider interface {
    Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
    Chat(ctx context.Context, messages []Message) (*Message, error)
}
```

## Build and Distribution

| Channel          | Target              | Command                                                         |
| ---------------- | ------------------- | --------------------------------------------------------------- |
| Homebrew formula | kapi CLI            | `brew install neokapi/tap/kapi-cli`                                 |
| Homebrew Cask    | Bowrain GUI (macOS) | `brew install --cask neokapi/tap/bowrain`                       |
| GitHub Releases  | All platforms       | Direct download                                                 |
| Go install       | Go developers       | `go install github.com/neokapi/neokapi/bowrain/cmd/kapi@latest` |

CI/CD runs via GitHub Actions: `ci.yml` (test, vet, lint, build on every
push) and `release.yml` (GoReleaser on tag push). See
[RELEASE.md](RELEASE.md) for the release process.
