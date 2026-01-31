# gokapi: Implementation Phases

Each phase is self-contained and can be handed to Claude as an executable task. Phases build on each other sequentially.

---

## Phase 1: Core Framework

**Goal:** Establish the foundational types, interfaces, and flow execution engine.

**Dependencies:** None (first phase).

### Deliverables

#### 1.1 Go module initialization

```bash
go mod init github.com/<org>/gokapi
```

Dependencies to add:
- `github.com/spf13/viper` (configuration)
- `github.com/spf13/cobra` (CLI framework — used in Phase 6, add early)
- `golang.org/x/text` (Unicode/locale support)
- `golang.org/x/sync` (errgroup for concurrent flow stages)

#### 1.2 Content model (`core/model/`)

| File | Types | Description |
|---|---|---|
| `part.go` | `PartType`, `Part`, `PartResult` | Stream unit, type enum, error-paired result |
| `block.go` | `Block` | Translatable content with Source/Targets |
| `layer.go` | `Layer` | Structural grouping (document/section/embedded content). Layers nest for embedded content with different formats. |
| `segment.go` | `Segment` | Individual segment within a Block's source or targets |
| `fragment.go` | `Fragment` | Text with coded span markers |
| `span.go` | `Span`, `SpanType` | Inline markup element |
| `data.go` | `Data` | Non-translatable document structure |
| `media.go` | `Media` | Binary/media content |
| `group.go` | `GroupStart`, `GroupEnd` | Structural group markers within a Layer |
| `rawdocument.go` | `RawDocument` | Unprocessed input document |
| `skeleton.go` | `Skeleton`, `SkeletonStrategy`, `SkeletonPart`, `SkeletonText`, `SkeletonRef` | Document structure preservation |
| `locale.go` | `LocaleID`, locale constants and utilities | BCP 47 language tags |
| `annotation.go` | `Annotation` interface, `AltTranslation` | Extensible metadata |
| `resource.go` | `Resource` interface | Common interface for all Part payloads |

#### 1.3 Data format interfaces (`core/format/`)

| File | Types | Description |
|---|---|---|
| `reader.go` | `DataFormatReader` interface | Document → Part stream |
| `writer.go` | `DataFormatWriter` interface | Part stream → Document |
| `base_reader.go` | `BaseFormatReader` struct | Embeddable shared reader behavior |
| `base_writer.go` | `BaseFormatWriter` struct | Embeddable shared writer behavior |
| `config.go` | `DataFormatConfig` interface | Format configuration contract |
| `detect.go` | `FormatDetector`, `FormatSignature` | Multi-strategy format detection: MIME type mapping, extension, magic bytes, content sniffing |

#### 1.4 Tool interfaces (`core/tool/`)

| File | Types | Description |
|---|---|---|
| `tool.go` | `Tool` interface | Part channel processing |
| `base.go` | `BaseTool` struct | Event dispatch + default pass-through handlers |
| `config.go` | `ToolConfig` interface | Tool configuration contract |

#### 1.5 Flow execution (`core/flow/`)

| File | Types | Description |
|---|---|---|
| `flow.go` | `Flow` struct | Named sequence of Tools |
| `executor.go` | `FlowExecutor` interface, `DefaultFlowExecutor` | Goroutine + channel orchestration |
| `builder.go` | `FlowBuilder` | Fluent API for Flow construction |
| `item.go` | `FlowItem` | Batch item (input doc + output config) |

Key implementation: `DefaultFlowExecutor.processItem()` launches one goroutine per Tool, connects them with buffered channels, and uses `errgroup` for coordinated error handling.

#### 1.6 Registries (`core/registry/`)

| File | Types | Description |
|---|---|---|
| `format.go` | `FormatRegistry`, `FormatReaderFactory`, `FormatWriterFactory` | Format lookup and creation |
| `tool.go` | `ToolRegistry`, `ToolFactory` | Tool lookup and creation |

#### 1.7 Configuration (`core/config/`)

| File | Types | Description |
|---|---|---|
| `config.go` | `AppConfig` | Viper-based configuration loader |

### Tests

- `core/model/*_test.go`: Unit tests for all model types (Block creation with source/target segments, Layer nesting for embedded content, Fragment span encoding/decoding, Skeleton reconstruction)
- `core/format/detect_test.go`: FormatDetector — MIME mapping, extension lookup, content sniffing, cascade priority
- `core/flow/executor_test.go`: Flow execution with mock tools — verify channel wiring, error propagation, context cancellation
- `core/tool/base_test.go`: BaseTool dispatch — verify each PartType routes to correct handler

### Validation Criteria

- [ ] All model types instantiate and serialize to JSON correctly
- [ ] Layer nesting works: root Layer contains child Layers with different Format IDs
- [ ] Block source/target segments create, read, and modify correctly
- [ ] FormatDetector resolves formats by MIME type, extension, and content sniffing
- [ ] A Flow with 3 mock tools (pass-through) processes 100 Parts correctly
- [ ] Context cancellation stops all goroutines cleanly
- [ ] Error in any tool propagates and stops the flow

---

## Phase 2: Essential Native Data Formats

**Goal:** Implement 8 core data formats natively in Go.

**Dependencies:** Phase 1 (core framework).

### Formats to implement

| Format | Package | Okapi Equivalent | Priority | Complexity |
|---|---|---|---|---|
| Plain Text | `formats/plaintext/` | Plain Text Filter | 1 | Low |
| HTML | `formats/html/` | HTML Filter | 2 | Medium |
| XML | `formats/xml/` | XML Filter | 3 | Medium |
| XLIFF 1.2 | `formats/xliff/` | XLIFF Filter | 4 | Medium |
| XLIFF 2.0 | `formats/xliff2/` | XLIFF-2 Filter | 5 | Medium |
| JSON | `formats/json/` | JSON Filter | 6 | Low |
| YAML | `formats/yaml/` | YAML Filter | 7 | Low |
| PO | `formats/po/` | PO Filter | 8 | Low |
| Properties | `formats/properties/` | Properties Filter | 9 | Low |

### Per-format deliverables

Each format package contains:

```
formats/<name>/
├── reader.go         # DataFormatReader implementation
├── writer.go         # DataFormatWriter implementation
├── config.go         # DataFormatConfig implementation
├── reader_test.go    # Reader unit tests (roundtrip, extraction)
├── writer_test.go    # Writer unit tests
└── testdata/         # Test input files (port from Okapi test resources)
```

### Format registration

```go
// formats/register.go
package formats

import (
    _ "github.com/<org>/gokapi/formats/plaintext"
    _ "github.com/<org>/gokapi/formats/html"
    _ "github.com/<org>/gokapi/formats/xml"
    _ "github.com/<org>/gokapi/formats/xliff"
    _ "github.com/<org>/gokapi/formats/xliff2"
    _ "github.com/<org>/gokapi/formats/json"
    _ "github.com/<org>/gokapi/formats/yaml"
    _ "github.com/<org>/gokapi/formats/po"
    _ "github.com/<org>/gokapi/formats/properties"
)
```

Each format's `init()` registers itself with the global `FormatRegistry`.

### Implementation approach per format

**Plain Text** — Simplest format. Each line or paragraph is a Block. Skeleton is re-parse strategy. Good first format to validate the framework.

**HTML** — Use `golang.org/x/net/html` tokenizer. Extract text nodes and attribute values as Blocks. Inline tags (`<b>`, `<i>`, `<a>`) become Spans. Use fragment-based skeleton.

**XML** — Use `encoding/xml` decoder. Configurable extraction rules (which elements/attributes are translatable). Fragment-based skeleton.

**XLIFF 1.2** — Parse `<trans-unit>` elements. Source and target map directly to Block Source/Targets. Inline codes (`<g>`, `<x/>`, `<bx/>`, `<ex/>`) become Spans.

**XLIFF 2.0** — Parse `<unit>` and `<segment>` elements. More complex inline code model (`<pc>`, `<ph>`, `<sc>`, `<ec>`). Support notes and metadata.

**JSON** — Walk JSON tree, extract string values as Blocks. Key path becomes Block name. Support configurable key patterns for extraction. When a JSON string value contains HTML or other embedded markup, emit a child Layer with the appropriate Format so the content can be processed by a different DataFormatReader.

**YAML** — Similar to JSON. Use `gopkg.in/yaml.v3` for node-level access. Preserve comments. Support child Layers for embedded content in string values.

**PO** — Parse msgid/msgstr pairs. Support plural forms, context (msgctxt), and translator comments.

**Properties** — Parse key=value pairs. Support Unicode escapes, multiline values, comments.

### Tests — Porting from Okapi

For each format, port test cases from the Okapi Java test suite:
1. Clone Okapi repo: `git clone https://gitlab.com/okapiframework/Okapi.git`
2. Find test resources: `Okapi/filters/<format>/src/test/resources/`
3. Copy relevant test files to `formats/<name>/testdata/`
4. Translate Java test assertions to Go table-driven tests

**Roundtrip test pattern:**
```go
func TestRoundTrip(t *testing.T) {
    reader := NewReader()
    err := reader.Open(ctx, rawDoc("testdata/sample.html"))
    require.NoError(t, err)

    var parts []*model.Part
    for result := range reader.Read(ctx) {
        require.NoError(t, result.Error)
        parts = append(parts, result.Part)
    }
    reader.Close()

    // Write back
    var buf bytes.Buffer
    writer := NewWriter()
    writer.SetOutputWriter(&buf)
    writer.SetLocale(model.LocaleEnglish)

    ch := make(chan *model.Part, len(parts))
    for _, p := range parts { ch <- p }
    close(ch)

    err = writer.Write(ctx, ch)
    require.NoError(t, err)

    // Compare output with original
    assert.Equal(t, originalContent, buf.String())
}
```

### Validation Criteria

- [ ] Each format passes roundtrip tests (read → write → compare)
- [ ] Each format extracts expected Blocks with correct source segments from test files
- [ ] Each format preserves inline Spans correctly
- [ ] Formats with embeddable content (JSON, XML) emit child Layers correctly
- [ ] All formats register in FormatRegistry and can be looked up by name and extension
- [ ] End-to-end: read HTML → pass through identity Flow → write HTML produces identical output

---

## Phase 3: Plugin System

**Goal:** Implement HashiCorp go-plugin based plugin architecture for Data Formats and Tools.

**Dependencies:** Phase 1 (interfaces), Phase 2 (at least one native format for testing).

### Deliverables

#### 3.1 Protocol Buffers (`plugin/proto/v1/`)

| File | Services/Messages | Description |
|---|---|---|
| `common.proto` | `PartMessage`, `InfoRequest` | Shared types for Part serialization |
| `format.proto` | `DataFormatReaderPlugin`, `DataFormatWriterPlugin` | Format plugin services |
| `tool.proto` | `ToolPlugin` | Tool plugin service |

Generate Go code: `protoc --go_out=. --go-grpc_out=. plugin/proto/v1/*.proto`

Dependencies to add:
- `github.com/hashicorp/go-plugin`
- `google.golang.org/grpc`
- `google.golang.org/protobuf`

#### 3.2 Plugin Host (`plugin/host/`)

| File | Types | Description |
|---|---|---|
| `manager.go` | `PluginManager` | Discovers, launches, and manages plugin processes |
| `format_client.go` | `FormatReaderPluginClient`, `FormatWriterPluginClient` | gRPC clients wrapping plugin as native interfaces |
| `tool_client.go` | `ToolPluginClient` | gRPC client wrapping plugin as native Tool interface |
| `handshake.go` | `Handshake`, `PluginMap` | go-plugin configuration |

Key: Plugin clients implement `DataFormatReader`, `DataFormatWriter`, and `Tool` interfaces, so they're transparent to the rest of the framework.

#### 3.3 Plugin Server Helpers (`plugin/server/`)

| File | Types | Description |
|---|---|---|
| `format_server.go` | `FormatReaderGRPCServer`, `FormatWriterGRPCServer` | gRPC servers wrapping native implementations |
| `tool_server.go` | `ToolGRPCServer` | gRPC server wrapping native Tool implementations |
| `main_helper.go` | `ServeFormatReader()`, `ServeTool()` | Helper functions for plugin `main()` |

These helpers make it easy to turn any native format/tool into a plugin executable.

#### 3.4 Remote Plugin Registry (`plugin/registry/`)

| File | Types | Description |
|---|---|---|
| `remote.go` | `RemoteRegistry` | HTTP client for fetching versioned plugins |
| `manifest.go` | `PluginManifest` | Plugin metadata (name, version, checksum, platform) |

#### 3.5 Example plugin

Create an example external plugin to validate the architecture:

```
examples/plugin-format-csv/
├── main.go          # Plugin entry point using ServeFormatReader()
├── reader.go        # CSV DataFormatReader implementation
└── Makefile         # Build to executable
```

### Tests

- `plugin/host/manager_test.go`: Plugin discovery, launch, gRPC handshake
- `plugin/host/format_client_test.go`: Roundtrip through gRPC — native reader → serialize → deserialize → compare
- Integration test: run a Flow with a plugin-based format and verify output matches native format

### Validation Criteria

- [ ] Example CSV plugin builds as standalone executable
- [ ] PluginManager discovers and loads the CSV plugin
- [ ] CSV plugin appears in FormatRegistry and is usable in Flows
- [ ] Plugin crash is handled gracefully (host doesn't crash)
- [ ] Part serialization/deserialization through gRPC is lossless

---

## Phase 4: Complex Data Formats via Plugins

**Goal:** Provide access to complex document formats through the Java bridge and begin native Go implementations for high-value formats.

**Dependencies:** Phase 3 (plugin system).

### 4.1 Java Bridge (`plugin/bridge/`)

#### Java side (`plugin/bridge/java/`)

Maven project that:
1. Embeds all Okapi filter JARs as dependencies
2. Implements `DataFormatReaderPlugin` gRPC service
3. Maps between Okapi `Event`/`TextUnit` and gokapi `PartMessage`
4. Accepts filter class name as CLI argument

```
plugin/bridge/java/
├── pom.xml
└── src/main/java/dev/gokapi/bridge/
    ├── Main.java              # go-plugin entry point
    ├── GrpcServer.java        # gRPC server implementation
    ├── OkapiAdapter.java      # Okapi IFilter → gRPC streaming
    └── PartConverter.java     # Event/TextUnit ↔ PartMessage conversion
```

#### Go side (`plugin/bridge/`)

| File | Description |
|---|---|
| `java_bridge.go` | Launches JVM subprocess, manages lifecycle |
| `bridge_test.go` | Integration test: read DOCX through Java bridge |

#### Build

```bash
cd okapi-bridge && mvn package -q
# Produces: target/gokapi-okapi-bridge.jar
# Launched by: java -jar gokapi-okapi-bridge.jar --filter=<class>
```

### 4.2 Bridged Formats (available immediately via Java)

All 43 Okapi filters become available through the bridge. High-priority ones:

| Format | Okapi Filter Class | File Types |
|---|---|---|
| OpenXML | `net.sf.okapi.filters.openxml.OpenXMLFilter` | DOCX, XLSX, PPTX, VSDX |
| IDML | `net.sf.okapi.filters.idml.IDMLFilter` | Adobe InDesign |
| MIF | `net.sf.okapi.filters.mif.MIFFilter` | Adobe FrameMaker |
| DTD | `net.sf.okapi.filters.dtd.DTDFilter` | DTD files |
| PHP | `net.sf.okapi.filters.php.PHPContentFilter` | PHP content |
| Markdown | `net.sf.okapi.filters.markdown.MarkdownFilter` | Markdown (compare with native) |
| Regex | `net.sf.okapi.filters.regex.RegexFilter` | Custom regex extraction |
| Table | `net.sf.okapi.filters.table.TableFilter` | CSV, TSV |
| TTX | `net.sf.okapi.filters.ttx.TTXFilter` | Trados TagEditor |
| TXML | `net.sf.okapi.filters.txml.TXMLFilter` | Wordfast |
| TS | `net.sf.okapi.filters.ts.TsFilter` | Qt Translation |
| TMX | `net.sf.okapi.filters.tmx.TmxFilter` | Translation Memory |
| EPUB | `net.sf.okapi.filters.epub.EPUBFilter` | Electronic publications |
| Archive | `net.sf.okapi.filters.archive.ArchiveFilter` | ZIP-based formats |
| Wiki | `net.sf.okapi.filters.wiki.WikiFilter` | Wiki markup |
| Doxygen | `net.sf.okapi.filters.doxygen.DoxygenFilter` | Doxygen comments |
| TEX | `net.sf.okapi.filters.tex.TEXFilter` | LaTeX documents |
| Moses | `net.sf.okapi.filters.mosestext.MosesTextFilter` | Moses parallel text |
| RTF | `net.sf.okapi.filters.rtf.RTFFilter` | Tagged RTF |
| Vignette | `net.sf.okapi.filters.vignette.VignetteFilter` | Vignette CMS |
| SDL Packages | Various | SDL Trados packages |
| Pensieve | `net.sf.okapi.filters.pensieve.PensieveTMFilter` | Pensieve TM |

### 4.3 Native Go implementations (high-value)

Begin native Go implementations for formats where the Java bridge is overkill or performance matters:

| Format | Priority | Rationale |
|---|---|---|
| Markdown | High | Simple format, already have Go libraries (`goldmark`) |
| CSV/TSV | High | Simple tabular format |
| DTD | Medium | XML-adjacent, straightforward |
| TMX | Medium | Important for TM workflows |
| SRT/VTT | New | Subtitle formats — not in Okapi, high demand |

### Tests

- Java bridge integration: read a DOCX file, verify Blocks are extracted correctly
- Compare: read a file with both native format and Java bridge, verify identical Parts
- Performance: benchmark native vs. bridge for shared formats

### Validation Criteria

- [ ] Java bridge builds and launches successfully
- [ ] DOCX, XLSX, PPTX files produce correct Blocks through the bridge
- [ ] Bridge handles errors gracefully (missing Java, corrupt files)
- [ ] At least one new native format (Markdown or CSV) passes roundtrip tests
- [ ] Bridge-based formats appear in FormatRegistry alongside native formats

---

## Phase 5: AI Integration

**Goal:** Integrate LLM capabilities as first-class Tools.

**Dependencies:** Phase 1 (tool interface), Phase 2 (formats for testing).

### Deliverables

#### 5.1 LLM Provider Interface (`ai/provider/`)

| File | Types | Description |
|---|---|---|
| `provider.go` | `LLMProvider` interface, `TranslateRequest`, `TranslateResponse`, `Message` | Common LLM abstraction |
| `anthropic.go` | `AnthropicProvider` | Claude API integration |
| `openai.go` | `OpenAIProvider` | OpenAI API integration |
| `ollama.go` | `OllamaProvider` | Ollama local model integration |

Dependencies:
- `github.com/anthropics/anthropic-sdk-go` (Anthropic)
- `github.com/sashabaranov/go-openai` (OpenAI)

#### 5.2 AI Tools (`ai/tools/`)

| File | Tool Name | Description |
|---|---|---|
| `translate.go` | `ai-translate` | Translates untranslated Blocks using LLM |
| `qualitycheck.go` | `ai-qa` | Checks translations for fluency, accuracy, terminology |
| `terminology.go` | `ai-terminology` | Extracts terminology from Blocks |
| `review.go` | `ai-review` | Reviews translations with explanations |

#### 5.3 Prompt Templates (`ai/prompt/`)

| File | Description |
|---|---|
| `translate.go` | Translation prompts with context, glossary, and format awareness |
| `qa.go` | Quality check prompts with configurable check types |

### AI Translation Tool Design

```go
type AITranslateTool struct {
    tool.BaseTool
    provider     provider.LLMProvider
    sourceLocale model.LocaleID
    targetLocale model.LocaleID
    glossary     map[string]string
    batchSize    int // Number of Blocks to translate per LLM call
    skipMatched  bool // Skip Blocks that already have TM matches
}

func (t *AITranslateTool) HandleBlock(part *model.Part) (*model.Part, error) {
    block := part.Resource.(*model.Block)
    if !block.Translatable {
        return part, nil
    }
    if t.skipMatched && block.HasTarget(t.targetLocale) {
        return part, nil
    }

    // Build context-aware prompt
    prompt := t.buildPrompt(block)

    // Call LLM
    resp, err := t.provider.Translate(ctx, provider.TranslateRequest{
        Source:       block.SourceText(),
        SourceLocale: t.sourceLocale,
        TargetLocale: t.targetLocale,
        Glossary:     t.glossary,
        Context:      prompt.context,
    })
    if err != nil {
        return nil, fmt.Errorf("AI translation failed: %w", err)
    }

    // Set target
    block.SetTargetText(t.targetLocale, resp.Translation)

    // Add annotation with metadata
    block.Annotations["alt-translations"] = &model.AltTranslation{
        Target:    model.NewFragment(resp.Translation),
        Locale:    t.targetLocale,
        Origin:    "ai:" + t.provider.Name(),
        Score:     resp.Confidence,
        MatchType: "ai",
    }

    return part, nil
}
```

### AI QA Tool Design

```go
type AIQACheckTool struct {
    tool.BaseTool
    provider provider.LLMProvider
    checks   []string // ["terminology", "fluency", "accuracy", "consistency"]
}

func (t *AIQACheckTool) HandleBlock(part *model.Part) (*model.Part, error) {
    block := part.Resource.(*model.Block)
    if !block.HasTarget(t.targetLocale) {
        return part, nil
    }

    issues, err := t.provider.Chat(ctx, t.buildQAPrompt(block))
    // Add QA annotations to block
    return part, nil
}
```

### Tests

- Mock LLM provider for deterministic testing
- Test AI translate tool with mock: verify Block targets are set
- Test AI QA tool with mock: verify annotations are added
- Integration test with real LLM (optional, requires API key, run manually)

### Validation Criteria

- [ ] AI translate tool sets Block targets correctly with mock provider
- [ ] AI QA tool adds quality annotations
- [ ] AI tools compose in a Flow with other tools (segmentation → leverage → AI translate → QA)
- [ ] Glossary constraints are passed to LLM prompts
- [ ] Blocks with existing TM matches are skipped when configured

---

## Phase 6: kapi CLI

**Goal:** Build a Cobra-based CLI tool for localization tasks.

**Dependencies:** Phase 1-2 (core + formats), Phase 5 (AI tools, optional).

### Deliverables

#### 6.1 CLI Framework (`cmd/kapi/`)

```
cmd/kapi/
├── main.go
├── root.go          # Root command with global flags
├── extract.go       # Extract translatable content
├── merge.go         # Merge translations back
├── convert.go       # Convert between formats
├── flow.go          # Execute a configured Flow
├── translate.go     # AI-powered translation
├── formats.go       # List available formats
├── tools.go         # List available tools
├── plugins.go       # Plugin management (install, list, update)
└── version.go       # Version info
```

#### 6.2 Commands

| Command | Description | Example |
|---|---|---|
| `kapi extract` | Extract content to XLIFF | `kapi extract -i doc.html -o doc.xlf` |
| `kapi merge` | Merge translated XLIFF back | `kapi merge -i doc.xlf -o doc_fr.html` |
| `kapi convert` | Convert between formats | `kapi convert -i doc.po -o doc.xlf` |
| `kapi flow` | Execute a named Flow | `kapi flow run ai-translate -i *.html -t fr` |
| `kapi flow list` | List configured Flows | `kapi flow list` |
| `kapi translate` | Quick AI translation | `kapi translate -i doc.html -t fr --provider anthropic` |
| `kapi formats` | List available formats | `kapi formats` |
| `kapi tools` | List available tools | `kapi tools` |
| `kapi plugins install` | Install a plugin | `kapi plugins install openxml@1.2.0` |
| `kapi plugins list` | List installed plugins | `kapi plugins list` |
| `kapi plugins update` | Update plugins | `kapi plugins update` |
| `kapi version` | Show version | `kapi version` |

#### 6.3 Global flags

```
--config, -c     Config file path (default: gokapi.yaml)
--verbose, -v    Verbose output
--quiet, -q      Suppress output
--format, -f     Override input format detection
--encoding, -e   Override input encoding
--source-lang    Source language (BCP 47)
--target-lang    Target language (BCP 47)
```

### Tests

- Command integration tests using `cmd.Execute()` with test args
- Extract → merge roundtrip test for each format
- Flow execution via CLI test

### Validation Criteria

- [ ] `kapi extract -i sample.html -o sample.xlf` produces valid XLIFF
- [ ] `kapi merge -i sample.xlf -o sample_fr.html` produces correct output
- [ ] `kapi flow run` executes a configured Flow
- [ ] `kapi formats` lists all registered formats (native + plugins)
- [ ] `kapi plugins install` fetches and installs a plugin
- [ ] `kapi translate` performs AI translation end-to-end

---

## Phase 7: Bowrain UI

**Goal:** Build a desktop GUI for flow configuration and execution.

**Dependencies:** Phase 1-2 (core + formats), Phase 6 (kapi as reference).

### Technology Stack

- **[Wails v2](https://wails.io/)** — Go ↔ JavaScript bridge for desktop apps
- **Vite** — Build tool
- **React** — UI framework
- **TypeScript** — Type-safe frontend
- **TailwindCSS** — Utility-first styling
- **shadcn/ui** — Component library

### Deliverables

#### 7.1 Wails App (`apps/bowrain/`)

```
apps/bowrain/
├── main.go              # Wails app entry point
├── app.go               # Go backend (exposed to frontend)
├── build/               # Wails build config
│   └── appicon.png
├── wails.json           # Wails project config
└── frontend/
    ├── package.json
    ├── vite.config.ts
    ├── tsconfig.json
    ├── tailwind.config.js
    ├── components.json      # shadcn/ui config
    ├── index.html
    └── src/
        ├── App.tsx
        ├── main.tsx
        ├── components/
        │   ├── FlowEditor.tsx       # Visual flow builder
        │   ├── FormatSelector.tsx    # Format picker
        │   ├── ToolConfigurator.tsx  # Tool config forms
        │   ├── BatchManager.tsx     # Batch file management
        │   ├── OutputViewer.tsx     # Results viewer
        │   └── PluginManager.tsx    # Plugin install/update UI
        ├── pages/
        │   ├── FlowsPage.tsx
        │   ├── BatchPage.tsx
        │   ├── SettingsPage.tsx
        │   └── PluginsPage.tsx
        └── lib/
            ├── api.ts              # Wails bindings
            └── types.ts            # TypeScript types matching Go models
```

#### 7.2 Go Backend (`app.go`)

Exposes methods to the frontend via Wails bindings:

```go
type App struct {
    ctx       context.Context
    registry  *registry.FormatRegistry
    toolReg   *registry.ToolRegistry
    pluginMgr *plugin.PluginManager
    config    *config.AppConfig
}

// Exposed to frontend
func (a *App) ListFormats() []FormatInfo { ... }
func (a *App) ListTools() []ToolInfo { ... }
func (a *App) ListFlows() []FlowInfo { ... }
func (a *App) ExecuteFlow(name string, items []FlowItemDTO) error { ... }
func (a *App) InstallPlugin(name, version string) error { ... }
func (a *App) GetConfig() map[string]interface{} { ... }
func (a *App) SaveConfig(cfg map[string]interface{}) error { ... }
```

#### 7.3 Key UI Features

- **Flow Editor**: Visual drag-and-drop flow builder (arrange tools in sequence)
- **Format Selector**: Pick input/output formats with auto-detection
- **Batch Manager**: Add/remove files, configure per-file settings
- **Live Progress**: Real-time progress as flows execute
- **Plugin Manager**: Browse, install, update plugins from registry
- **Settings**: Configure providers, paths, defaults

### Build

```bash
# Development
cd apps/bowrain && wails dev

# Production build
wails build -platform darwin/arm64
wails build -platform windows/amd64
wails build -platform linux/amd64
```

### Validation Criteria

- [ ] Bowrain launches and displays the main UI
- [ ] Flow editor creates valid Flow configurations
- [ ] Batch processing works with file selection
- [ ] Plugin manager lists and installs plugins
- [ ] Builds produce native app bundles for macOS, Windows, Linux

---

## Phase 8: Advanced Features

**Goal:** REST server, translation memory, connectors, and additional tools.

**Dependencies:** Phases 1-6 (everything prior).

### 8.1 REST Server

Longhorn-equivalent REST API for remote flow execution.

```
cmd/gokapi-server/
├── main.go
├── handlers/
│   ├── flow.go         # POST /flows/{name}/execute
│   ├── formats.go      # GET /formats
│   ├── tools.go        # GET /tools
│   └── health.go       # GET /health
└── middleware/
    ├── auth.go
    └── logging.go
```

Framework: [Echo](https://echo.labstack.com/) or [Gin](https://gin-gonic.com/)

### 8.2 Pensieve Translation Memory

In-memory and persistent TM system.

```
lib/pensieve/
├── tm.go            # TranslationMemory interface
├── memory.go        # In-memory TM implementation
├── persistent.go    # SQLite-backed persistent TM
├── tmx_import.go    # TMX file import
├── tmx_export.go    # TMX file export
└── fuzzy.go         # Fuzzy matching algorithms
```

### 8.3 External Connectors

Translation service connectors as Tools:

| Connector | Package | Service |
|---|---|---|
| Google Translate | `connectors/google/` | Google Cloud Translation API |
| Microsoft Translator | `connectors/microsoft/` | Azure Cognitive Services |
| DeepL | `connectors/deepl/` | DeepL API |
| MyMemory | `connectors/mymemory/` | MyMemory TM API |
| ModernMT | `connectors/modernmt/` | ModernMT API |

### 8.4 Additional Tools

| Tool | Package | Description |
|---|---|---|
| SRX Editor | `tools/srxeditor/` | SRX segmentation rule management |
| XML Validation | `tools/xmlvalidation/` | XML schema validation |
| XSLT Transform | `tools/xslt/` | Apply XSLT transformations |
| Encoding Detection | `tools/chardet/` | Automatic encoding detection |
| Spell Check | `tools/spellcheck/` | Spelling verification |
| Terminology Check | `tools/termcheck/` | Terminology consistency checking |
| Pseudo Translation | `tools/pseudotranslate/` | Generate pseudo-translations for testing |

### 8.5 Additional Native Formats

Gradually replace Java bridge formats with native Go implementations:

| Format | Priority | Complexity |
|---|---|---|
| OpenXML (DOCX/XLSX/PPTX) | High | High (complex ZIP+XML structure) |
| EPUB | Medium | Medium (ZIP+XHTML) |
| SRT/VTT (subtitles) | Medium | Low |
| Markdown (enhanced) | Medium | Low |
| IDML | Low | High |
| MIF | Low | High |

### Validation Criteria

- [ ] REST API accepts and executes flows via HTTP
- [ ] Pensieve TM imports TMX and returns fuzzy matches
- [ ] At least one external connector works end-to-end
- [ ] Pseudo-translation tool generates output for all formats
