---
id: 043-tool-io-model
sidebar_position: 43
title: "AD-043: Tool IO Contracts"
---

# AD-043: Tool IO Contracts

## Context

Tools in the processing pipeline ([AD-004](./004-processing-engine.md),
[AD-006](./006-tool-system.md)) operate on Blocks as they stream through
channels. Each Block carries source segments and a map of target segments
keyed by locale (`Targets map[LocaleID][]*Segment`). The data model
supports multiple target locales simultaneously — but the tool system
doesn't declare what each tool reads, writes, or requires.

Without IO declarations, the runner cannot determine whether a flow
should iterate all project target languages, run once for a fixed locale,
or run once with no target at all. The flow editor cannot show which
annotations a tool produces. And tools that need multiple target locales
(cross-locale comparison, multi-target QA) have no standard way to
express that requirement.

## Decision

### IO Contract on ToolMeta

Each tool declares an IO contract in its `ToolMeta` (the schema metadata
registered alongside the tool). The contract describes what the tool
consumes and produces at the Block level.

```go
type ToolMeta struct {
    // ... existing fields (ID, Category, DisplayName, Inputs, Outputs, Tags) ...

    // TargetMode declares how the tool interacts with target locales.
    TargetMode TargetMode

    // DefaultTargetLocale is the fixed target locale for TargetModeFixed tools.
    // Empty for all other modes.
    DefaultTargetLocale string

    // Produces lists the annotation types this tool writes to Blocks.
    Produces []AnnotationType

    // SideEffects lists external systems this tool reads from or writes to.
    SideEffects []SideEffect
}
```

### Typed Constants

`TargetMode`, `AnnotationType`, and `SideEffect` are typed string
constants. Using typed strings instead of raw `string` gives compile-time
safety (typos won't compile), discoverability via IDE autocomplete,
and JSON/YAML serializability for schemas and project files.

**Annotation types** — the framework defines well-known types as
constants. Plugins register additional types via an annotation registry.

```go
type AnnotationType string

const (
    AnnotationQAIssues       AnnotationType = "quality.qa-issues"
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

**Side effects** — a closed set of known external interactions:

```go
type SideEffect string

const (
    SideEffectTMRead       SideEffect = "tm-read"
    SideEffectTMWrite      SideEffect = "tm-write"
    SideEffectTermbaseRead SideEffect = "termbase-read"
    SideEffectTermbaseWrite SideEffect = "termbase-write"
    SideEffectAPICall      SideEffect = "api-call"
    SideEffectAnalytics    SideEffect = "analytics"
)
```

### Annotation Registry

The annotation registry provides validation at tool registration time
and discoverability for the flow editor:

```go
type AnnotationTypeInfo struct {
    Type        AnnotationType
    DisplayName string
    Description string
    Source      string // "built-in" or plugin name
}

type AnnotationRegistry struct {
    types map[AnnotationType]AnnotationTypeInfo
}

func (r *AnnotationRegistry) Register(info AnnotationTypeInfo)
func (r *AnnotationRegistry) Validate(t AnnotationType) error
func (r *AnnotationRegistry) List() []AnnotationTypeInfo
```

Built-in annotation types are registered during `RegisterAll`. Plugin
annotation types are registered during plugin metadata scanning. A tool
that declares `Produces: []AnnotationType{AnnotationQAIssues}` is
validated at registration time — if the annotation type is unknown, the
registration fails fast rather than silently producing unrecognized
metadata at runtime.

### Target Modes

```go
type TargetMode string

const (
    // TargetModeNone — tool reads source only, ignores targets.
    // Examples: word-count, segment-count, encoding-detect.
    // Runner: run once with no target locale.
    TargetModeNone TargetMode = "none"

    // TargetModeSingle — tool reads source and writes/validates one target.
    // The target locale is provided at runtime by the runner.
    // Examples: ai-translate, qa-check, tm-leverage.
    // Runner: iterate all project target languages.
    TargetModeSingle TargetMode = "single"

    // TargetModeFixed — like Single but with a built-in default locale.
    // The tool always targets this locale unless overridden.
    // Examples: pseudo-translate (qps).
    // Runner: run once for the default locale.
    TargetModeFixed TargetMode = "fixed"

    // TargetModeAll — tool reads source and all present targets.
    // Used for cross-locale operations.
    // Examples: translation-comparison, cross-locale QA, consistency-check.
    // Runner: run once after all per-target tools have populated targets.
    TargetModeAll TargetMode = "all"
)
```

### Flow Target Inference

The runner inspects the tool chain's `TargetMode` declarations to
determine which target locales to process:

```go
func ResolveFlowTargets(toolMetas []ToolMeta, projectTargets []string) []string
```

1. Collect `TargetMode` from each tool in the flow
2. Apply resolution rules:
   - If **all tools are `none`** → return `nil` (source-only flow, run once)
   - If **any tool is `single`** → include all `projectTargets`
   - If **any tool is `fixed`** → include its `DefaultTargetLocale`
   - If **any tool is `all`** → include all `projectTargets` (targets
     must be populated before this tool runs)
3. Return the deduplicated union

The flow runner calls `ResolveFlowTargets` before execution instead of
blindly iterating project target languages. No per-flow configuration
is needed — the tool chain's metadata determines the iteration strategy.

**Examples:**

| Flow | Tools | Resolved Targets |
|------|-------|------------------|
| pseudo-translate | `[pseudo-translate(fixed:qps)]` | `["qps"]` |
| translate | `[ai-translate(single)]` | `["de-DE","fr-FR","ja-JP","nb-NO","ar-SA"]` |
| translate-and-qa | `[ai-translate(single), qa-check(single)]` | `["de-DE","fr-FR","ja-JP","nb-NO","ar-SA"]` |
| word-count | `[word-count(none)]` | `nil` (run once) |
| compare | `[translation-comparison(all)]` | `["de-DE","fr-FR","ja-JP","nb-NO","ar-SA"]` |
| translate+pseudo | `[ai-translate(single), pseudo-translate(fixed:qps)]` | `["de-DE","fr-FR","ja-JP","nb-NO","ar-SA","qps"]` |

### Multi-Locale Tools

Tools with `TargetModeAll` receive Blocks where the `Targets` map is
already populated by earlier tools in the pipeline (or from previous
runs stored in the content store). These tools read multiple target
locales in a single pass:

```go
// Example: cross-locale consistency check
func (t *ConsistencyTool) handleBlock(block *Block) (*Block, error) {
    for locale, segments := range block.Targets {
        // Compare each target against source and other targets
    }
    // Produce annotations about cross-locale inconsistencies
    return block, nil
}
```

For flows that mix `single` and `all` tools, the runner processes
per-target tools first (populating individual targets), then runs
`all`-mode tools once on the fully-populated Blocks. This ordering
is implicit in the tool chain sequence — the flow author places
comparison/validation tools after translation tools.

### Annotation Production

The `Produces` field declares which annotation types a tool writes,
using typed `AnnotationType` constants. This serves three purposes:

1. **Flow editor** — shows what data flows between tools, enables
   connection validation (e.g., "qa-check produces `AnnotationQAIssues`,
   term-enforce consumes `AnnotationTerms`")
2. **Documentation** — auto-generated tool docs include output types
3. **Conflict detection** — warn if two tools in a flow produce the
   same annotation type (potential overwrite)

Annotation types follow the pattern `category.name` and are defined as
typed constants: `AnnotationQAIssues`, `AnnotationTMMatch`,
`AnnotationAltTranslation`, etc. Plugins register custom annotation
types via the `AnnotationRegistry` at scan time.

### Side Effects

Tools that interact with external systems declare their side effects
using typed `SideEffect` constants:

```go
Produces:    []AnnotationType{AnnotationTMMatch, AnnotationAltTranslation}
SideEffects: []SideEffect{SideEffectTMRead}
```

Side effect declarations are informational metadata for the flow editor
and documentation. They are not enforced at runtime — a tool with
`SideEffects: [SideEffectTMWrite]` still runs normally even if no TM is
configured (it simply skips the write). This keeps the tool interface
simple while giving the UI enough information to show meaningful
warnings ("this flow writes to TM — make sure one is configured").

The `SideEffect` type is a closed set of known external interactions.
Unlike annotation types which are extensible via the registry, side
effects represent infrastructure capabilities that the framework itself
provides (TM, termbase, API calls, analytics). Plugins that introduce
new infrastructure interactions add new `SideEffect` constants to the
framework.

### Mutable Streaming Model

Tools modify Blocks in place as they flow through channels. This is a
deliberate choice:

- **Performance**: no copying or delta accumulation for high-volume
  streaming. Parts flow through the pipeline with zero allocation per
  tool for pass-through Part types.
- **Simplicity**: tools read and write fields on the same Block object.
  No need for immutable builders, lenses, or patch application.
- **Proven pattern**: Okapi Framework uses the same mutable-event model
  in production across thousands of localization workflows.

The alternative — immutable Parts with delta accumulation (event
sourcing style) — would provide full audit trails and safe concurrency
but at significant complexity cost. The streaming pipeline already
provides ordering guarantees through channel semantics, and the tracing
system ([AD-004](./004-processing-engine.md)) records before/after
snapshots for debugging.

Immutability is achieved at the **document level** by the content store
([AD-003](./003-content-store.md)) which versions entire Block states.
Within a single pipeline execution, mutable streaming is the right
trade-off.

## Trade-offs

**Declarative IO vs. runtime validation.** IO contracts are metadata
declarations, not enforced types. A tool that declares `TargetMode:
none` can still access `block.Targets` — the contract is documentation
and tooling support, not a compile-time guarantee. This keeps the tool
interface simple (one `Process` method) while enabling flow validation
and runner inference.

**Target mode enum vs. arbitrary IO graphs.** Four target modes cover
the known use cases. A more expressive model (arbitrary input/output
port declarations like NiFi) would handle edge cases but adds
significant complexity to the flow editor and runner. The enum is
extensible — new modes can be added without changing the tool interface.

**Per-flow iteration vs. per-tool iteration.** The runner iterates
target locales at the flow level (all tools in a flow run for the
same target in each iteration). An alternative — per-tool target
selection — would allow different tools in the same flow to target
different locales independently. This is more flexible but makes flow
execution harder to reason about. The current model handles the common
case (translate all targets, then QA all targets) and mixed cases
(translate + pseudo in the same flow) through target union resolution.

**Side effects as metadata vs. capability system.** Side effects are
declared but not enforced. A richer model would use capability-based
injection (tools request TM access, runner provides it or rejects).
The metadata approach is simpler and sufficient for flow editor hints
and documentation.

**Typed constants vs. raw strings.** `AnnotationType`, `SideEffect`,
and `TargetMode` are typed string constants rather than raw `string`.
This catches typos at compile time, enables IDE autocomplete, and
provides a clear vocabulary of known values. The `AnnotationRegistry`
extends the closed built-in set for plugins that introduce custom
annotation types. A tool declaring `Produces: []AnnotationType{"typo"}`
fails at registration time, not silently at runtime.
