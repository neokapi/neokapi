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
supports multiple locales simultaneously — but the tool system doesn't
declare what each tool reads, writes, or requires.

Without IO declarations, the runner cannot determine whether a flow
should iterate target languages, run once, or run for a specific locale
set. The flow editor cannot show annotation data flow or warn about
conflicts. And tools that compare or validate across locales have no
standard way to express their requirements.

## Decision

### Locale Cardinality

Tools declare how many locales they operate on per execution using
three cardinality levels:

```go
type LocaleCardinality string

const (
    // Monolingual — tool operates on a single locale.
    // Examples: word-count (source), pseudo-translate (target),
    // encoding-detect (source), target normalization (target).
    Monolingual LocaleCardinality = "monolingual"

    // Bilingual — tool operates on exactly two locales.
    // The locales are provided at runtime as a pair.
    // Examples: ai-translate (source→target), qa-check (source vs target),
    // pivot comparison (fr vs de).
    Bilingual LocaleCardinality = "bilingual"

    // Multilingual — tool operates on N locales simultaneously.
    // The locale set is provided at runtime.
    // Examples: translation-comparison, cross-locale QA,
    // consistency-check, multi-target word count.
    Multilingual LocaleCardinality = "multilingual"
)
```

Locale cardinality describes **how many** locales a tool needs. **Which**
locales are always provided at runtime by the runner or flow
configuration — not hardcoded in the tool.

### Uniform Locale Access

Blocks carry one source locale and N target locales. The source locale
is structurally distinct because it defines the document skeleton, inline
code positions, and format round-tripping anchor. However, tools should
not need to know whether a locale is "source" or "target" — they just
need text for a given locale.

```go
// Text returns the segment text for a locale. Checks source first
// (if the locale matches the Block's source locale), then targets.
// Returns empty string if the locale has no segments.
func (b *Block) Text(locale LocaleID) string

// SetText writes segment text for a locale. Writes to source if the
// locale matches the Block's source locale, otherwise to targets.
func (b *Block) SetText(locale LocaleID, text string)

// HasLocale reports whether the Block has segments for a locale
// (source or target).
func (b *Block) HasLocale(locale LocaleID) bool
```

This gives tools a uniform API without changing the underlying storage
model. `SourceText()` and `TargetText(locale)` remain available for
tools that explicitly need the source-anchored skeleton or a specific
target, but most tools use `Text(locale)` and don't care about the
source/target distinction.

A bilingual tool comparing `[fr, de]` calls `block.Text("fr")` and
`block.Text("de")` — it doesn't need to know that `fr` might be a
target and `de` might also be a target, or that one of them might be
the source locale. The Block resolves the mapping internally.

### IO Contract on ToolMeta

Each tool declares an IO contract in its `ToolMeta`:

```go
type ToolMeta struct {
    // ... existing fields (ID, Category, DisplayName, Inputs, Outputs, Tags) ...

    // Cardinality declares how many locales the tool operates on.
    Cardinality LocaleCardinality

    // DefaultLocale is an optional default locale for monolingual and
    // bilingual tools. When set, the runner uses it if no locale is
    // specified. Example: pseudo-translate defaults to "qps".
    DefaultLocale string

    // Produces lists the annotation types this tool writes to Blocks.
    Produces []AnnotationType

    // SideEffects lists external systems this tool reads from or writes to.
    SideEffects []SideEffect
}
```

### Typed Constants

`LocaleCardinality`, `AnnotationType`, and `SideEffect` are typed string
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
    SideEffectTMRead        SideEffect = "tm-read"
    SideEffectTMWrite       SideEffect = "tm-write"
    SideEffectTermbaseRead  SideEffect = "termbase-read"
    SideEffectTermbaseWrite SideEffect = "termbase-write"
    SideEffectAPICall       SideEffect = "api-call"
    SideEffectAnalytics     SideEffect = "analytics"
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

### Flow Target Inference

The runner inspects the tool chain's cardinality declarations to
determine which locales to process:

```go
func ResolveFlowLocales(
    toolMetas []ToolMeta,
    sourceLocale string,
    projectTargets []string,
) [][]string
```

The return type is a slice of locale sets — one set per execution pass.
Each set contains the locales the tools receive for that pass.

**Resolution rules:**

1. If **all tools are monolingual with no default** and don't need a
   target → return `[[sourceLocale]]` (run once on source)
2. If **any bilingual tool has no default** → one pass per project
   target: `[[source, target1], [source, target2], ...]`
3. If **all bilingual tools have defaults** → one pass per unique
   default: `[[source, qps]]`
4. If **any multilingual tool** → one pass with all locales:
   `[[source, target1, target2, ...]]`
5. Mixed flows → union of all needed passes

**Examples:**

| Flow | Tools | Passes |
|------|-------|--------|
| word-count | `[word-count(mono)]` | `[[en]]` |
| pseudo-translate | `[pseudo-translate(bi, default:qps)]` | `[[en, qps]]` |
| translate | `[ai-translate(bi)]` | `[[en, de], [en, fr], [en, ja], ...]` |
| translate+qa | `[ai-translate(bi), qa-check(bi)]` | `[[en, de], [en, fr], ...]` |
| compare de vs fr | `[comparison(bi)]` with config `[de, fr]` | `[[de, fr]]` |
| cross-locale QA | `[consistency-check(multi)]` | `[[en, de, fr, ja, nb, ar]]` |
| translate+pseudo | `[ai-translate(bi), pseudo(bi, default:qps)]` | `[[en, de], [en, fr], ..., [en, qps]]` |

### Source as a Tagged Locale

The Block's source locale is structurally special — it anchors the
document skeleton, inline codes, and format round-tripping. But from a
tool's perspective, it's just another locale with a tag.

The Block stores which locale is the source via `SourceLocale LocaleID`.
The `Text(locale)` method uses this to resolve whether to read from
`Source` (the skeleton-anchored segments) or `Targets[locale]`. Tools
don't branch on this — they call `Text(locale)` with whatever locales
the runner provides.

This means a bilingual tool configured with `[de, es]` compares two
target locales without touching source. A bilingual tool configured with
`[en, fr]` compares source against a target. The tool code is identical
in both cases — only the locale pair differs.

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

**Monolingual/bilingual/multilingual vs. none/single/all.** The locale
cardinality model uses domain language from linguistics rather than
abstract CS terms. "Bilingual" immediately communicates that a tool
works with two locales — it doesn't say which two, or whether one is
source. This maps directly to how localization professionals think about
tools.

**Uniform `Text(locale)` vs. explicit `SourceText()`/`TargetText()`.** The
uniform API treats locales as peers at the tool level while preserving
structural asymmetry in storage. A bilingual tool comparing `[de, es]`
and one comparing `[en, fr]` use the same code. The cost: tools that
specifically need the source skeleton (format writers, inline code
validators) must still use `SourceText()`. Both APIs coexist.

**Source as storage vs. source as tag.** Source segments remain
structurally separate in the Block (`Source []*Segment` vs
`Targets map[LocaleID][]*Segment`) because the source anchors the
document skeleton and inline code positions. Making source just another
entry in the targets map would lose this structural guarantee. The
`Text(locale)` API abstracts over this for tools that don't need it.

**Declarative IO vs. runtime validation.** IO contracts are metadata
declarations, not enforced types. A tool that declares
`Cardinality: Monolingual` can still access multiple locales — the
contract is documentation and tooling support, not a compile-time
guarantee. This keeps the tool interface simple (one `Process` method)
while enabling flow validation and runner inference.

**Locale cardinality vs. arbitrary IO graphs.** Three cardinalities
cover the known use cases. A more expressive model (arbitrary
input/output port declarations like NiFi) would handle edge cases but
adds significant complexity to the flow editor and runner. The enum is
extensible — new cardinalities can be added without changing the tool
interface.

**Per-flow locale iteration vs. per-tool locale selection.** The runner
determines locale passes at the flow level. Tools in a bilingual flow
all receive the same locale pair per pass. An alternative — per-tool
locale selection within a pass — would let different tools target
different locale pairs independently. This is more flexible but makes
flow execution harder to reason about. The current model handles mixed
flows (translate + pseudo) through pass union resolution.

**Side effects as metadata vs. capability system.** Side effects are
declared but not enforced. A richer model would use capability-based
injection (tools request TM access, runner provides it or rejects).
The metadata approach is simpler and sufficient for flow editor hints
and documentation.

**Typed constants vs. raw strings.** `AnnotationType`, `SideEffect`,
and `LocaleCardinality` are typed string constants rather than raw
`string`. This catches typos at compile time, enables IDE autocomplete,
and provides a clear vocabulary of known values. The `AnnotationRegistry`
extends the closed built-in set for plugins that introduce custom
annotation types.
