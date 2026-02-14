---
sidebar_position: 1
title: Architecture
---

# gokapi: Architecture

gokapi is an open localization platform built in Go. Bidirectional connectors
sync content from live systems into a versioned store, composable tools process
content through a concurrent pipeline, and automation drives the workflow. For
the reasoning behind each major design choice, see the
[Architecture Decision Records](/docs/adr/index).

## Platform Architecture

```mermaid
graph TB
    subgraph "Source Systems"
        CMS[CMS]
        Design[Design Tools]
        Code[Code Repos]
        Marketing[Marketing]
        Files[Files]
    end

    subgraph "Connectors"
        C1[CMS Connector]
        C2[Design Connector]
        C3[Code Connector]
        C4[Marketing Connector]
        C5[FileConnector]
    end

    CMS --- C1
    Design --- C2
    Code --- C3
    Marketing --- C4
    Files --- C5

    subgraph "Content Store"
        CS[Versioned Store<br/>SHA-256 · dedup · history]
    end

    C1 -->|pull/push| CS
    C2 -->|pull/push| CS
    C3 -->|pull/push| CS
    C4 -->|pull/push| CS
    C5 -->|pull/push| CS

    subgraph "Processing Pipeline"
        direction LR
        DFR[Format Reader]
        T1[Segmentation]
        T2[TM Leverage]
        T3[AI Translation]
        T4[QA Check]
        DFW[Format Writer]
        DFR -->|"chan Part"| T1
        T1 -->|"chan Part"| T2
        T2 -->|"chan Part"| T3
        T3 -->|"chan Part"| T4
        T4 -->|"chan Part"| DFW
    end

    CS --> DFR
    DFW --> CS

    subgraph "Resources"
        TM[Translation Memory]
        TB[Terminology]
    end

    TM -.- T2
    TB -.- T4

    subgraph "Automation"
        EVT[Event Bus<br/>triggers · gates · webhooks]
    end

    CS -.- EVT
    EVT -.- DFR

    subgraph "Plugin System (gRPC)"
        P1[Native Go]
        P2[Java Bridge]
        P3[Remote Plugin]
    end

    P1 -.- T1
    P2 -.- DFR
    P3 -.- T3

    style T3 fill:#e1f5fe
    style CS fill:#fff3e0
    style EVT fill:#f3e5f5
```

Content flows from source systems through bidirectional connectors into a
versioned content store. The processing pipeline runs each tool in its own
goroutine, connected by buffered channels with automatic backpressure.
Event-driven automation triggers flows, enforces quality gates, and sends
notifications. See [ADR-001](/docs/adr/001-vision) and
[ADR-004](/docs/adr/004-processing-engine).

## Package Layout

```
gokapi/
├── core/                       # Model types, interfaces, registries
│   ├── model/                  # Part, Block, Layer, Fragment, Span, Data, Media
│   ├── format/                 # DataFormatReader/Writer interfaces, detection
│   ├── tool/                   # Tool interface, BaseTool dispatch
│   ├── flow/                   # FlowExecutor, FlowBuilder, FlowDefinition, FlowStore
│   ├── registry/               # FormatRegistry, ToolRegistry
│   ├── config/                 # AppConfig (layered YAML/env/flags)
│   ├── encoding/               # Text encoding utilities
│   └── kaz/                    # KAZ archive format
│
├── formats/                    # 15 built-in format implementations
│   ├── html/                   # Each has reader.go, writer.go, config.go
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
│   └── register.go            # init() registration
│
├── ai/                         # AI/LLM integration
│   ├── provider/               # LLMProvider: Anthropic, OpenAI, Ollama
│   ├── tools/                  # AI translate, QA, terminology, review
│   └── prompt/                 # Prompt templates
│
├── mt/                         # Machine translation
│   ├── provider/               # MTProvider: DeepL, Google, Microsoft, ModernMT, MyMemory
│   └── tools/                  # MT translate tool
│
├── lib/
│   ├── sievepen/               # Translation memory (in-memory + SQLite)
│   ├── termbase/               # Terminology management (in-memory + SQLite, TBX-inspired)
│   └── tools/                  # Utility tools (wordcount, pseudo-translate, segmentation, qa-check, tm-leverage, term-lookup, term-enforce, etc.)
│
├── plugin/
│   ├── host/                   # PluginManager, gRPC clients
│   ├── server/                 # gRPC server helpers (plugin side)
│   ├── bridge/                 # Java bridge: protocol, pool, format adapters
│   ├── loader/                 # Plugin discovery and loading
│   ├── registry/               # Multi-version plugin registry
│   ├── shared/                 # DTO types shared between host and bridge
│   └── proto/                  # Protobuf service definitions
│
├── cmd/
│   ├── kapi/                   # Cobra CLI
│   └── bowrain-server/          # Echo v4 REST API server
│
├── apps/
│   └── bowrain/                # Wails v3 desktop app (Go + React/TypeScript)
│
├── internal/testutil/          # Shared test helpers
└── docs/                       # Documentation and ADRs
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
        +[]Segment Source
        +map~LocaleID,[]Segment~ Targets
        +Skeleton Skeleton
    }

    class Segment {
        +string ID
        +Fragment Content
    }

    class Fragment {
        +string CodedText
        +[]Span Spans
        +Text() string
        +HasSpans() bool
    }

    class Span {
        +SpanType SpanType
        +string Type
        +string ID
        +string Data
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
    Block --> Segment : Source, Targets
    Segment --> Fragment : Content
    Fragment --> Span : Spans
```

Embedded content (HTML inside JSON, CDATA in XML) is modeled as nested
Layers, each with its own DataFormat. See
[ADR-002](/docs/adr/002-content-model).

### Inline Span Encoding

Fragments use coded text: inline markup is replaced by Unicode PUA markers
(U+E000-U+E0FF), with the actual markup stored in the Spans slice. This
allows string operations on text without corrupting markup.

```
Source HTML: "Click <b>here</b> for info"

Fragment:
    CodedText: "Click \uE001here\uE002 for info"
    Spans: [
        {SpanType: SpanOpening, Type: "bold", Data: "<b>"},
        {SpanType: SpanClosing, Type: "bold", Data: "</b>"},
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

| Okapi (Java)               | gokapi (Go)                |
|----------------------------|----------------------------|
| Filter                     | DataFormat (Reader/Writer)  |
| Step                       | Tool                       |
| Pipeline                   | Flow                       |
| PipelineDriver             | FlowExecutor               |
| Event                      | Part                       |
| TextUnit                   | Block                      |
| TextFragment               | Fragment                   |
| Code                       | Span                       |
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
type FlowExecutor interface {
    Execute(ctx context.Context, items []FlowItem) error
}

// AI providers
type LLMProvider interface {
    Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
    Chat(ctx context.Context, messages []Message) (*Message, error)
}
```

## Build and Distribution

| Channel | Target | Command |
|---------|--------|---------|
| Homebrew formula | kapi CLI | `brew install gokapi/tap/kapi` |
| Homebrew Cask | Bowrain GUI (macOS) | `brew install --cask gokapi/tap/bowrain` |
| GitHub Releases | All platforms | Direct download |
| Go install | Go developers | `go install github.com/gokapi/gokapi/cmd/kapi@latest` |

CI/CD runs via GitHub Actions: `ci.yml` (test, vet, lint, build on every
push) and `release.yml` (GoReleaser on tag push). See
[Release Process](/docs/developer/release) for details.
