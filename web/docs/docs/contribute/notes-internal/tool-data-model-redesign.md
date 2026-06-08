---
sidebar_position: 12
title: "Design Proposal: Tool & Data-Type Model Redesign"
description: "A design proposal (RFC) for redesigning the tool model around the data facets tools consume and produce — unifying overlays and annotations under one typed-schema registry, declaring optional/required data dependencies, adding a first-class segment iteration interface over stand-off overlays, and using the resulting contract to make flow validation and the source/sink flow editor coherent."
keywords: [tool model, data facets, stand-off overlays, annotations, segmentation, IO contract, consumes, produces, flow editor, source, sink, binding, design proposal, RFC]
---

# Design Proposal: Tool & Data-Type Model Redesign

**Status:** Proposal / RFC — not yet decided. This note investigates whether and
how to redesign the tool model around the data types tools consume and produce.
It does not describe current behaviour; for that see
[AD-006](/contribute/architecture/006-tool-system),
[AD-002](/contribute/architecture/002-content-model), and
[AD-026](/contribute/architecture/026-flow-io-binding).

## Motivation

The flow and tool model predates stand-off overlays (AD-002). The IO contract
that exists today (`core/schema.ToolMeta`) describes tools at the **Part-type**
granularity — `Inputs`/`Outputs` are the strings `"block"`, `"data"`, `"media"`,
`"layer"`, `"group"`. In a localization pipeline almost every interesting tool
operates on Blocks, so that granularity carries no discriminating information:
the flow editor's connection check (`isValidConnection` in
`packages/flow-editor/src/FlowEditor.tsx`) compares two sets that are nearly
always `["block"]`, so it never says anything useful.

The data that actually flows between tools is **not** the Part type — it is the
set of *interpretations* riding on each Block: its segmentation overlay, term
and entity overlays, QA findings, alt-translations, TM-match scores, and the
target content itself. AD-006 already names the questions a tool system must
answer uniformly ("What annotations does it produce? **Which does it
consume?**"), but only the produce half is implemented (`ToolMeta.Produces`),
and only for annotations, not overlays.

Four concrete symptoms follow:

1. **Overlays have no declared schema, annotations do.** `model.Annotation` has
   a typed registry (`RegisterAnnotation`/`NewAnnotation`, each type self-naming
   via `AnnotationType()`). Overlays (`model.Overlay`) have a hardcoded
   `OverlayType` enum and untyped `map[string]string` span props — no registry,
   no schema, no validation. The same concept (a term, an entity) is modelled
   *both* as a typed block annotation (`model.TermAnnotation`,
   `model.EntityAnnotation`, each carrying a `RunRange Position`) *and* as a
   positional overlay (`OverlayTerm`, `OverlayEntity`). Which is canonical is
   ambiguous.

2. **No notion of optionality / graceful degradation.** TM leverage works on the
   whole block, and *additionally* per segment span when a segmentation overlay
   is present (AD-002, "Leverage is hybrid"). There is no way to declare
   "optionally consumes segmentation; degrades to whole-block when absent." The
   dependency is invisible to the flow validator and the editor.

3. **Segment iteration is ad-hoc and source-only.** Consumers read segments via
   `Block.SourceSegmentCount()` / `SourceSegmentRuns(i)`
   (`core/model/overlay.go`). These cover only the *primary source* layer. Every
   tool that wants to operate per segment re-implements the same "if a
   segmentation overlay exists, iterate spans; else treat the whole block as one
   unit" dance, and there is no uniform, writable iterator that maps a per-unit
   target write back into the correct run range — so per-segment translation is
   error-prone.

4. **The flow editor's source/sink is half-wired.** AD-026 made source and sink
   *bindings* (endpoint pickers) rather than reader/writer graph nodes. The
   model is right, but the UI is incomplete: new flows do not initialize a
   `binding`, bindings can be dropped on some round-trips, and — because the only
   IO contract is the meaningless part-type `Inputs`/`Outputs` — the editor
   cannot validate that the chosen source can satisfy the first tool, or that the
   last tool's output is materializable by the chosen sink.

The thesis of this proposal: **these are one problem.** Give the system a single
typed vocabulary for the data that rides on a Block — call them **facets** —
let tools declare which facets they *consume* (optionally or as a requirement)
and *produce*, unify overlays and annotations under one registry, and the segment
iterator, the flow validator, and the typed source/sink editor all fall out of
the same contract.

## Recommendation

**Yes, redesign — but as consolidation, not a rewrite.** The foundation is
already present: the tool registry, `ToolMeta`, the capability-typed views
(`BlockView`/`TargetView`/`SourceView`), the overlay model, and the
annotation registry. The work is to (a) unify the two metadata carriers under one
typed registry, (b) raise the IO contract from part-types to facets with
optionality, (c) add a first-class unit/segment iterator on the views, and
(d) wire the resulting contract into flow validation and the editor. Each is
independently shippable behind the existing surfaces.

## Design

### 1. Facets: one typed vocabulary for block-borne data

Define a **facet** as any typed interpretation that can ride on a Block. Each
facet has a stable type id, a registered Go payload schema, a **carrier** (how it
is stored), and a **side** (which run sequence it pertains to).

```go
// core/model (or a new core/facet package)

type FacetType string // "segmentation", "term", "entity", "qa", "alignment",
                       // "alt-translation", "tm-match", "word-count", "target", …

type FacetSide int
const (
    SideBlock  FacetSide = iota // non-positional, whole-block (annotations, properties)
    SideSource                  // pertains to Block.Source
    SideTarget                  // pertains to a target variant
)

type Carrier int
const (
    CarrierOverlay    Carrier = iota // run-anchored span set (positional)
    CarrierAnnotation                // typed block annotation (non-positional)
    CarrierProperty                  // scalar string property
    CarrierTarget                    // the committed Target itself
)

type FacetSchema struct {
    Type     FacetType
    Carrier  Carrier
    NewValue func() any        // typed payload constructor (overlay span props,
                               // annotation struct, …) — for wire (de)serialization
    Layered  bool              // segmentation: multiple named layers may coexist
}
```

Register facets the way annotations are registered today
(`model.RegisterAnnotation`), folding the overlay enum into the same registry so
overlay kinds gain the typed-schema and validation treatment annotations already
have. This **resolves the term/entity duplication**: the positional carrier (the
overlay span, run-anchored) is canonical for *where* a term/entity is; the
non-positional annotation carrier is reserved for genuinely block-level facts.
The existing `model.TermAnnotation.Position`/`EntityAnnotation.Position` fields
migrate to span props on the overlay; the registry records which carrier is
canonical per type.

The win: a facet type now has a *declared schema*, so `kapi tools schema`,
the flow editor's data-flow view, and wire (de)serialization all read the same
source of truth instead of an implicit `map[string]string` contract.

### 2. Tool IO contract: Consumes + Produces over facets, with optionality

Raise `ToolMeta` from part-type strings to facet-level dependencies. Keep the
part-type `Inputs` as a coarse pre-filter (it still answers "does this tool touch
Blocks at all"), but add the facet contract that carries the real information:

```go
// core/schema

type IOFacet struct {
    Type     model.FacetType
    Side     model.FacetSide
    Optional bool   // graceful degradation: tool runs without it, does more with it
    Layer    string // segmentation granularity, "" = primary; optional
}

type ToolMeta struct {
    // … existing fields (ID, Category, Cardinality, Requires, SideEffects, …) …

    Consumes []IOFacet // what the tool reads upstream; non-Optional = a requirement
    Produces []IOFacet // what it writes (supersedes the annotation-only Produces)
}
```

This makes the motivating cases expressible:

| Tool | Consumes | Produces |
| --- | --- | --- |
| `segmentation` | — | `segmentation@source` |
| `tm-leverage` | `segmentation@source` *(optional)* | `tm-match`, `alt-translation`, `target` |
| `ai-translate` | `term@source` *(opt)*, `entity@source` *(opt)* | `target` |
| `term-lookup` | — | `term@source` |
| `qa-check` | `target` *(required)* | `qa@target` |
| `unredact` | secret recovery *(required)* | `target`, `source` |

`tm-leverage` declaring `segmentation@source` as **optional** is exactly "works
on both blocks and segments": the validator never *requires* an upstream
segmenter, but the editor can surface that adding one upgrades the tool's
behaviour, and a flow that *does* segment is known to feed the per-segment path.

**Capability and facets are orthogonal and compose.** The capability
(`Annotate`/`Translate`/`Transform`, AD-006) is the *write-surface* contract —
what kind of mutation the tool is allowed to make. The facet contract is the
*data-dependency* contract — which interpretations it reads and writes. A tool
declares both: e.g. `tm-leverage` is `Translate`-capable (writes target) and
optionally consumes the segmentation facet. Neither subsumes the other; the
immutability backstop continues to enforce the capability, and the facet contract
drives validation and UI.

Validate `Produces`/`Consumes` against the facet registry at tool registration —
the same way `AnnotationRegistry` already rejects an unknown `Produces`
annotation type (AD-006) — so typos fail at startup, not at runtime.

### 3. A first-class unit / segment iterator over stand-off overlays

The user-facing requirement: *consumers should have good Go interfaces for
iterating over segments even though they are stand-off annotations.* Add a
**Unit** abstraction to the views (`core/tool/view.go`) that yields the
granularity a tool should operate on — whole block when unsegmented, per-segment
span when a segmentation overlay is present — with writes that map back to the
correct run range.

```go
// core/tool

// Unit is one processing granularity within a block: the whole block, or one
// segment span when a segmentation overlay is present. It hides whether
// segmentation is materialized as structure or as a stand-off overlay.
type Unit interface {
    Index() int
    Range() model.RunRange   // zero range = whole block
    Ignorable() bool         // segmentation span marked non-translatable

    SourceRuns() []model.Run
    TargetRuns(loc model.LocaleID) []model.Run
}

// Read-only iteration is available on every view tier.
type BlockView interface {
    // … existing methods …
    // SourceUnits yields source segments of the given layer ("" = primary),
    // or a single whole-block unit when no segmentation overlay is present.
    SourceUnits(layer string) iter.Seq[Unit]
}

// Writable per-unit target production for Translate/Transform tiers, splicing
// each unit's runs back into the block at the unit's range and preserving
// ignorable spans verbatim.
type TargetView interface {
    BlockView
    // … existing methods …
    TargetUnits(loc model.LocaleID, layer string) iter.Seq[WritableUnit]
}

type WritableUnit interface {
    Unit
    SetTargetRuns(loc model.LocaleID, runs []model.Run)
}
```

Implementation reuses the existing machinery: `RunRange.ExtractRuns`
(`core/model/overlay.go`) for reads, and an inverse splice for writes that
respects half-open ranges and `Span.Ignorable()`. The iterator is the single
place the "segmented or not" branch lives; every per-segment tool
(`tm-leverage` segment keys, per-segment MT, segment-level QA) drops its
hand-rolled loop. The interface generalizes the source-only
`Block.SourceSegmentRuns` to any side and any named layer, and pairs naturally
with the alignment facet for source↔target unit correspondence.

This is additive: tools that want the whole block keep using `SourceRuns()`;
tools that want units opt into `SourceUnits("")`.

### 4. Flow validation from the contract

With facets and a `Consumes`/`Produces` contract, the flow loader/builder
(`core/flow/builder.go`, `definition.go`) can do **data-flow validation** it
cannot do today:

- For each tool's **required** (non-optional) consumed facet, some upstream
  producer must supply it — an earlier tool's `Produces`, the ingest settle
  stage (AD-026 §4 — segmentation/normalization persisted at extract), or the
  **source binding** (below). Otherwise the flow is rejected with a precise
  message ("`qa-check` requires a `target`; no upstream tool produces one").
- Optional consumed facets never gate validation; they feed the editor's
  "this upgrades when X is present" affordance.
- This complements the existing structural checks (cycle detection, stage
  capability gating) and the source-transform rule that `Build` already enforces
  (only `CapTransform` tools in the leading stage).

### 5. Bindings as facet producers/consumers — fixing the editor

AD-026 already says source and sink are bindings, not nodes, and that the editor
should surface them as endpoint pickers. The facet contract is what makes that
coherent: a **binding advertises the facets it provides or accepts.**

| Binding | As source: provides | As sink: accepts |
| --- | --- | --- |
| `file` | `source` content (one locale, or bilingual for interchange) | requires materializable `target` |
| `store` / `klz` | existing `source` + any persisted overlays (segmentation, terms, …) | accepts any facet (commits overlays) |
| `import`/`export` | `source` + `target` + `segmentation` + `alignment` (AD-017) | emits interchange; requires `target` |
| `none` | — | accepts anything (discards) |

The first tool's required `Consumes` must be satisfiable by the source binding's
provided facets; the last stage's `Produces` must be acceptable by the sink. A
process-only run (`sink: store`/`none`, AD-026 §3) needs no materializable
target; a `file` sink does. This turns the editor's currently-inert
`isValidConnection` into a real check at both the head and the tail of the graph,
and gives the editor the typed "data flowing along each edge" view AD-006
promised.

Concretely, the editor work (`packages/flow-editor/`, plus the
`bowrain/apps/web` and `bowrain/apps/bowrain/frontend` hosts):

- **Initialize `binding` on new flows.** `ProjectFlowsEditor.tsx` and
  `FlowBuilder.tsx` create flows without a `binding`; default it explicitly
  (`{ source: "file" }`) so the pickers reflect real state from creation.
- **Persist bindings on every round-trip.** Audit `graphToSteps`/`stepsToGraph`
  and `defToSpec`/`specToDef` so the `bindings` argument is never dropped; add a
  full save→load→save round-trip test (today only the isolated adapter is
  tested).
- **Type the endpoint pickers from the contract.** The `SourcePicker`/
  `SinkPicker` (`nodes/EndpointPicker.tsx`) advertise provided/accepted facets;
  the canvas validates the head/tail against the first/last tool and shows a
  warning chip when unsatisfied (e.g. a monolingual `file` source under a
  `qa-check`-first flow that needs a `target`).
- **Render real port types.** Tool node ports show facet-level data, not the
  uniform `block`, so a connection that would deliver no consumed facet is
  visibly inert.

## Migration plan (phased, each independently shippable)

1. **Facet registry.** Introduce `FacetType`/registry; fold the `OverlayType`
   enum into it; keep the existing annotation registry working. No behaviour
   change. Pick the canonical carrier per type and document term/entity
   resolution.
2. **Unit iterator.** Add `Unit`/`SourceUnits`/`TargetUnits` to the views;
   reimplement `tm-leverage` segment keys and one per-segment tool on it to prove
   the interface; leave whole-block tools untouched.
3. **IO contract.** Add `Consumes []IOFacet` and migrate `Produces` to
   `[]IOFacet` (keep a compatibility shim mapping the old `AnnotationType`
   produces). Backfill contracts for built-in tools in `core/tools/register.go`
   (and `core/ai/tools`, `core/mt/tools`). Validate against the registry at
   registration.
4. **Flow validation.** Add data-flow validation in `builder.go`/`definition.go`
   (warn-only first, then error) using the contract + source binding facets.
5. **Editor wiring.** Fix binding init/persistence; type the endpoint pickers and
   ports from the contract; add the round-trip test. Re-record affected flow
   editor walkthrough scenes per the UI-change checklist in CLAUDE.md.

## Risks and open questions

- **Carrier canonicalization for terms/entities** is a real data-model change
  (today both annotation and overlay exist). Needs a decision and a migration for
  any persisted `.klz`/store data and bilingual round-trips.
- **Contract accuracy is load-bearing once it gates flows.** Start validation in
  warn-only mode; only escalate to hard errors after the built-in contracts are
  audited, or correct flows will be rejected.
- **Scope of `Outputs` vs `Produces`.** Decide whether to retire the part-type
  `Outputs` entirely or keep it as the coarse pre-filter alongside facet
  `Produces`.
- **Plugin tools** (AD-007) declare metadata over gRPC; the facet vocabulary must
  be extensible by plugins (register facet types) and survive the bridge, or
  plugin tools become second-class in validation.
- **`iter.Seq`** requires Go 1.23+; confirm the toolchain floor before adopting
  range-over-func in the public view interface (a slice-returning form is the
  fallback).

## Related

- [AD-002: Content Model](/contribute/architecture/002-content-model) — Blocks, stand-off overlays, segmentation, annotations.
- [AD-006: Tool System](/contribute/architecture/006-tool-system) — capability-typed handlers, `ToolMeta`, the IO contract this proposal extends.
- [AD-026: Flow I/O Binding](/contribute/architecture/026-flow-io-binding) — source/sink as bindings; the facet algebra makes the binding ends typed.
- [Flow Steps Format](flow-steps-format.md) — the steps document the editor reads and writes.
- [Session-Scoped Tool Authoring](session-tool-authoring.md) — overlay conventions for SessionTools.
</content>
