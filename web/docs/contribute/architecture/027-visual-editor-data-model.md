---
id: 027-visual-editor-data-model
sidebar_position: 27
title: "AD-027: Visual Editor Data Model & Round-Trip"
description: "Architecture decision: the visual editor is a render-and-inspect surface over the content model — a Part stream becomes a ContentTree, normalized to a format-shaped RenderDoc, rendered with vocabulary-styled runs and stand-off overlay marks; a target edit is committed on the model via SetTargetRuns and round-trips to byte-faithful output via reader + skeleton + writer."
keywords: [visual editor, preview, ContentTree, RenderDoc, DocumentViewer, BlockInspector, overlay rendering, vocabulary, round-trip, skeleton, architecture decision, neokapi]
---

import { PipelineDiagram, RoundTripDiagram } from "@neokapi/docs-shared";

# AD-027: Visual Editor Data Model & Round-Trip

## Summary

The visual editor is a **render-and-inspect surface over the content model**
([AD-002](002-content-model.md)), shared as a kit in `@neokapi/ui-primitives`
(the `/preview` subpath). A document's `Part` stream is first projected to a
hierarchical, JSON-serializable **`ContentTree`** (`core/editor`), then
normalized on the TypeScript side to a **`RenderDoc`** whose `kind`
(`slides | sheet | doc | pages | list | sections`) drives a format-shaped
renderer in `FormatPreview`, wrapped by `DocumentViewer`'s tabs
(Preview / Blocks / Raw / Stats / Download). Inline runs render through the
**vocabulary** (color, label, editing constraints); stand-off **overlays**
render as inline marks whose accent is keyed by `(overlay type, span category)`;
`BlockInspector` surfaces overlays, annotations, and per-variant targets.

The kit is deliberately **render-and-inspect** — the framework's own apps
(kapi-desktop, kapi-lab) consume it to display content; it does not itself ship
a production translation-editing application. The canonical way to commit a
translation is the content model's own **`model.Block.SetTargetRuns`**. The
**round-trip** to byte-faithful output is a framework mechanism independent of
who edited: it replays the source through its reader, injects the committed
targets, and reconstructs the original document via the **skeleton**
([AD-005](005-format-system.md)). Targets and annotations are model carriers;
overlays are reconstructed on demand.

<RoundTripDiagram
  animated
  hub={{ label: "Content model", sub: "Block · Run · Target" }}
  forward={[
    { label: "Source", sub: "file / store", role: "io" },
    { label: "Reader", sub: "+ skeleton" },
    { label: "Editor", sub: "render + inspect", role: "tool" },
  ]}
  back={[
    { label: "Edit", sub: "SetTargetRuns", role: "translate" },
    { label: "Writer", sub: "+ skeleton" },
    { label: "Output", sub: "faithful original", role: "io" },
  ]}
  forwardLabel="render"
  backLabel="edit → write"
  caption="The visual editor sits on the content model: the top row projects model → pixels; the bottom row commits an edit and reconstructs faithful output."
/>

## Context

The content model ([AD-002](002-content-model.md)), the format system and its
byte-faithful skeleton ([AD-005](005-format-system.md)), the tool system and its
capability-typed immutability ([AD-006](006-tool-system.md)), and the KLF
interchange family ([AD-025](025-klf-package.md)) each have an Architecture
Decision. The **visual editor does not** — its data representation, render
contract, and round-trip path lived only in component code and Storybook.
[AD-014](014-kapi-desktop.md) documents the desktop *application* shell (Wails,
flow runner, plugin manager) but not the editor's model.

Two properties make the editor worth documenting on its own:

- **It spans Go and TypeScript.** The model is projected to a `ContentTree` in
  `core/editor` (Go), then normalized and rendered in `packages/ui` (TypeScript).
  The hand-off contract (`ContentTree` ⇄ its TS mirror in
  `preview/types.ts`) is the seam where the two halves meet.
- **The back path is the least-documented seam.** The framework supplies the
  target-edit primitive (`model.Block.SetTargetRuns`) and the faithful
  round-trip (reader + skeleton + writer), but not a production editing
  application — so the contract worth pinning down is *model → edit → write*,
  not any particular UI's commit flow.

## Decision

### The editor renders the content model, it does not own a parallel one

The bridge between the engine and any preview/editor UI is the **`ContentTree`**
(`core/editor/anatomy.go`): a hierarchical, JSON-serializable view of a
document's `Part` stream. `BuildContentTree` walks the stream with a container
stack, attaching each `Part` to the innermost open container and producing nodes
of `Kind` `layer | group | block | data | media`. A block node preserves the
**run sequence** (`Source` / `Targets` as `[]model.Run`, via `blockNode` /
`targetMeta`), the stand-off **overlay views** (`overlayViews`, carrying the text
each span covers), and **segment spans** (`segmentSpans`, by run-index range from
the segmentation overlay — [AD-002](002-content-model.md)).

`ContentTree` is distinct from the editor's other projection, **`BlockIndex`**,
which flattens a block's source to plain strings for reconstruction. The preview
kit consumes `ContentTree` (run-preserving), so the editor renders exactly what
the model holds — inline placeholders, paired codes, plurals — rather than a
lossy string view.

### Render path: `ContentTree` → `RenderDoc` → view

The TypeScript side turns the tree into a structured, format-shaped document and
then into JSX.

1. **`treeToRenderDoc` (`preview/renderDoc.ts`)** normalizes a `ContentTree` into
   a `RenderDoc` via a data-driven **`STRUCTURE_RULES`** table. It scans all
   blocks for target locales (`collectLocales`), then tries layer-shape detectors
   in order (first match wins) — slides (`ppt/slides/slideN.xml`), sheet
   (`xl/worksheets/sheetN.xml`), doc (`word/document.xml`), pages (a
   `page N` layer pattern) — falling back to a format-family classification
   (`DOC_FORMATS`, `LIST_FORMATS`) or a generic `sections` extraction. The result
   is a `RenderDoc { kind, format, locales?, … }` where `kind` is one of
   `slides | sheet | doc | pages | list | sections`. Every block projects to a
   `RenderLine` (`lineFromBlock`) carrying `id`, `text` (`runsText`), per-locale
   `targets`, `role`, `overlays`, and `annotations`.

2. **`FormatPreview` (`preview/FormatPreview.tsx`)** dispatches on `doc.kind` to a
   kind-specific renderer (`Slides`, `Sheet`, `Doc`, `Pages`, `List`,
   `Sections`). Leaf text is rendered by `LineText`, which applies the active
   transition, resolves overlay marks (`resolveOverlaySpans`), and — when a
   `before` doc is supplied — word-level diff highlighting.

3. **`DocumentViewer` (`preview/DocumentViewer.tsx`)** composes the full surface:
   a header (filename, file-type badge, byte size, download), a source↔target
   `ToggleGroup` shown only when target locales exist, and the five tabs —
   **Preview** (`FormatPreview`), **Blocks** (`BlockInspector` per block),
   **Raw** (syntax-highlighted bytes), **Stats** (counts by kind), **Download**.

<PipelineDiagram
  animated
  stages={[
    { label: "Source", sub: "file / store", role: "io" },
    { label: "Reader", sub: "DataFormatReader" },
    { label: "ContentTree", sub: "core/editor", note: "Part stream → tree" },
    { label: "RenderDoc", sub: "renderDoc.ts", note: "STRUCTURE_RULES" },
    { label: "View", sub: "DocumentViewer", role: "tool" },
  ]}
  caption="Render path: a Part stream becomes a hierarchical ContentTree, normalized to a format-shaped RenderDoc whose kind drives the JSX renderer."
/>

### Run ↔ vocabulary ↔ rendering

Inline runs are styled through the **vocabulary registry**
(`packages/ui/src/vocabularies`). `VocabularyRegistry.lookupOrFallback(typeName)`
resolves a run's `Type` to a `SpanTypeInfo` — `{ category, label, html, display,
chipLabel, color, equiv, constraints }`. An **unknown** type is not an error: the
registry synthesizes a `SpanTypeInfo` from a `defaultFallback`, interpolating the
type name into the display/html templates and deriving a short chip label, with a
neutral gray accent and permissive constraints. The editor uses these fields for
styled chips, tooltips, and the deletable/cloneable/reorderable editing
constraints.

This mirrors the model contract: the vocabulary is **descriptive** — it drives
display and editing affordances — and is never consulted by writers, which replay
each run's `Data` verbatim (`RenderRunsWithData`, [AD-002](002-content-model.md)).

### Overlay → styling dispatch

Stand-off overlays ([AD-002](002-content-model.md)) become color-coded,
tooltipped marks via `preview/overlayHighlight.ts`. The accent is a function of
the overlay type **and** the span's props, not the type alone:

- **`effectiveKey(type, span)`** resolves the accent key: a `qa`
  overlay whose `span.props.category` is `"brand-vocabulary"` resolves to the
  `brand-vocabulary` key (a brand violation, pink); every other overlay resolves
  on its type. `overlayStyle` looks the effective key up in **`OVERLAY_STYLES`**
  (`term` → violet "Vocabulary", `qa` → amber "QA", `entity` → sky "Entity",
  `segmentation` → slate "Segment", `alignment` → teal "Alignment"), falling back
  to a neutral "Annotation" accent.
- **`resolveOverlaySpans`** locates each overlay span in the rendered text by
  substring-matching the engine-extracted `span.text` (overlays anchor to
  run-index ranges, but the renderer works over the concatenated literal text, so
  matching by text is robust across the run↔text projection); spans whose text
  cannot be found — e.g. spans covering only inline markup — are dropped.
- **`segmentText`** flattens overlapping spans with an **innermost-wins** rule:
  for each character position the narrowest covering span owns it, and contiguous
  runs under the same owner emit a single non-overlapping `TextSegment`.

`BlockInspector` (`preview/BlockInspector.tsx`) is the structural counterpart to
the styled preview: a collapsible per-block view rendering the source
`RunSequence`, each variant's `TargetRow` (variant key, status, score, origin),
`OverlayRow`s (type, side, layer, and each span's id / run range / text / props),
`AnnotationRow`s (type badge, summary, fields), the properties grid, and flag
badges (skeleton, referent, preserve-whitespace, identity).

### The shared editor surface (reuse boundary)

`@neokapi/ui-primitives` is the single source of truth for the preview/editor
kit, exported under the `./preview` subpath: `DocumentViewer`, `FormatPreview`,
`FileBrowser`, `BlockInspector`, `ContentTreeView`, `RunSequence`, `CodeView`, and
the utilities `renderDoc`, `overlayStyle` / `resolveOverlaySpans`, and the
`vocabularies` registry.

| Consumer | How it uses the kit | Role |
| --- | --- | --- |
| **kapi-desktop** (`apps/kapi-desktop/frontend`) | `FilePreview` imports `DocumentViewer` for file inspection | preview / inspect |
| **kapi-lab** (`packages/kapi-lab`) | `OutputView` wraps `DocumentViewer` to inspect engine output; explorers use `ContentTreeView` / `FileBrowser` | preview / inspect |

The boundary is deliberate: the shared kit **renders and inspects** the content
model. The framework ships no production translation-editing surface of its own;
an application that needs editing builds its commit surface on top of the model's
`SetTargetRuns` and its own persistence (out of scope for the framework). Keeping
the kit free of an editing/commit dependency is what lets the framework apps
share it as pure, dependency-light UI.

### Edit path and round-trip

The editor-side **`BlockIndex`** (built at parse time, serialized for a frontend)
carries a flattened, string-valued view; its `UpdateTarget` method is a **test
helper** that mutates that in-memory projection — it is *not* a commit path.

The canonical way to commit a target edit is on the content model itself:
**`model.Block.SetTargetRuns(locale, runs)`** sets the variant's runs in place
(with `SetTargetText` / `SetText` for the plain-text path —
[AD-006](006-tool-system.md)). How a host application transports an edit to the
model and persists the result — the project **`BlockStore`**
([AD-008](008-project-model.md)), **KLF** files ([AD-025](025-klf-package.md)), a
database — is the application's concern and outside this AD.

The **round-trip** to byte-faithful output is a framework mechanism, independent
of who edited: the source is replayed through its `DataFormatReader`, the
committed targets are injected into the emitted `PartBlock`s, and a writer
reconstructs the document by pairing the reader's `SkeletonStoreEmitter` with the
writer's `SkeletonStoreConsumer` — interleaving literal skeleton fragments with
the target runs rather than re-serializing a parse tree
([AD-005](005-format-system.md)). For standalone kapi the equivalent bilingual
round-trip is the `extract` / `merge` workflow
([AD-017](017-bilingual-format-interop.md)).

<PipelineDiagram
  animated
  channelLabel="edit"
  stages={[
    { label: "Edit", sub: "target runs", role: "tool" },
    { label: "SetTargetRuns", sub: "model.Block", note: "in place" },
    { label: "Reader replay", sub: "source", note: "inject targets" },
    { label: "Writer", sub: "+ skeleton" },
    { label: "Output", sub: "faithful original", role: "io" },
  ]}
  caption="Edit path: a target is set on the model, then the round-trip replays the source through its reader, injects the targets, and reconstructs byte-faithful output via the skeleton. Persistence is a host-application concern."
/>

### Persistence: what round-trips, what is reconstructed

Within the content model and its KLF interchange ([AD-025](025-klf-package.md)):

- **Targets** are first-class records — runs plus `Status`, `Origin`, `Score`
  ([AD-002](002-content-model.md)).
- **Annotations** are the block-scoped typed carrier
  ([AD-002](002-content-model.md)); `.klf` carries blocks, targets, and
  properties, and the `.klfl` JSON-Lines sidecar carries annotation overlays
  (anchor kinds `block` / `run` / `range` / `form`).
- **Skeleton** is the binary `SkeletonStore` ([AD-005](005-format-system.md)).

**Overlays are reconstructed on demand**, not serialized as positional structure:
segmentation is recomputed from the runs, and term / entity / QA overlays are
re-attached by the tools that produce them within a session. Because overlay
spans anchor to run ranges, a source rewrite shifts or drops them via
`model.RemapOverlays` ([AD-006](006-tool-system.md)); targets and annotations are
unaffected. How a host application stores these artefacts (files, database) is
outside the framework.

## Consequences

- The editor has one documented contract shared by kapi-desktop and kapi-lab; a
  new `RenderKind`, overlay accent, or inspector row is a localized change against
  a known seam.
- Rendering is a **pure projection** of the content model — the editor never
  diverges from what the engine holds, because it consumes the run-preserving
  `ContentTree`, not a separate model.
- Editing/commit is intentionally **outside** the shared kit (and outside the
  framework): the canonical target-edit primitive is `model.Block.SetTargetRuns`,
  and the kit stays render-and-inspect, which keeps it dependency-light and
  shareable across the framework's apps.
- Faithful round-trip is a property of the **reader + skeleton + writer**
  ([AD-005](005-format-system.md)), not of any editor — setting a target run
  sequence and replaying the source is what reconstructs the document.
- Overlays are **ephemeral** in the live preview: durable interpretations must be
  stored as annotations (or re-derived by re-running the producing tool). A
  feature that needs persistent positional overlays would adopt the defined
  `.klfl` sidecar rather than inventing a new store.
- `BlockIndex.UpdateTarget` is a test helper; relying on it as a commit path would
  be a mistake — `model.Block.SetTargetRuns` is the canonical target-edit
  operation.

## Open questions / known divergences

- **Overlay persistence.** There is no persistence layer for positional overlays
  in the framework today; the `.klfl` sidecar ([AD-025](025-klf-package.md)) is
  defined but not yet read or written by the preview kit. Whether term/entity/QA
  overlays should persist (vs. always re-derive) is unsettled.
- **`BlockIndex` lifecycle on edit.** Whether a frontend holds its own
  `BlockIndex` and sends delta edits, or re-derives it after each commit, is not
  pinned down.
- **No framework editing surface.** The framework ships no production
  translation-editing/commit UI: kapi-desktop uses the kit for preview
  (`FilePreview`) and kapi-lab for inspection. A consuming editor is out of scope
  for this AD.
- **`DisplayHint` / `ContentRef` scope.** Whether these are populated by all
  readers and persisted by a given host store is format- and
  application-dependent.
- **Segment span IDs across edits.** Whether overlay span IDs (e.g. `s1`, `s2`)
  remain stable when source text changes, or shift with rebasing.

## Related

- [AD-002: Content Model](002-content-model.md) — Part/Block/Run, overlays, annotations, targets, the run-text projections the editor renders
- [AD-005: Format System](005-format-system.md) — readers/writers and the skeleton that makes the round-trip byte-faithful
- [AD-006: Tool System](006-tool-system.md) — capability-typed immutability, `SetTargetRuns`/`SetText`, and `RemapOverlays` overlay rebasing
- [AD-008: Project Model](008-project-model.md) — the `BlockStore` that a host persists edits through
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — the desktop application that hosts the preview kit
- [AD-017: Bilingual Format Interop](017-bilingual-format-interop.md) — the standalone-kapi `extract`/`merge` faithful round-trip
- [AD-025: KLF Family and `.klz` Package](025-klf-package.md) — `.klf` blocks and the `.klfl` annotation sidecar
