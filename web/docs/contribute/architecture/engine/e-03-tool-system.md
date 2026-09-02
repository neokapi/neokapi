---
id: e-03-tool-system
sidebar_position: 3
title: "E-03: The tool system"
description: "A tool is a single composable pipeline stage reading Parts from an input channel and writing Parts to an output channel; BaseTool provides pass-through dispatch, and the handler a tool sets declares what it may write."
keywords: [neokapi, architecture decision, tool system, BaseTool, pipeline stage, capability, IO contract, tool group, schema, immutability]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# E-03: The tool system

## Summary

A tool is a single stage in a processing pipeline. It reads Parts from an input
channel and writes Parts to an output channel. Tools compose into flows; flows
are executed by the pipeline engine ([E-01](e-01-processing-engine.md)). The
`BaseTool` struct with optional handler fields (a capability-typed block handler,
`Annotate` / `Produce` / `Transform`, plus untyped handlers for other Part types)
lets most tools implement only the handler for the Part type they care about;
everything else passes through unchanged. The block handler a tool sets also
declares what it may write. Tools declare parameter schemas via
`SchemaProvider`, which drives CLI flag generation, flow-editor config panels,
and validation. An IO contract on `ToolMeta` declares locale cardinality, the
stand-off layers a tool produces, and its side effects, so the runner can infer
locale iteration and the flow editor can show data flow. A tool whose behaviour
comes from one of several interchangeable backends is a **tool group**: it
registers self-describing members with a default, and the user selects and
configures one member at a time.

## Context

Most tools only care about one or two Part types. A translation tool processes
Blocks; a word counter reads Blocks; a binary extractor handles Media. Requiring
every tool to implement the full `Process(ctx, in, out)` method with a type
switch over all Part types produces repetitive boilerplate and creates a risk of
accidentally dropping Parts.

Beyond structural dispatch, a tool system needs to answer several questions
uniformly for the CLI, the flow editor, and plugin consumers:

- What parameters does this tool accept, and what are their types?
- How many locales does it operate on? Which ones?
- What stand-off layers does it produce? Which does it consume?
- What external systems does it touch (content memory, terms, remote APIs)?

## Decision

### Tool interface and BaseTool dispatch

The core interface is minimal:

```go
type Tool interface {
    Process(ctx context.Context, in <-chan *Part, out chan<- *Part) error
}
```

`BaseTool` provides a standard dispatch shell. The block handler is one of three
capability-typed fields; the tool sets exactly one, and the parameter type
bounds what it may write:

```go
type BaseTool struct {
    Annotate  func(BlockView) error             // read-only: overlays/annotations/properties
    Produce   func(VariantView) error           // writes target
    Transform func(BlockView) (EditPlan, error) // read-only producer; the applier rewrites source

    HandleDataFn       PartHandler
    HandleMediaFn      PartHandler
    HandleLayerStartFn PartHandler

    SchemaFn func() *schema.ComponentSchema
}
```

`BaseTool.Process` reads Parts from the input channel, dispatches Blocks to
whichever capability-typed handler is set (and other Part types to their
`Handle*Fn`), and passes unhandled Part types through unchanged. Concrete tools
embed `BaseTool` and set only the handlers they need. A tool that needs the full
stream (batching, 1→N fan-out, cross-block state) overrides `Process` directly;
it may reuse a typed handler over a held block via `tool.NewBlockView` /
`tool.NewVariantView`.

### SessionTool extension

The channel-based `Tool.Process` is a forward-only transform. Some tools need
random access to the project's block state: lookup by content hash, reading
prior overlays (memory matches, check findings, previously-produced targets) to
skip work that is already done, or writing annotations that downstream tools in
the same or a later run will consult. Those tools opt into the `SessionTool`
interface alongside `Tool`:

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

Lifecycle is owned by the executor, not the tool:

1. At flow start the executor opens a `blockstore.Session` against the project's
   declared store backend ([C-01](../context/c-01-project-model.md)).
2. For each tool the executor calls `SessionProcess` when the tool implements
   `SessionTool`, otherwise the plain streaming `Process`. Hybrid implementations
   are allowed: `SessionProcess` can read from `in`, enrich via the session, and
   emit to `out`.
3. The executor commits the session on success or rolls back on error. Tools must
   not call `Commit` or `Rollback` themselves.

Every `SessionTool` also implements `Tool`, so flow composition keeps working
whether or not a step uses the session. See
[the SessionTool authoring guide](/contribute/implementation/engine/session-tool-authoring)
for idiomatic patterns.

### Tool categories

Every tool declares one of six canonical categories, which set expectations for
idempotency and ordering and drive the grouping in the reference and the config
UI:

| Category | Constant | Responsibility |
| --- | --- | --- |
| `translation` | `schema.CategoryTranslation` | produces target content |
| `quality` | `schema.CategoryQuality` | validates a target; produces check or term findings |
| `analysis` | `schema.CategoryAnalysis` | read-only metrics and reports |
| `text-processing` | `schema.CategoryTextProcessing` | rewrites, segments, or redacts source |
| `convert` | `schema.CategoryConvert` | format conversion |
| `pipeline` | `schema.CategoryPipeline` | composite or sub-pipeline |

### IO model

Each tool declares an IO contract in its `ToolMeta` (package `core/schema`). The
contract is expressed over **`IOPort`s** (typed stand-off layers of a Block,
[F-02](../foundations/f-02-content-model.md)) rather than over coarse part-type
names:
`Consumes` lists the ports a tool reads upstream and `Produces` the ports it
writes. An `IOPort`'s `Type` names an overlay type (`term`, `qa`, …), a
block-annotation type (`voice`, …), or a pseudo-port (`PortTarget` /
`PortSource`); its `Side` says which side it pertains to; and `Optional` marks a
consumed port as degradable rather than required.

```go
// core/schema/schema.go
type IOPort struct {
    Type     string     // overlay type, annotation type, or "target"/"source"
    Side     model.Side // source | target
    Optional bool       // consumed: degrades without it, does more with it
    Layer    string     // segmentation granularity; "" = primary
}

// PortTarget is the committed Target; PortSource is a rewritten source.
const (
    PortTarget = "target"
    PortSource = "source"
)

type ToolMeta struct {
    ID          string
    Category    string // one of the six canonical categories above
    DisplayName string
    Description string
    Tags        []string

    // Requires declares external resources the tool needs at runtime:
    // "target-language", "source-language", "memory", "terms", "credentials",
    // "retryable". A tool that declares one and does not get it cannot run.
    Requires []string

    // Accepts declares optional resources the tool uses when the run has them
    // ("memory"). A tool that declares one and does not get it still runs.
    Accepts []string

    // Cardinality declares how many locales the tool operates on per execution.
    Cardinality LocaleCardinality

    // DefaultLocale is an optional default for monolingual and bilingual tools.
    DefaultLocale model.LocaleID

    // Consumes / Produces are the IO contract. Non-Optional consumed
    // ports are hard requirements the flow validator enforces.
    Consumes []IOPort
    Produces []IOPort

    // SideEffects lists external systems this tool reads from or writes to.
    SideEffects []SideEffect

    // Recoverable marks a transformer that vaults the originals it removes
    // and restores them later (redaction); the placement pass holds it to
    // the remote-egress rule.
    Recoverable bool

    WritesOutput          bool     // CLI adds -o/--output when true
    DefaultParallelBlocks int      // concurrency for IO-bound tools
    Aliases               []string // alternative CLI command names
}
```

For example `recycle` optionally consumes source segmentation and produces a
memory-match annotation, an alternative translation, and `target`; `qa` requires
a `target` and produces the `qa` overlay. The flow loader uses these contracts
for data-flow validation: a flow whose tool needs a port that no upstream tool
or the source binding supplies is rejected at build
([E-04](e-04-flows-and-io-binding.md)).

`Requires` is also a construction hook. When a tool declares `terms`, the runner
opens the project's terms store and brackets the tool with the term stages:
`term-lookup` (package `terms`) runs `terms.Locate`, the one pass over the terms
store and the recipe's `term_rules:`, and records where declared terms are used
without grading them; enforcement then grades the uses by the gate's own policy.
When a tool declares `memory`, a content-memory provider is injected from
`--memory` or the project's own store; `Accepts: memory` asks for the same
provider without failing when there is none. These stages are built by the
runner rather than registered, so they do not appear in `kapi tools`; they are
how a declaration is honoured.

Every governed step takes its terminology under one recipe key, `term_rules:`,
as a list of `profile.TermRule`: one term, what to use instead, how hard it
bites, and an optional concept id that ties it to the terms store and the graph.
`profile.TermRuleMap` is the single projection into the map a prompt or a check
consumes, and that map feeds the context fingerprint a target's `Origin` records
([F-02](../foundations/f-02-content-model.md)).

#### Locale cardinality

Tools declare how many locales they operate on per execution:

```go
type LocaleCardinality string

const (
    // Monolingual: operates on a single locale.
    Monolingual LocaleCardinality = "monolingual"

    // Bilingual: operates on exactly two locales, provided as a pair.
    Bilingual LocaleCardinality = "bilingual"

    // Multilingual: operates on N locales simultaneously.
    Multilingual LocaleCardinality = "multilingual"
)
```

Cardinality describes **how many** locales a tool needs. **Which** locales are
provided at runtime by the runner or flow configuration, never hardcoded in the
tool.

#### Uniform locale access

Blocks carry one source locale and N target locales. The source locale is
structurally distinct because it anchors the document skeleton and inline code
positions, but tools should not need to know whether a locale is source or
target; they just need text for a given locale:

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
`block.Text("de")`: identical code whether `fr` is source or target.
`SourceText()` and `TargetText(locale)` remain available when a tool specifically
needs the source-anchored skeleton.

#### Units: one iterator over segmented and unsegmented blocks

A segment is a span in the segmentation overlay rather than a structural type
([F-02](../foundations/f-02-content-model.md)), so a tool that works per segment
would otherwise repeat the same branch: iterate the spans when a segmentation
overlay exists, treat the whole block as one unit when none does, and map each
per-unit write back into the right run range. The tool views (`core/tool/view.go`)
carry that branch once:

```go
type BlockView interface {
    // SourceUnits yields the source units of the given segmentation layer
    // (LayerPrimary = primary), or one whole-block unit when none is present.
    SourceUnits(layer string) iter.Seq[Unit]
}

type VariantView interface {
    BlockView
    // TargetUnits yields writable per-unit target production over the source
    // segmentation of the given layer, splicing each unit's runs back into the
    // block at the unit's range and preserving ignorable spans verbatim.
    TargetUnits(loc model.LocaleID, layer string) iter.Seq[WritableUnit]
}
```

Reads reuse `Anchor.ExtractRuns`; writes use the inverse splice, which respects
half-open ranges and `Span.Ignorable()`. A tool that wants the whole block keeps
using `SourceRuns()`; a tool that wants units opts into `SourceUnits("")`, on any
side and any named layer, and pairs naturally with the `alignment` overlay for
source-to-target unit correspondence.

#### Stand-off types and the payload registry

The stand-off types a tool consumes and produces are typed string constants
([F-02](../foundations/f-02-content-model.md)). Positional, run-anchored layers
use the `OverlayType` constants; block-scoped metadata uses the annotation-key
constants. Both an overlay span's `Value` and an annotation value are typed
payloads; the framework registers the well-known content payloads, and formats
and plugins register additional types and their constructors via one payload
registry (`model.RegisterPayload` / `model.NewPayload`):

```go
// Positional layers (Block.Overlays): core/model/overlay.go
const (
    OverlaySegmentation  OverlayType = "segmentation"
    OverlayTerm          OverlayType = "term"
    OverlayEntity        OverlayType = "entity"
    OverlayCheck            OverlayType = "qa"
    OverlayAlignment     OverlayType = "alignment"
    OverlayTermCandidate OverlayType = "term-candidate"
)

// Block-scoped metadata (Block.Annotations): core/model/annotation_access.go
const (
    AnnoNote           = "note"
    AnnoAltTranslation = "alt-translation"
    AnnoMemoryMatch    = "tm-match"
    AnnoWordCount      = "word-count"
    AnnoVoice          = "voice"
    // …char-count, seg-count, comparison, repetition, entity-mapping,
    //   term-enforcement, scoping-report
)
```

A handful of these string values (`"tm-match"` above, and the memory and terms
side effects below) keep spellings the prose no longer uses. Annotation keys and
side-effect ids are a wire and storage boundary, where renaming a value splits a
record rather than moving it. The Go identifiers carry the current vocabulary;
the strings behind them do not change.

Every checker (terminology, do-not-translate, placeholder, quality, voice)
writes the same `qa` overlay (a `core/check.FindingsAnnotation` payload carrying
a `[]check.Finding` plus a rolled-up score), so one scoring, annotation, and
governance path serves them all.

A tool's `Consumes`/`Produces` name these overlay and annotation types, or a
pseudo-port, so the same registry that discriminates a payload's concrete type on
the wire is the vocabulary the flow validator checks the IO contract against.

Three consequences follow from validating on the declared contract. A wrong
`Consumes`/`Produces` on a built-in tool breaks a real flow, so each tool's
declaration is audited against its actual reads and writes, with an end-to-end
test per built-in flow as the guardrail. Plugin tools
([E-05](e-05-plugin-system.md)) declare the same metadata over gRPC, and the
overlay and annotation vocabulary is open to them through `model.RegisterPayload`:
a plugin-defined type crosses the bridge by type name and JSON, and rehydrates
to its concrete type wherever the payload constructor is registered. Alignment
is the one relational overlay: it links a source span to a target span, so its
payload carries the counterpart range while the `Overlay` shape stays
single-sided.

#### Side effects

Side effects are a closed set of known external interactions:

```go
type SideEffect string

const (
    SideEffectMemoryRead  SideEffect = "tm-read"
    SideEffectMemoryWrite SideEffect = "tm-write"
    SideEffectTermsRead   SideEffect = "termbase-read"
    SideEffectTermsWrite  SideEffect = "termbase-write"
    SideEffectAPICall     SideEffect = "api-call"
    SideEffectAnalytics   SideEffect = "analytics"

    // RemoteSourceEgress marks a tool that sends source content to a remote
    // system, distinct from APICall: a local detector, terms, or
    // content-memory lookup must not carry it, every cloud-provider call must.
    SideEffectRemoteSourceEgress SideEffect = "remote-source-egress"
)
```

Most side-effect declarations are informational metadata for the flow editor and
the documentation. They are not enforced at runtime: a tool declaring a memory
write still runs normally with no content memory configured; it skips the
write. This keeps the tool interface simple while giving the UI enough
information to warn meaningfully. The one exception is `RemoteSourceEgress`: the
transformer placement pass keys a hard build error off it, and a tool whose
remoteness depends on configuration (a model tool pointed at a local runtime or
the offline demo provider) refines it away through its contract resolver.

#### Flow locale inference

The runner inspects the tool chain's cardinality declarations to determine which
locales to process. This is the single applicability-based answer to "which
locales does this flow run for": the CLI's project flow-run and the desktop
runner both resolve their locale passes through it, so the two surfaces cannot
disagree. Convergence (`kapi up`) intentionally answers a different question with
a need-based selection: only the locales still short of their ship gate run a
pass.

```go
func ResolveFlowLocales(
    spec *StepsSpec,
    toolInfos map[registry.ToolID]registry.ToolInfo,
    sourceLocale string,
    projectTargets []string,
) [][]string
```

The runner passes the flow's `*StepsSpec` plus a map from `registry.ToolID` to
`registry.ToolInfo`, which carries each tool's cardinality and default-locale
metadata.

Resolution returns a slice of locale sets, one set per execution pass:

| Flow               | Tools                                         | Passes                                 |
| ------------------ | --------------------------------------------- | -------------------------------------- |
| case-transform     | `[case-transform(mono)]`                      | `[[en]]`                               |
| pseudo-translate   | `[pseudo-translate(bi, default:qps)]`         | `[[en, qps]]`                          |
| translate          | `[translate(bi)]`                             | `[[en, de], [en, fr], [en, ja], ...]`  |
| translate+qa       | `[translate(bi), qa(bi)]`                     | `[[en, de], [en, fr], ...]`            |
| compare de vs fr   | `[comparison(bi)]` with config `[de, fr]`     | `[[de, fr]]`                           |
| cross-locale check | `[consistency-check(multi)]`                  | `[[en, de, fr, ja, nb, ar]]`           |
| translate + pseudo | `[translate(bi), pseudo(bi, default:qps)]`    | `[[en, de], [en, fr], ..., [en, qps]]` |

Mixed flows resolve to the union of all needed passes.

### Parameter schemas

Tools declare parameter schemas via the `tool.SchemaProvider` interface with
`ComponentSchema` in the `core/schema/` package:

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
    StepMeta    *StepMeta                 // bridge step metadata, when applicable
    Properties  map[string]PropertySchema // parameter definitions
    RawJSON     json.RawMessage           // full schema access
}
```

`schema.FromStruct(cfg, meta)` generates a `ComponentSchema` by reflecting on a
Go struct. It supports struct tags for additional metadata:

```go
type PseudoConfig struct {
    ExpansionPercent int    `schema:"description=Text expansion percentage,min=0,max=200"`
    Prefix           string `schema:"description=Prefix for pseudo text"`
    Suffix           string `schema:"description=Suffix for pseudo text"`
    InternalField    string `schema:"-"` // excluded from schema
}
```

`schema.ApplyConfig()` bridges `map[string]any` configuration (from flow YAML) to
a typed struct via a JSON round-trip.

The `ToolRegistry` stores schemas alongside factories via
`RegisterWithSchema(name, factory, schema)`. All built-in tools register
auto-generated schemas. Schema-driven features:

- **CLI flags**: `cli.RegisterSchemaFlags()` auto-generates Cobra flags from the
  schema, mapping camelCase properties to kebab-case flags.
- **Flow editor**: schema-driven config panels for tool nodes, reusing the same
  editor component that drives format configuration.
- **Validation**: `ComponentSchema.Validate()` checks parameter values against
  the schema.
- **JSON export**: `kapi tools schema <name>` prints the schema for any tool.
- **MCP exposure**: `host/mcp_tools.go` registers every CLI-visible tool on the
  `kapi mcp` stdio server, projecting the tool's schema plus a `text` input into
  the MCP input schema and running the tool over the supplied text. The exposed
  set is **scoped by mode**, mirroring the desktop's project/ad-hoc split: inside
  a kapi project only the tools the project declares are advertised, with the
  project's target language as the default; ad-hoc, the full set is exposed.
  Resource-wrapping helpers (voice profile, terms, content memory) stay
  hand-authored in `host/mcp_voice.go`.

Model-backed tool schemas include provider fields (provider, API key, model, with
enum support for provider selection), so their CLI flags are generated the same
way as any other tool's.

### Registration

Tools register into a `registry.ToolRegistry` with a name, factory function, and
optional parameter schema:

```go
reg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
    return NewPseudoTranslateTool(&PseudoConfig{Prefix: "▒ ", Suffix: " ▒", TargetLocale: "qps"})
}, toolSchema(&PseudoConfig{Prefix: "▒ ", Suffix: " ▒"},
   toolMeta("pseudo-translate", "Pseudo Translate", schema.CategoryTranslation, ...)))
```

The factory is a zero-argument `func() tool.Tool` (`registry.ToolFactory`); it
returns a tool built from a default config, with no error return. A separate
config factory (`SetConfigFactory`) builds the tool from a config map when flow
YAML overrides the defaults.

`RegisterAll(reg)` in `core/tools/register.go` auto-registers all built-in tools.
The model-backed tools are auto-registered separately by `aitools.RegisterAll`
and `mttools.RegisterAll` (`core/ai/tools`, `core/mt/tools`), called alongside
the built-ins during app init. Each registers with a default offline factory plus
a config factory; the real provider is resolved from the credential-bearing
config map at tool-creation time, not at registration time
([E-07](e-07-model-providers.md)).

Plugin tools ([E-05](e-05-plugin-system.md)) use the same `Tool` interface via
gRPC translation, so plugin-provided tools and built-in tools are
interchangeable from the pipeline's perspective.

### Tool groups (pluggable backends)

Some tools are a family of interchangeable backends rather than one
implementation: `segmentation` runs on SRX rules, UAX-29, the browser
`Intl.Segmenter`, an LLM, or an ML segmenter; `qa` runs deterministic rules or an
LLM judge; `translate` runs any model provider; `entity-extract` runs a local NER
model, an LLM, or both. Each backend carries its own configuration and the family
has a sensible default.

A **tool group** models this: a tool whose behaviour is provided by one of
several self-describing **members**, selected by a discriminator field, with a
default member and common config. The user picks a member and configures only
that one; the group never merges members' parameters together.

```go
reg.RegisterGroup(registry.ToolGroupDef{
    Name:          "segmentation",
    Discriminator: "engine",                  // the config key that selects the member
    Default:       "srx",
    Common:        commonSchema,              // discriminator + shared options + ToolMeta
    Members:       members,                   // each: Name, Label, Description, own Schema
    ConfigFactory: NewSegmentationFromConfig, // dispatches on the discriminator
})
```

Members are not separately-registered tools; they belong to the group, and the
candidate members already exist as domain sub-registries: segmentation engines
([M-02](../multilingual/m-02-segmentation.md)) and model providers
([E-07](e-07-model-providers.md)).

A member may carry its **own factory** (`ToolGroupMember.Factory`). The group's
registered `ConfigFactory` dispatches on the discriminator: a member with a
factory is built directly, the rest fall through to the group's factory. This is
the seam for **runtime-contributed members**: `RegisterGroup` defines the
built-in members, and `AddGroupMember(group, member)` appends one later
(recomposing the flat schema, `MemberSchema`, the `ToolInfo.Group` metadata, and
the dispatcher), so a source outside the group's own package can extend it
without that package knowing. The discriminator value selects the right backend
whether it is built-in or contributed.

For **plugin-contributed members**, segmentation wires its engines from the
manifest `segmenters[]` capability into the segmentation engine sub-registry over
a dedicated `Segment` RPC ([E-05](e-05-plugin-system.md)); segmentation members
are narrow sub-components (text → boundaries), not full tools. The other groups'
members are full block-processing tools; a manifest and daemon transport for
contributing those is **not yet built**.

A group is a single registry entry, so flat consumers are unaffected; it serves
two renderings from one definition:

- **Master-detail** (the config UI, the docs reference) reads `ToolInfo.Group`
  (members, default, discriminator) and `MemberSchema(group, member)`, and renders
  the common fields plus a member selector plus only the selected member's own
  schema. Nothing is merged or hidden.
- **Flat projection** (CLI flags, MCP input, and the registry's `Schema(id)`) is
  inherently flat (cobra flags and a single input schema cannot be selected into
  at parse time), so the group projects to one schema via
  `schema.ComposeVariants`: a discriminator `select` plus each member's fields,
  grouped and gated to their member. `ComposeVariants` is therefore a projection
  of a group, not the model.

The discriminator carries a default, so the bare tool works with no
configuration. Where a tool can infer its backend from another field (`qa` from
whether a provider is set), that inference is the fallback when the discriminator
is unset.

### Annotation-based communication

Tools communicate through overlays and annotations on Blocks. A typical pipeline:

<PipelineDiagram
  stages={[
    { label: "source", role: "io" },
    { label: "entity-extract", role: "annotate" },
    { label: "recycle", role: "translate" },
    { label: "translate", role: "translate" },
    { label: "term-check", role: "qa" },
    { label: "qa", role: "qa" },
    { label: "sink", role: "io" },
  ]}
/>

- `entity-extract` adds `OverlayEntity` with named entities and term candidates.
- `recycle` reads entity annotations for generalized matching and adds an
  alternative translation.
- `translate` reads term and entity annotations for context-aware translation.
- `term-check` verifies terminology consistency in the target.
- `qa` validates translation quality.

Each tool reads the annotations it cares about and adds its own, keeping tools
loosely coupled through a shared data model rather than direct dependencies.

### Built-in tool inventory

All built-in tools register via `RegisterAll()` in `core/tools/register.go`; the
model-backed ones register from `core/ai/tools` and `core/mt/tools`. The
authoritative list with every parameter is the generated
[Tool Reference](/tools); the categories below are the shape of it.

**Translation** (produce target content): `translate` (a model provider),
`recycle` (content memory, exact and fuzzy), `diff-leverage` (reuse from a
previous document version), `pseudo-translate` (deterministic
translation-readiness testing), `source-gate`.

**Quality** (validate without rewriting content): `qa` (rule-based, or an LLM
judge when a provider is set), `review` (per-translation score, assessment and
suggestion), `term-check`, `dnt-check` (alias `dnt`), `placeholder-check`,
`voice-check`, `voice-vocab-check`, `xml-validation`.

The `qa` checkset also carries length constraints (ratio and absolute character
and word limits), invalid or forbidden characters and charset conformance, regex
pattern rules (required and forbidden), and cross-block translation consistency,
rather than splitting each rule family into its own tool.

**Analysis** (read-only metrics and reports): `entity-extract`, `term-extract`,
`voice-infer`, `encoding-detect`.

**Text processing** (rewrite, segment, or redact): `case-transform`,
`search-replace`, `whitespace-correct`, `inline-codes-remove`, `span-classify`,
`tag-protect`, `properties-set`, `create-target`, `remove-target`,
`segmentation`, `media-refine`, `redact`, `unredact`, plus the stream-shaping
`batch`, `layer-processor`, `external-command`, and `script`.

Byte-level *output* style (BOM, newlines, charset) is writer configuration, not a
pipeline stage: set `output.bom` / `output.newline` / `output.encoding` under any
format's config and every writer applies them as a shared post-encode step
([E-02](e-02-format-system.md)).

`external-command` and `script` are the **exec class**: their job is to run code
the configuration names, rather than to transform content with code kapi ships.
They are ordinary tools on the command line (`kapi exec` runs both), but a
recipe cannot arm one silently. Which surfaces may name them, and how a recipe
asks, is [E-06](e-06-execution-trust.md).

### Model-backed tools

Model-backed tools are registered at startup like the other built-ins, so they
appear in `kapi tools` and resolve in flows. Their distinguishing trait is
provider injection: the registry holds a default offline-provider factory, and
the real provider, with credentials, is supplied on demand via the config factory
when the tool is instantiated. They use the same `Tool` interface and work
identically in flows. Translation is one `translate` tool across every backend
and quality one `qa` tool, with the backend chosen by `--provider`
([E-07](e-07-model-providers.md)).

### Flow steps format

Flows are authored as a YAML step list, compiled to the internal graph by the
executor ([E-01](e-01-processing-engine.md)):

> A flow's source and sink are context-resolved bindings
> ([E-04](e-04-flows-and-io-binding.md)), not fields of the flow document; the
> steps carry only the composition.

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  steps:
    - tool: recycle
      config:
        fuzzyThreshold: 75
    - tool: translate
      config:
        provider: anthropic
    - tool: qa
    - parallel:
        - tool: term-check
        - tool: xml-validation
```

Steps are sequential by default; `parallel:` blocks provide fan-out. The `script`
step lets authors drop in custom JavaScript when no existing tool fits.

### Mutable streaming model

Tools modify Blocks in place as they flow through channels. This is a deliberate
trade-off:

- **Performance**: no copying or delta accumulation for high-volume streaming;
  zero allocation per tool for pass-through Part types.
- **Simplicity**: tools read and write fields on the same Block. No immutable
  builders, lenses, or patch application.
- **Proven pattern**: the Okapi Framework, the Java content and translation
  framework neokapi reimagines, uses the same mutable-event model across
  thousands of production workflows.

Document-level immutability is achieved by external storage layers that version
entire Block states. Within a single pipeline execution, mutable streaming is the
right trade-off.

#### Content immutability by capability

A tool's write surface is a compile-time property: it declares what it may
write by which block handler it sets on `BaseTool`, and the handler's parameter
type makes the wrong writes unrepresentable.

| Handler | View | May write |
| --- | --- | --- |
| `Annotate(BlockView)` | source + target read-only | overlays, annotations, properties |
| `Produce(VariantView)` | source read-only | target content, plus the above |
| `Transform(BlockView)` | source + target read-only | an edit plan the framework applies to source |

- **Analysis and check** tools (`qa`, `term-check`, `entity-extract`, the
  segmenter) set `Annotate`. `BlockView` exposes no source or target setter, so
  they *cannot* mutate content; they emit overlays, annotations, and properties.
- **Target-producing** tools (`translate`, `recycle`, `create-target`) set
  `Produce` and write `Block.Targets`; source stays read-only.
- **Transformers** (redaction, normalization, case and encoding conversion) are
  the only tools that rewrite `Block.Source`, and they never do so directly. A
  transformer is a read-only **edit producer**: it inspects the block and returns
  an *edit plan*: a set of structured `model.RunEdit`s (a span → replacement
  map), any originals to vault (recoverable transformers such as redaction), or
  an opaque whole-block replacement for rewrites with no derivable mapping. A
  single framework-owned **applier** is the one place that mutates the block: it
  applies the edits, **rebases** the surviving run-anchored overlays once
  (`model.RemapOverlays`) so segmentation, terms, and entities follow the
  rewrite, vaults any secrets, and bounds-checks the result, all atomically. Because
  tool code holds no source setter, a transformer cannot corrupt run-anchoring or
  leak a secret; an opaque whole-block replacement drops the overlays it cannot
  rebase. Recoverable transformers keep the original in a block annotation or a
  sidecar vault and restore it on the way out.

The read views hand back the block's live run slices, which Go cannot make deeply
immutable without copying. So a **backstop** in `BaseTool`'s block dispatch
content-hashes source and targets around each handler and errors if a handler
edited a surface its capability forbids, catching in-place edits made through
those aliased slices. Hashing every block twice per handler is dev and test
tooling, not production work, so it is **off unless a caller asks for it**: an
executor opts a whole run in with `flow.WithImmutabilityCheck`, any dispatch path
opts in per context with `tool.WithImmutabilityCheck`, and the tool, tools, and
flow test suites turn it on for every test. The applier likewise asserts that
every *surviving* source overlay span still anchors **in bounds** against the
rewritten runs (`Block.SourceOverlaysInBounds`), so a rebase that left an overlay
dangling is rejected; that check is unconditional. A tool that genuinely needs
the maximal surface (`script`, which runs arbitrary JavaScript) overrides
`Process` instead and self-gates source mutation behind its own flag.

#### Transformer placement

Transformers and analyzers are ordinary steps in one ordered tool list; there is
no separate structural stage. Because the applier mutates inline and in order,
each transformer settles the source before later steps observe it, so analysis
that depends on a transform (segmentation over normalized text, or an annotator
feeding a redactor such as `entity-extract` → `redact`) sees the applied result.

Ordering safety is a **placement pass** that runs beside the data-flow contract,
using the capability and `SideEffects` a tool already declares:

| Severity | Rule | Rationale |
| --- | --- | --- |
| Error | a transformer must not follow a step that produces a committed target, unless it produces the target port itself (`unredact` rewrites both sides coherently) | rewriting source orphans the targets, which anchor to it |
| Error | a recoverable (redacting) transformer must run before any step that egresses source to a remote sink, except the step or steps producing an input its config-resolved contract *requires* | otherwise unprotected source leaks before redaction applies; a cloud NER feeding entity-driven redaction is the documented detection trade-off ([C-10](../context/c-10-redaction.md)) |
| Warning | a transformer placed later than its earliest valid slot (after its last required input) | every overlay present at apply time must be rebased; an earlier slot avoids the work |

The remote-egress rule keys off a *remote source egress* side effect, distinct
from a plain API call, so a local detector or term lookup does not trip it while
a cloud-provider call does. The effect itself is config-refined: a model tool
pointed at a local provider carries no remote egress. Tools, including plugins,
contribute their own placement diagnostics through the same config-derived
contract hook that resolves a tool's required inputs from its configuration. For
example redaction requires an upstream `entity` overlay only when entity
detection is enabled, and only a *required* input exempts its producer from the
egress rule, so a rules-only `redact` placed after a cloud NER step is still
rejected.

## Consequences

- Implementing a new tool requires only embedding `BaseTool` and setting one
  handler function field.
- Unhandled Part types pass through automatically; there is no risk of
  accidentally dropping Parts.
- Plugin tools use the same interface via gRPC translation, so the pipeline
  treats all tools uniformly ([E-05](e-05-plugin-system.md)).
- Schema-driven CLI flags, flow-editor config panels, and validation all share one
  schema representation, so a change to a tool's config propagates automatically.
- IO contracts enable flow-level locale inference: the runner works out whether to
  iterate project targets, run once on source, or run for a specific locale set
  from the declared cardinality.
- Annotation-based inter-tool communication keeps tools loosely coupled through
  shared data rather than direct dependencies.
- Typed constants for overlay types, side effects, and locale cardinality catch
  typos at compile time and enable IDE autocomplete.
- Mixed-cardinality flows resolve cleanly through pass union; tool authors do not
  coordinate locale iteration.
- The capability-typed handler makes the wrong write unrepresentable rather than
  merely discouraged, and the single applier means overlay rebasing and secret
  vaulting are written once rather than per transformer.
- Tool groups let a family of interchangeable backends share one tool while each
  backend keeps its own config; members come from domain registries or plugins,
  and the same definition drives the master-detail config UI and the flat
  CLI/docs/MCP projection.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): Blocks, overlays, annotations, and Fragment projections
- [E-01: The processing engine](e-01-processing-engine.md): how tools compose into flows
- [E-02: The format system](e-02-format-system.md): readers and writers bracket the tool chain
- [E-04: Flows and I/O binding](e-04-flows-and-io-binding.md): a flow is composition only; tool = unit, binding = the ends
- [E-05: The plugin system](e-05-plugin-system.md): plugin tools
- [E-06: Execution trust](e-06-execution-trust.md): the exec class and how a recipe arms it
- [E-07: Model and translation providers](e-07-model-providers.md): provider injection into model-backed tools
