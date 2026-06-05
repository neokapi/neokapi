---
id: 006-tool-system
sidebar_position: 6
title: "AD-006: Tool System"
description: "Architecture decision: a Tool is a single composable pipeline stage — it reads Parts from an input channel and writes Parts to an output channel. BaseTool provides default pass-through; handlers set only the types they care about."
keywords: [tool system, BaseTool, pipeline stage, composable, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-006: Tool System

## Summary

A Tool is a single stage in a processing pipeline. It reads Parts from an
input channel and writes Parts to an output channel. Tools compose into
Flows; Flows are executed by the pipeline engine
([AD-004: Processing Engine](004-processing-engine.md)). The `BaseTool`
struct with optional handler fields — a capability-typed block handler
(`Annotate` / `Translate` / `Transform`) plus untyped `HandleDataFn`,
`HandleMediaFn` for other Part types — lets most tools implement only the
handler for the Part type they care about; everything else passes through
unchanged. The block handler a tool sets also declares what it may write
(see "Content immutability by capability" below). Tools
declare parameter schemas via `SchemaProvider`, which drives CLI flag
generation, flow-editor config panels, and validation. An IO contract on
`ToolMeta` declares locale cardinality, produced annotations, and side
effects so the runner can infer locale iteration and the flow editor can
show data flow.

## Context

Most tools only care about one or two Part types. A translation tool
processes Blocks; a word counter reads Blocks; a binary extractor handles
Media. Requiring every tool to implement the full `Process(ctx, in, out)`
method with a type switch over all Part types produces repetitive
boilerplate and creates risk of accidentally dropping Parts.

Beyond structural dispatch, a tool system needs to answer several questions
uniformly for CLI, flow editor, and plugin consumers:

- What parameters does this tool accept, and what are their types?
- How many locales does it operate on? Which ones?
- What annotations does it produce? Which does it consume?
- What external systems does it touch (TM, termbase, APIs)?

## Decision

### Tool interface and BaseTool dispatch

The core interface is minimal:

```go
type Tool interface {
    Process(ctx context.Context, in <-chan *Part, out chan<- *Part) error
}
```

`BaseTool` provides a standard dispatch shell. The block handler is one of
three capability-typed fields — the tool sets exactly one, and the parameter
type bounds what it may write:

```go
type BaseTool struct {
    Annotate  func(BlockView) error  // read-only: overlays/annotations/properties
    Translate func(TargetView) error // writes target
    Transform func(SourceView) error // rewrites source (and may write target)

    HandleDataFn  func(ctx context.Context, data *Data)   (*Data, error)
    HandleMediaFn func(ctx context.Context, media *Media) (*Media, error)

    SchemaFn func() *schema.ComponentSchema
}
```

`BaseTool.Process` reads Parts from the input channel, dispatches Blocks to
whichever capability-typed handler is set (and other Part types to their
`Handle*Fn`), and passes unhandled Part types through unchanged. Concrete tools
embed `BaseTool` and set only the handlers they need. A tool that needs the full
stream — batching, 1→N fan-out, cross-block state (e.g. the batch collector, the
concurrent ai-translate path) — overrides `Process` directly; it may reuse a
typed handler over a held block via `tool.NewBlockView`/`NewTargetView`/
`NewSourceView`.

### SessionTool extension

The channel-based `Tool.Process` is a forward-only transform. Some tools
need random access to the project's block state — lookup by content hash,
reading prior overlays (TM matches, QA findings, previously-produced
targets) to skip work that's already done, or writing annotations that
downstream tools in the same or a later run will consult. Those tools
opt into the `SessionTool` interface alongside `Tool`:

```go
type SessionTool interface {
    Tool

    SessionProcess(
        ctx context.Context,
        sess blockstore.Session,
        in <-chan *Part,
        out chan<- *Part,
    ) error
}
```

Lifecycle (owned by the executor, not the tool):

1. At flow start the executor opens a `blockstore.Session` against the
   project's declared store backend (`memory`, `cache`, remote — see
   [AD-008: Kapi Project Model](008-project-model.md)).
2. For each tool the executor calls `SessionProcess` when the tool
   implements `SessionTool`, otherwise the plain streaming `Process`.
   Hybrid implementations are allowed: `SessionProcess` can read from
   `in`, enrich via the session, and emit to `out`.
3. The executor commits the session on success or rolls back on error.
   Tools MUST NOT call `Commit` / `Rollback` themselves.

SessionTool is additive — every SessionTool also implements Tool so
flow composition (chaining steps that may or may not use the session)
keeps working. See [the SessionTool authoring guide](/contribute/notes-internal/session-tool-authoring)
for idiomatic patterns (skip-if-cached, overlay conventions, provider
selection).

### Tool categories

Tools fall into four categories that set expectations for idempotency and
ordering:

| Category      | Responsibility                  | Examples                                          |
| ------------- | ------------------------------- | ------------------------------------------------- |
| **Transform** | Modify content in place         | case change, search/replace, redaction            |
| **Enrich**    | Add metadata or overlays        | segmentation, TM leveraging, AI translation, terminology lookup |
| **Validate**  | Check quality without modifying | QA checks, word count, character count            |
| **Convert**   | Transform representations       | Encoding conversion, line-break normalization     |

### IO model

Each tool declares an IO contract in its `ToolMeta` (package `core/schema`).
Inputs/outputs are part-type name strings (`"block"`, `"data"`, `"media"`,
`"layer"`, `"group"`); the full struct also carries CLI and UI hints:

```go
// core/schema/schema.go
type ToolMeta struct {
    ID          string
    Category    string // "translate","validate","enrich","convert","transform","pipeline"
    DisplayName string
    Description string
    Inputs      []string // part-type names
    Outputs     []string
    Tags        []string

    // Requires declares external resources the tool needs at runtime.
    Requires []string // "target-language","tm","termbase","credentials",…

    // Cardinality declares how many locales the tool operates on per execution.
    Cardinality LocaleCardinality

    // DefaultLocale is an optional default for monolingual and bilingual tools.
    DefaultLocale model.LocaleID

    // Produces lists annotation types this tool writes to Blocks.
    Produces []AnnotationType

    // SideEffects lists external systems this tool reads from or writes to.
    SideEffects []SideEffect

    WritesOutput          bool     // CLI adds -o/--output when true
    DefaultParallelBlocks int      // concurrency for IO-bound tools
    Aliases               []string // alternative CLI command names
}
```

#### Locale cardinality

Tools declare how many locales they operate on per execution:

```go
type LocaleCardinality string

const (
    // Monolingual — operates on a single locale.
    // Examples: word-count (source), pseudo-translate (target),
    // encoding-detect (source).
    Monolingual LocaleCardinality = "monolingual"

    // Bilingual — operates on exactly two locales, provided as a pair.
    // Examples: ai-translate (source→target), qa-check (source vs target).
    Bilingual LocaleCardinality = "bilingual"

    // Multilingual — operates on N locales simultaneously.
    // Examples: translation-comparison, cross-locale QA.
    Multilingual LocaleCardinality = "multilingual"
)
```

Cardinality describes **how many** locales a tool needs. **Which** locales
are provided at runtime by the runner or flow configuration — never
hardcoded in the tool.

#### Uniform locale access

Blocks carry one source locale and N target locales. The source locale is
structurally distinct because it anchors the document skeleton and inline
code positions, but tools should not need to know whether a locale is
"source" or "target" — they just need text for a given locale:

```go
// Text returns the plain text for a locale: the source text if the
// locale matches the Block's source locale, otherwise the target text.
func (b *Block) Text(locale LocaleID) string

// SetText writes text for a locale (source if it matches the source
// locale, otherwise a target).
func (b *Block) SetText(locale LocaleID, text string)

// HasLocale reports whether the Block has content for the locale.
func (b *Block) HasLocale(locale LocaleID) bool
```

A bilingual tool comparing `[fr, de]` calls `block.Text("fr")` and
`block.Text("de")` — identical code whether `fr` is source or target.
`SourceText()` and `TargetText(locale)` remain available when a tool
specifically needs the source-anchored skeleton.

#### Annotation types and registry

Annotation types are typed string constants. The framework defines
well-known types; plugins register additional types via an annotation
registry:

```go
type AnnotationType string

const (
    AnnotationFindings       AnnotationType = "quality.findings"
    AnnotationTMMatch        AnnotationType = "leverage.tm-match"
    AnnotationAltTranslation AnnotationType = "leverage.alt-translation"
    AnnotationTerms          AnnotationType = "terminology.annotations"
    AnnotationTermEnforce    AnnotationType = "terminology.enforcement"
    AnnotationWordCount      AnnotationType = "analysis.word-count"
    AnnotationCharCount      AnnotationType = "analysis.char-count"
    AnnotationSegCount       AnnotationType = "analysis.seg-count"
    AnnotationEntityMapping  AnnotationType = "entity.mapping"
    AnnotationComparison     AnnotationType = "analysis.comparison"
)
```

Every checker — terminology, do-not-translate, placeholder, QA, brand
voice — writes the same `AnnotationFindings` ("quality.findings"), a
`core/check.FindingsAnnotation` carrying a `[]check.Finding` plus a
rolled-up score, so one scoring, annotation, and governance path serves
them all.

The `AnnotationRegistry` validates tool registrations: a tool declaring
`Produces: []AnnotationType{AnnotationFindings}` is rejected at
registration if the annotation type is unknown. This catches typos and
prevents silent production of unrecognized metadata at runtime.

#### Side effects

Side effects are a closed set of known external interactions:

```go
type SideEffect string

const (
    SideEffectTMRead        SideEffect = "tm-read"
    SideEffectTMWrite       SideEffect = "tm-write"
    SideEffectTermbaseRead  SideEffect = "termbase-read"
    SideEffectTermbaseWrite SideEffect = "termbase-write"
    SideEffectAPICall       SideEffect = "api-call"
    SideEffectAnalytics     SideEffect = "analytics"
)
```

Side-effect declarations are informational metadata for the flow editor
and documentation. They are not enforced at runtime — a tool with
`SideEffects: [SideEffectTMWrite]` still runs normally even if no TM is
configured (it simply skips the write). This keeps the tool interface
simple while giving the UI enough information to warn meaningfully.

#### Flow locale inference

The runner inspects the tool chain's cardinality declarations to determine
which locales to process:

```go
func ResolveFlowLocales(
    spec *StepsSpec,
    toolInfos map[registry.ToolID]registry.ToolInfo,
    sourceLocale string,
    projectTargets []string,
) [][]string
```

The runner passes the flow's `*StepsSpec` plus a map from `registry.ToolID` to
`registry.ToolInfo` (which carries each tool's cardinality and default-locale
metadata), not a `[]ToolMeta` slice.

Resolution returns a slice of locale sets — one set per execution pass.
Examples:

| Flow               | Tools                                         | Passes                                 |
| ------------------ | --------------------------------------------- | -------------------------------------- |
| word-count         | `[word-count(mono)]`                          | `[[en]]`                               |
| pseudo-translate   | `[pseudo-translate(bi, default:qps)]`         | `[[en, qps]]`                          |
| translate          | `[ai-translate(bi)]`                          | `[[en, de], [en, fr], [en, ja], ...]`  |
| translate+qa       | `[ai-translate(bi), qa-check(bi)]`            | `[[en, de], [en, fr], ...]`            |
| compare de vs fr   | `[comparison(bi)]` with config `[de, fr]`     | `[[de, fr]]`                           |
| cross-locale QA    | `[consistency-check(multi)]`                  | `[[en, de, fr, ja, nb, ar]]`           |
| translate + pseudo | `[ai-translate(bi), pseudo(bi, default:qps)]` | `[[en, de], [en, fr], ..., [en, qps]]` |

Mixed flows resolve to the union of all needed passes.

### Parameter schemas

Tools declare parameter schemas via the `tool.SchemaProvider` interface
with `ComponentSchema` in the `core/schema/` package:

```go
type SchemaProvider interface {
    Schema() *schema.ComponentSchema
}

type ComponentSchema struct {
    ID          string                    // "$id"
    Version     string                    // "$version"
    Title       string
    Description string
    Type        string                    // "object"
    ToolMeta    *ToolMeta                 // tool identity (see above)
    Groups      []ParameterGroup          // UI groupings ("ui:groups")
    StepMeta    *StepMeta                 // Okapi-bridge step metadata, when applicable
    Properties  map[string]PropertySchema // parameter definitions
    RawJSON     json.RawMessage           // full schema access
}
```

`schema.FromStruct(cfg, meta)` generates a `ComponentSchema` by reflecting
on a Go struct. It supports struct tags for additional metadata:

```go
type PseudoConfig struct {
    ExpansionPercent int    `schema:"description=Text expansion percentage,min=0,max=200"`
    Prefix           string `schema:"description=Prefix for pseudo text"`
    Suffix           string `schema:"description=Suffix for pseudo text"`
    InternalField    string `schema:"-"` // excluded from schema
}
```

`schema.ApplyConfig()` bridges `map[string]any` configuration (from flow
YAML) to a typed struct via JSON round-trip.

The `ToolRegistry` stores schemas alongside factories via
`RegisterWithSchema(name, factory, schema)`. All built-in tools register
auto-generated schemas.

Schema-driven features:

- **CLI flags** — `cli.RegisterSchemaFlags()` auto-generates Cobra flags
  from the schema, mapping camelCase properties to kebab-case flags.
- **Flow editor** — schema-driven config panels for tool nodes, reusing
  the same `FilterConfigEditor` component that drives format filter
  configuration.
- **Validation** — `ComponentSchema.Validate()` checks parameter values
  against the schema.
- **JSON export** — `kapi tools schema <name>` prints the schema for any
  tool.

AI tool schemas include provider fields (Provider, APIKey, Model with enum
support for provider selection), so AI-tool CLI flags are generated the
same way as any other tool's.

### Registration

Tools register into a `ToolRegistry` with a name, factory function, and
optional parameter schema:

```go
reg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
    return NewPseudoTranslateTool(&PseudoConfig{Prefix: "▒ ", Suffix: " ▒", TargetLocale: "qps"})
}, toolSchema(&PseudoConfig{Prefix: "▒ ", Suffix: " ▒"}, toolMeta("pseudo-translate", "Pseudo Translate", schema.CategoryTranslation, ...)))
```

The factory is a zero-argument `func() tool.Tool` (`registry.ToolFactory`); it
returns a tool built from a default config, with no error return. A separate
config factory (`SetConfigFactory`) builds the tool from a config map when flow
YAML overrides the defaults.

`RegisterAll(reg)` in `core/tools/register.go` auto-registers all built-in
tools. AI and MT tools are auto-registered separately by `aitools.RegisterAll`
and `mttools.RegisterAll` (`core/ai/tools`, `core/mt/tools`), called alongside
the built-ins during App init (`cli/app.go`). Each registers with a default
offline factory (the mock LLM provider for AI tools, the demo MT provider for
the `<provider>-translate` tools) plus a config factory (`SetConfigFactory`);
the real provider is resolved from the credential-bearing config map at
tool-creation time, not at registration time.

Plugin tools ([AD-007: Plugin System and Okapi Bridge](007-plugin-system.md))
use the same `Tool` interface via gRPC translation, so plugin-provided
tools and built-in tools are interchangeable from the pipeline's
perspective.

### Annotation-based communication

Tools communicate through annotations on Blocks. A typical pipeline:

<PipelineDiagram
  stages={[
    { label: "reader", role: "io" },
    { label: "ai-entity-extract", role: "annotate" },
    { label: "term-lookup", role: "annotate" },
    { label: "tm-leverage", role: "translate" },
    { label: "ai-translate", role: "translate" },
    { label: "term-enforce", role: "qa" },
    { label: "qa-check", role: "qa" },
    { label: "writer", role: "io" },
  ]}
/>

- `ai-entity-extract` adds `EntityAnnotation` with named entities.
- `term-lookup` adds `TermAnnotation` with matched terminology.
- `tm-leverage` reads entity annotations for generalized matching, adds `AltTranslation`.
- `ai-translate` reads term and entity annotations for context-aware translation.
- `term-enforce` validates terminology consistency in targets.
- `qa-check` validates translation quality.

Each tool reads the annotations it cares about and adds its own, keeping
tools loosely coupled through a shared data model rather than direct
dependencies.

### Built-in tool inventory

All built-in tools register via `RegisterAll()` in `core/tools/register.go`.

**Transform tools** — modify content in place:

| Tool                  | Description                                                               |
| --------------------- | ------------------------------------------------------------------------- |
| `pseudo-translate`    | Generate pseudo-translations with accent marks and prefix/suffix wrapping |
| `search-replace`      | Regex-based search and replace in content                                 |
| `case-transform`      | Transform case of source and/or target text                               |
| `create-target`       | Create a target for blocks, optionally copying the source runs            |
| `remove-target`       | Remove a locale's target (or all targets) from blocks                     |
| `inline-codes-remove` | Strip inline-code runs to produce clean plain text                        |
| `properties-set`      | Set or modify block properties programmatically                           |
| `whitespace-correct`  | Normalize and fix whitespace issues in translations                       |
| `span-classify`       | Reclassify `code:markup` spans into semantic vocabulary types             |
| `tag-protect`         | Identify and mark tags and placeholders for protection                    |
| `xslt-transform`      | Apply regex-based tag/text transformations to block text                  |
| `redact`              | Replace sensitive spans with placeholders pre-translation (recoverable source-transform) |
| `unredact`            | Restore redacted spans from the vault post-translation                    |

**Enrich tools** — add metadata or overlays via annotations:

| Tool                  | Description                                                                |
| --------------------- | -------------------------------------------------------------------------- |
| `segmentation`        | Annotate blocks with a sentence-segmentation overlay (SRX-like rules)      |
| `tm-leverage`         | Pre-fill translations from Sievepen TM                                     |
| `diff-leverage`       | Compare against previous version, preserve translations for unchanged text |
| `repetition-analysis` | Analyze source text repetitions across blocks in the pipeline              |

**Validate tools** — check quality without modifying:

| Tool                     | Description                                                                             |
| ------------------------ | --------------------------------------------------------------------------------------- |
| `word-count`             | Count words per block                                                                   |
| `char-count`             | Count characters per block                                                              |
| `segment-count`          | Count source and target segments in blocks                                              |
| `qa-check`               | Rule-based quality checks (missing translations, whitespace, numbers, span constraints) |
| `dnt-check`              | Flag do-not-translate spans that were translated in the target (alias `dnt`)            |
| `placeholder-check`      | Verify placeholders/variables are preserved between source and target                   |
| `brand-vocab-check`      | Check target text against brand vocabulary / preferred-term rules                       |
| `term-check`             | Verify terminology usage in translations against a glossary                             |
| `inconsistency-check`    | Check for translation inconsistencies across blocks                                     |
| `length-check`           | Verify translation length constraints                                                   |
| `chars-check`            | Check for invalid or unexpected characters in translations                              |
| `pattern-check`          | Validate regex patterns in translations (placeholders, variables)                       |
| `translation-comparison` | Compare translations across two target locales and report differences                   |
| `xml-validation`         | Validate XML well-formedness of block text                                              |
| `chars-listing`          | List all unique characters used in content (for font subsetting)                        |
| `scoping-report`         | Classify blocks into scoping categories based on repetition and match status            |

**Convert tools** — transform representations:

| Tool                | Description                                             |
| ------------------- | ------------------------------------------------------- |
| `encoding-convert`  | Convert character encoding of text content              |
| `encoding-detect`   | Detect encoding characteristics of block text           |
| `linebreak-convert` | Normalize line endings in source and/or target text     |
| `bom-convert`       | Add or remove the Unicode BOM marker on document layers |
| `fullwidth-convert` | Convert between half-width and full-width characters    |
| `uri-convert`       | Encode or decode URI escape sequences in text           |

**Pipeline tools** — operate on the part stream:

| Tool               | Description                                                              |
| ------------------ | ------------------------------------------------------------------------ |
| `layer-processor`  | Apply format-specific tool chains to child layers                        |
| `external-command` | Execute an external command on block text                                |
| `script`           | Run user-provided JavaScript (ES5 via goja) on each part                 |
| `batch`            | Collect blocks into configurable batches for downstream batch processing |

### AI, MT, and terminology tools

AI and MT tools are registered at startup like the other built-ins, so they
appear in `kapi tools` and resolve in flows. Their distinguishing trait is
provider injection: the registry holds a default offline-provider factory, and
the real LLM/MT provider (with credentials) is supplied on demand via the
config factory when the tool is instantiated. They use the same `Tool`
interface and work identically in flows.

**AI tools** (`core/ai/tools/`):

| Tool                | Description                                                        |
| ------------------- | ------------------------------------------------------------------ |
| `ai-translate`      | Translate blocks using an LLM provider (batch + concurrent)        |
| `ai-qa`             | Check translation quality using an LLM provider                    |
| `ai-review`         | Review translations with explanations using an LLM                 |
| `ai-terminology`    | Extract terminology from blocks using an LLM                       |
| `ai-entity-extract` | Extract named entities and term candidates using AI + optional NER |

**MT tools** (`core/mt/tools/`):

| Tool                   | Description                                                                          |
| ---------------------- | ------------------------------------------------------------------------------------ |
| `{provider}-translate` | Translate blocks using an MT provider (DeepL, Google, Microsoft, ModernMT, MyMemory) |

**Terminology tools** (`termbase/`):

| Tool           | Description                                         |
| -------------- | --------------------------------------------------- |
| `term-lookup`  | Annotate blocks with matching terms from a TermBase |
| `term-enforce` | Verify correct terminology usage in translations    |

**TM tools** (`sievepen/`):

| Tool          | Description                                                                |
| ------------- | -------------------------------------------------------------------------- |
| `tm-leverage` | Content-aware TM leverage with generalized, structural, and plain matching |

### Flow steps format

Flows are authored as a YAML step list (compiled to the internal graph by
the executor, see
[AD-004: Processing Engine](004-processing-engine.md)):

> The `input:` / `output:` fields are reframed as context-resolved `source:` /
> `sink:` bindings in [AD-026: Flow I/O Binding](026-flow-io-binding.md); a flow
> itself carries only its steps.

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  input: auto
  output: auto
  steps:
    - tool: tm-leverage
      config:
        fuzzyThreshold: 75
    - tool: ai-translate
      config:
        provider: anthropic
    - tool: qa-check
    - parallel:
        - tool: word-count
        - tool: char-count
```

Steps are sequential by default; `parallel:` blocks provide fan-out. The
`script` step lets authors drop in custom JavaScript when no existing tool
fits.

### Mutable streaming model

Tools modify Blocks in place as they flow through channels. This is a
deliberate trade-off:

- **Performance** — no copying or delta accumulation for high-volume
  streaming; zero allocation per tool for pass-through Part types.
- **Simplicity** — tools read and write fields on the same Block. No
  immutable builders, lenses, or patch application.
- **Proven pattern** — Okapi Framework uses the same mutable-event model
  across thousands of localization workflows.

Document-level immutability is achieved by external storage layers that
version entire Block states. Within a single pipeline execution, mutable
streaming is the right trade-off.

#### Content immutability by capability

Mutable-in-place does not mean anything goes. A tool's write surface is a
compile-time property: it declares what it may write by which process-named
block handler it sets on `BaseTool`, and the handler's parameter type makes the
wrong writes unrepresentable.

| Handler | View | May write |
| --- | --- | --- |
| `Annotate(BlockView)` | source + target read-only | overlays, annotations, properties |
| `Translate(TargetView)` | source read-only | target content (+ the above) |
| `Transform(SourceView)` | source writable | source content (+ the above) |

- **Analysis / annotation** tools (qa-check, word-count, term-lookup,
  entity-extract, the segmenter) set `Annotate`. `BlockView` exposes no
  source/target setter, so they *cannot* mutate content — they emit overlays,
  annotations, and properties.
- **Translation** tools (ai-translate, the MT tools, tm-leverage,
  create-target) set `Translate` and write `Block.Targets`; source stays
  read-only.
- **Source-transform** tools (redaction, normalization, case/encoding
  conversion) set `Transform` and may rewrite `Block.Source`. They belong in a
  flow's source-transform stage (below), which runs before any stand-off
  overlay is attached — run-anchored overlays (segmentation, terms, entities;
  see [AD-002](002-content-model.md)) would be invalidated by a later source
  rewrite. Recoverable transforms (redaction) record the original as a block
  annotation (or a sidecar vault) and restore it on the way out.

The read views hand back the block's live run slices, which Go cannot make
deeply immutable without copying. So a dev/test **backstop** in
`BaseTool.handleBlock` content-hashes source and targets around each handler and
errors if a handler edited a surface its tier forbids (catching in-place edits
through those aliased slices), or if a source transform ran while an overlay was
already attached. The backstop is gated by `tool.EnforceImmutability` (on by
default). A tool that genuinely needs the maximal surface — `script`, which runs
arbitrary JavaScript — overrides `Process` instead and self-gates source
mutation behind its `allowSourceMutation` flag.

#### The source-transform stage

A `Flow` has an optional leading **source-transform stage**: `Transform`-capable
tools that settle the source/model — redaction, a simplifier, normalization —
before the main tools run. The executor runs the stage ahead of the main tools
(`Flow.Pipeline()` concatenates them), so downstream tools — segmentation,
terminology, entities, translation, QA — all operate on one settled, canonical
source. The stage is **explicit**, not inferred from capability: a tool is
source-transform-*capable* (it can write source) independently of whether it is
*placed* early, so a `Transform` tool used late on the target (e.g.
`case-transform` configured for the target only) is never auto-hoisted. `Build`
rejects any tool in the stage that is not `tool.CapTransform`.

## Consequences

- Implementing a new tool requires only embedding `BaseTool` and setting
  one handler function field.
- Unhandled Part types pass through automatically; no risk of accidentally
  dropping Parts.
- Plugin tools use the same interface via gRPC translation, so the
  pipeline treats all tools uniformly
  ([AD-007: Plugin System and Okapi Bridge](007-plugin-system.md)).
- Schema-driven CLI flags, flow editor config panels, and validation all
  share one schema representation — changes to a tool's config propagate
  automatically.
- IO contracts enable flow-level locale inference: the runner figures out
  whether to iterate project targets, run once on source, or run for a
  specific locale set based on declared cardinality.
- Annotation-based inter-tool communication keeps tools loosely coupled
  through shared data, not direct dependencies.
- Typed constants for `AnnotationType`, `SideEffect`, and
  `LocaleCardinality` catch typos at compile time and enable IDE
  autocomplete.
- Mixed-cardinality flows resolve cleanly through pass union; tool
  authors do not coordinate locale iteration.

## Related

- [AD-002: Content Model](002-content-model.md) — Blocks, Annotations, and Fragment projections
- [AD-004: Processing Engine](004-processing-engine.md) — how Tools compose into Flows
- [AD-005: Format System](005-format-system.md) — readers and writers bracket the tool chain
- [AD-026: Flow I/O Binding](026-flow-io-binding.md) — a flow is composition only; tool = unit, binding = the ends
- [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md) — plugin tools
