---
sidebar_position: 12
title: "Tool & Data Model Rationale"
description: "Why the tool and data model is shaped the way it is: stand-off interpretations carried as positional Overlays and block-scoped Annotations under one typed payload registry, a typed consumes/produces IO contract over IOPorts, a uniform unit/segment iterator over segmentation overlays, and typed source/sink bindings that make flow validation and the flow editor coherent."
keywords: [tool model, stand-off overlays, annotations, segmentation, IO contract, IOPort, consumes, produces, flow editor, source, sink, binding]
---

# Tool & Data Model Rationale

This note records the **design rationale** behind neokapi's tool and data
model — why stand-off interpretations are carried the way they are, why the IO
contract is typed over the data that rides on a Block, why there is a uniform
unit/segment iterator on the tool views, and why source and sink are typed
bindings. The canonical, normative descriptions live in the ADs:

- [AD-002: Content Model](/contribute/architecture/002-content-model) — Overlays
  and Annotations as the two stand-off carriers.
- [AD-006: Tool System](/contribute/architecture/006-tool-system) — the
  capability-typed handlers, the `IOPort` IO contract, and the unit iterator.
- [AD-026: Flow I/O Binding](/contribute/architecture/026-flow-io-binding) —
  typed source/sink bindings and data-flow validation.

Read this note for the *why*; read the ADs for the authoritative *what*.

## The data that flows between tools

A Block's content is its `Source []Run` and its variant-keyed `Targets`. The
data that actually flows *between* tools in a localization pipeline is not the
coarse Part type — almost every interesting tool operates on Blocks — it is the
set of typed **interpretations** riding on each Block: its segmentation, term
and entity spans, QA findings, alt-translations, TM-match scores, and the target
content itself. The model is shaped around making those interpretations
first-class, typed, and declarable, because that is what lets the flow validator
and the flow editor reason about a pipeline at all.

Each interpretation is held **stand-off** — separate from the runs — so the same
content can carry segmentation, terminology, QA findings, notes, and analysis
results at once without rewriting it, and so dropping an interpretation restores
the plain content. There are two carriers, distinguished by whether the
interpretation has a position:

- **Overlays** (`Block.Overlays []model.Overlay`) are **positional**: each
  overlay groups one type's run-anchored spans on one side of the block. An
  `Overlay` has a `Type` (an `OverlayType`), an optional `Variant` (nil = source
  side; set = a target variant), an optional `Layer` (segmentation granularity;
  `LayerPrimary` = primary), and `Spans`. A `Span` carries a run `Range` (its position), an
  `ID`, optional `Props`, and a typed payload `Value`. Because a span's range
  anchors into the runs, a source rewrite moves it — the framework applier
  rebases surviving spans onto the rewritten runs (`model.RemapOverlays`) and
  drops any span overlapping a rewritten range.
- **Annotations** (`Block.Annotations map[string]any`) are **block-scoped**:
  typed metadata keyed by type name, with no position — notes, alt-translations,
  analysis results, and format round-trip state. A source rewrite does not
  invalidate them. Multiplicity lives inside the value, never in numbered keys:
  every alternative translation is one `AltTranslations` collection under the
  single `alt-translation` key, not `alt-translation-1`, `-2`, and so on.

Whether an interpretation is positional is **structural** — it is either an
`Overlay` or an `Annotation` — not a runtime flag. The two kinds differ in
cardinality, access pattern, and lifecycle under source edits, so they are
separate types rather than one carrier with a flag.

### Why typed payloads under one registry

Both an overlay span's `Value` and an annotation value are **typed payloads**,
not untyped `map[string]string` bags, so a tool reads a concrete struct
(`TermAnnotation`, `EntityAnnotation`, `AltTranslations`, …) rather than parsing
strings. A single payload registry keyed by type name
(`model.RegisterPayload` / `model.NewPayload`) lets the wire (the subprocess
plugin gRPC bridge) and the SQLite store layers rehydrate the concrete type on
the far side from its type name alone. The framework registers the well-known
content payloads; formats and plugins register their own. An unknown payload type
crossing the bridge degrades to a `GenericAnnotation` map keyed by name rather
than being dropped, so a plugin-defined type round-trips by name and JSON even
where the peer has not registered its constructor.

A term and an entity exist in exactly one place — a run-anchored span on the
`term` / `entity` overlay, where the span's range *is* the position and its id
the identity. There is no parallel block-annotation form carrying a duplicate
position field.

### Why `Properties` is pass-through only

`Block.Properties map[string]string` is reserved for opaque, non-interpretive
metadata — connector keys (`cms-path`), format round-trip hints. Every
analytic or interpretive result a tool produces — `word-count`, `tm-match`
scores, brand-vocab findings, repetition status — is an overlay or an
annotation, never a property. Keeping interpretive results off `Properties`
removes any contradiction between what a tool declares it `Produces` and where it
actually writes, and lets the IO contract (below) name a single source of truth
for each datum.

A committed `Target` stays its own first-class carrier: it is the *chosen*
output, not an interpretation of content. Candidate proposals (TM/MT/AI) remain
`alt-translation` annotations until one is committed as the Target.

## A typed IO contract over `IOPort`s

Tools communicate by reading the overlays and annotations produced upstream and
writing their own downstream — loose coupling through the shared data model
rather than direct dependencies. For the flow validator and the editor to reason
about that, a tool must **declare** which interpretations it reads and writes.
A coarse Part-type contract (`"block"` in, `"block"` out) carries no
discriminating information, because nearly every tool is `["block"]` → `["block"]`.

So the IO contract on `core/schema.ToolMeta` is expressed over **`IOPort`s** —
the typed stand-off data of a Block, not part types:

```go
// core/schema/schema.go
type IOPort struct {
    Type     string     // an OverlayType, an annotation key, or "target"/"source"
    Side     model.Side // source | target
    Optional bool       // consumed: degrades without it, does more with it
    Layer    string     // segmentation granularity; LayerPrimary = primary
}

type ToolMeta struct {
    // … ID, Category, Cardinality, Requires, SideEffects, … …
    Consumes []IOPort // read upstream; a non-Optional entry is a hard requirement
    Produces []IOPort // written to the Block
}
```

An `IOPort.Type` names an overlay type (`OverlayTerm`, `OverlayQA`, …), a
block-annotation key (`AnnoBrandVoice`, …), or a **pseudo-port** —
`schema.PortTarget` (`"target"`, the committed Target) or `schema.PortSource`
(`"source"`, a rewritten source) — which participate in data-flow validation but
are not stored as stand-off layers. The `schema.Port[T ~string](t, side)` helper
builds an `IOPort` from any of these type names without a `string()` at the call
site.

This makes the motivating cases expressible:

| Tool | Consumes | Produces |
| --- | --- | --- |
| `segmentation` | — | `segmentation@source` |
| `tm-leverage` | `segmentation@source` *(optional)* | `tm-match`, `alt-translation`, `target` |
| `translate` | `term@source` *(opt)*, `entity@source` *(opt)* | `target` |
| `term-lookup` | — | `term@source` |
| `qa` | `target` *(required)* | `qa@target` |
| `unredact` | secret recovery *(required)* | `target`, `source` |

### Why optionality

`tm-leverage` declaring `segmentation@source` as **optional** is exactly "works
on both blocks and segments": the validator never *requires* an upstream
segmenter, but the tool does more when one is present — it leverages per segment
span instead of only whole-block. Optional consumed ports model graceful
degradation, so the editor can surface "adding a segmenter upgrades this tool"
without making it a hard dependency. Non-optional consumed ports are hard
requirements the flow validator enforces.

### Why capability and ports are orthogonal

The capability a tool declares by which block handler it sets on `BaseTool`
(`Annotate` / `Translate` / `Transform`, AD-006) is the **write-surface**
contract — what kind of mutation the tool may make. The `IOPort` contract is the
**data-dependency** contract — which interpretations it reads and writes. They
compose: `tm-leverage` is `Translate`-capable (writes the target) *and*
optionally consumes the segmentation overlay. Neither subsumes the other — the
immutability backstop enforces the capability; the port contract drives flow
validation and the UI. `Consumes`/`Produces` types are validated against the
payload registry at tool registration, so a typo fails at startup, not at
runtime.

## A uniform unit/segment iterator

Because a "segment" is just a span in the segmentation overlay — not a structural
type — every tool that wants to operate per segment would otherwise re-implement
the same dance: *if a segmentation overlay exists, iterate its spans; else treat
the whole block as one unit*, and then map a per-unit target write back into the
correct run range. That is error-prone, and the ad-hoc helpers only covered the
primary source layer.

So the tool views (`core/tool/view.go`) expose a uniform **unit** iterator that
yields the granularity a tool should operate on — the whole block when
unsegmented, one segment span when a segmentation overlay is present — and hides
whether segmentation is materialized as structure or as a stand-off overlay:

```go
// core/tool
type BlockView interface {
    // … SourceUnits yields the source units of the given segmentation layer
    // (LayerPrimary = primary), or a single whole-block unit when none is present.
    SourceUnits(layer string) iter.Seq[Unit]
}

type TargetView interface {
    BlockView
    // TargetUnits yields writable per-unit target production over the source
    // segmentation of the given layer, splicing each unit's runs back into the
    // block at the unit's range and preserving ignorable spans verbatim.
    TargetUnits(loc model.LocaleID, layer string) iter.Seq[WritableUnit]
}
```

Reads reuse `RunRange.ExtractRuns` (`core/model/overlay.go`); writes use an
inverse splice that respects half-open ranges and `Span.Ignorable()`. The
iterator is the single place the "segmented or not" branch lives — every
per-segment tool (`tm-leverage` segment keys, per-segment MT, segment-level QA)
drops its hand-rolled loop. It generalizes the source-only segment access to any
side and any named layer, and pairs naturally with the `alignment` overlay for
source↔target unit correspondence. It is additive: a tool that wants the whole
block keeps using `SourceRuns()`; a tool that wants units opts into
`SourceUnits("")`.

## Flow validation from the contract

With a typed `Consumes`/`Produces` contract, the flow loader/builder
(`core/flow/builder.go`, `definition.go`) does **data-flow validation** a
part-type contract cannot:

- For each tool's **required** (non-optional) consumed port, some upstream
  producer must supply it — an earlier tool's `Produces`, ingest-time settlers
  (AD-026 — segmentation/normalization persisted at extract), or the **source
  binding** (below). Otherwise the flow is rejected at load/build with a precise
  message ("`qa` requires a `target`; no upstream tool produces one")
  rather than failing at runtime.
- Optional consumed ports never gate validation; they feed the editor's "this
  upgrades when X is present" affordance.

This complements the structural checks (cycle detection) and the transformer
**placement pass** (`core/flow/placement.go`, AD-006), which runs beside
data-flow validation at the same build/load gates.

## Bindings as port producers/consumers

AD-026 makes source and sink **bindings** (endpoint pickers), not reader/writer
graph nodes. The port contract is what makes that coherent: a binding advertises
the ports it provides as a source or accepts as a sink.

| Binding | As source: provides | As sink: accepts |
| --- | --- | --- |
| `file` | `source` content (one locale, or bilingual for interchange) | requires materializable `target` |
| `store` / `klz` | existing `source` + any persisted overlays (segmentation, terms, …) | accepts any port (commits overlays) |
| `import` / `export` | `source` + `target` + `segmentation` + `alignment` (AD-017) | emits interchange; requires `target` |
| `none` | — | accepts anything (discards) |

The first tool's required `Consumes` must be satisfiable by the source binding's
provided ports; the last stage's `Produces` must be acceptable by the sink. A
process-only run (`sink: store` / `none`, AD-026) needs no materializable target;
a `file` sink does. This gives the flow editor a real check at both the head and
tail of the graph, and the typed "data flowing along each edge" view: tool node
ports render the overlay/annotation types they carry, so a connection that would
deliver no consumed port is visibly inert, and a monolingual `file` source under
a `qa`-first flow that needs a `target` shows an unsatisfied-binding
warning.

## Notes on edge cases

- **Contract accuracy is load-bearing.** With hard validation, a wrong
  `Consumes`/`Produces` on a built-in tool breaks a real flow. Each tool's
  declared contract is audited against its actual reads/writes; an end-to-end test
  per built-in flow is the guardrail.
- **Plugin tools** (AD-007) declare their metadata over gRPC. The overlay /
  annotation vocabulary is extensible by plugins (`model.RegisterPayload`) and
  crosses the bridge via the `OverlayMessage` carrier, so a term, entity, qa,
  alignment, or any plugin-defined type round-trips by type name and JSON; full
  typed rehydration on a peer requires that peer to have registered the payload
  constructor.
- **Alignment is relational** — it links a source span to a target span, the one
  overlay whose payload references another side's range rather than annotating its
  own. The single-side `Overlay` shape carries the counterpart range in the
  payload.
- **`Span.Value any`** is a typed payload; the wire format and the SQLite store
  schema serialize it by type name through the payload registry, the same path
  the bridge and store use to rehydrate any stand-off value.

## Related

- [AD-002: Content Model](/contribute/architecture/002-content-model) — Blocks, Overlays, Annotations, segmentation.
- [AD-006: Tool System](/contribute/architecture/006-tool-system) — capability-typed handlers, `ToolMeta`, the `IOPort` IO contract, the unit iterator.
- [AD-026: Flow I/O Binding](/contribute/architecture/026-flow-io-binding) — source/sink as bindings; typed binding ends.
- [Flow Steps Format](flow-steps-format.md) — the steps document the editor reads and writes.
- [Session-Scoped Tool Authoring](session-tool-authoring.md) — overlay conventions for SessionTools.
