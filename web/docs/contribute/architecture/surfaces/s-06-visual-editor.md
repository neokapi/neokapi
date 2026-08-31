---
id: s-06-visual-editor
sidebar_position: 6
title: "S-06: The visual editor data model"
description: "The preview kit is a render-and-inspect surface over the content model: a Part stream becomes a ContentTree, normalized to a format-shaped RenderDoc, rendered with vocabulary-styled runs and overlay marks through a projection declared per run kind; a target edit is committed on the model and round-trips through reader, skeleton, and writer."
keywords: [neokapi, architecture decision, visual editor, preview kit, ContentTree, RenderDoc, DocumentViewer, BlockInspector, vocabulary, overlays, RunSpec, round-trip]
---

import { PipelineDiagram, RoundTripDiagram } from "@neokapi/docs-shared";

# S-06: The visual editor data model

## Summary

The visual editor is a **render-and-inspect surface over the content model**
([F-02](../foundations/f-02-content-model.md)), shipped as a kit in
`@neokapi/ui-primitives` under the `./preview` subpath. A document's `Part`
stream is projected in Go to a hierarchical, JSON-serializable **`ContentTree`**
(`core/editor`), then normalized on the TypeScript side to a **`RenderDoc`**
whose `kind` drives a format-shaped renderer. Inline runs are styled through the
**vocabulary**; stand-off **overlays** render as inline marks whose accent is a
function of the overlay type *and* the span's own properties. Every projection
of a run sequence to text is declared per run kind, so a placeholder or a plural
can never vanish from a view. Editing is deliberately kept out of the kit: the
canonical way to commit a translation is the model's own `SetTargetRuns`, and
the byte-faithful round-trip is a property of reader + skeleton + writer,
independent of who edited.

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
  ariaLabel="The visual editor over the content model: render forward, edit and write back"
  caption="The top row projects model to pixels; the bottom row commits an edit and reconstructs faithful output."
/>

## Context

The content model, the format system and its byte-faithful skeleton, the tool
system, and the interchange package family each have their own decision record.
The editor spans two of them and belongs to neither, for two reasons.

**It spans Go and TypeScript.** The model is projected to a `ContentTree` in
`core/editor`, then normalized and rendered in `packages/ui`. The hand-off, the
`ContentTree` and its TypeScript mirror, is the seam where the two halves meet,
and a seam nobody wrote down is a seam that drifts.

**The back path is the least obvious part.** The framework supplies a target-edit
primitive and a faithful round-trip, but ships no production translation-editing
application. So the contract worth pinning down is *model → edit → write*, not
any particular interface's commit flow.

## Decision

### The editor renders the content model

The bridge between the engine and any preview surface is the **`ContentTree`**
(`core/editor/anatomy.go`): a hierarchical, JSON-serializable view of a
document's `Part` stream. `BuildContentTree` walks the stream with a container
stack, attaching each part to the innermost open container and producing nodes of
kind `layer`, `group`, `block`, `data`, or `media`.

A block node preserves what the model holds: the **run sequences** for source and
each target, per-target metadata, the stand-off **overlay views** carrying the
text each span covers, and **segment spans** derived by run-index range from the
segmentation overlay ([M-02](../multilingual/m-02-segmentation.md)).

`ContentTree` is distinct from the editor's other projection, **`BlockIndex`**,
which flattens a block's source to plain strings for reconstruction. The preview
kit consumes the run-preserving `ContentTree`, so the editor renders exactly what
the model holds (inline placeholders, paired codes, plurals) rather than a
lossy string view.

### Render path

<PipelineDiagram
  animated
  stages={[
    { label: "Source", sub: "file / store", role: "io" },
    { label: "Reader", sub: "DataFormatReader" },
    { label: "ContentTree", sub: "core/editor", note: "Part stream → tree" },
    { label: "RenderDoc", sub: "renderDoc.ts", note: "STRUCTURE_RULES" },
    { label: "View", sub: "DocumentViewer", role: "tool" },
  ]}
  caption="A Part stream becomes a hierarchical tree, normalized to a format-shaped document whose kind selects the renderer."
/>

**`treeToRenderDoc`** normalizes a `ContentTree` into a `RenderDoc` through a
data-driven `STRUCTURE_RULES` table. It collects target locales across all
blocks, then tries layer-shape detectors in declaration order, first match
winning (presentation slides, spreadsheet worksheets, a word-processing document
body, a paged layer pattern), falling back to a format-family classification and
finally to a generic section extraction. The result carries a `kind` of `slides`,
`sheet`, `doc`, `pages`, `list`, or `sections`, and every block projects to a
render line carrying its id, text, inline codes, per-locale targets, role,
overlays, and annotations.

**`FormatPreview`** dispatches on that kind to a kind-specific renderer. Leaf
text goes through a shared component that applies the active transition, resolves
overlay marks, and, when a `before` document is supplied, word-level diff
highlighting.

**`DocumentViewer`** composes the full surface: a header carrying the filename, a
file-type badge, the byte size, and a download button; a source-to-target toggle
shown only when target locales exist; and a tab strip. **Preview**, **Blocks**,
and **Stats** are always present. **Structure** appears when the tree carries
structural roles, **Layout** when it carries page geometry
([E-08](../engine/e-08-document-structure-tiers.md)), **Media** when it carries a
temporal or raster media layer ([M-03](../multilingual/m-03-multimodal-content.md)),
and **Raw** when the host actually holds the bytes. A host can append its own
tabs; the lab uses them for output-format pills. Downloading is the single
header button rather than a duplicate tab, because on the desktop the local file
is already the source of truth.

### Projection is declared per run kind

Turning a run sequence into text is the step every surface performs and the one
easiest to get wrong. The lossy form is a loop that keeps text runs and skips
the rest, which reads as "concatenate the text" and behaves as "delete every
placeholder, paired code and plural". Nothing fails; the content is gone.

So a projection is **declared**, never looped. A `RunSpec`
(`packages/kapi-format/src/run-projection.ts`) answers for every kind in
`RUN_KINDS`, and its type is a mapped type over that list, so a spec that omits
a kind does not compile. Each kind is rendered, expanded, dropped with a stated
reason, or declared unsupported and reported, in which case the spec's required
`fallback` is drawn in its place. A kind added to the model breaks every
projection until each has said what it does with it.
`scripts/check-run-projection.sh` keeps a hand-written loop from returning; it
runs in `make lint` and in CI.

The kit's render line therefore carries a block's inline codes beside its text,
so a placeholder is drawn as the same chip wherever it appears, and a plural
block reads its `other` branch behind a chip that says so rather than rendering
as an empty line. The chip names the variable a placeholder stands for, because
the JSX vocabulary is one the default registry loads.

### Run, vocabulary, rendering

Inline runs are styled through the **vocabulary registry**. The vocabulary packs
are canonical as a Go embed in `core/model/vocabularies`; `packages/ui/src/vocabularies`
carries a byte-identical mirror, and `scripts/check-vocab-packs.mjs` holds the
two equal in `make lint`. Resolving a run's type yields display information: a
category, a label, HTML and display templates, a chip label, a colour, an
equivalence text, and editing constraints. An **unknown** type is not an error:
the registry synthesizes an entry from a default fallback, interpolating the
type name into the templates, deriving a short chip label, and giving it a
neutral accent and permissive constraints. The editor uses those fields for
styled chips, tooltips, and the deletable, cloneable, and reorderable
affordances.

This mirrors the model contract exactly: the vocabulary is **descriptive**. It
drives display and editing affordances and is never consulted by writers, which
replay each run's own data verbatim ([F-02](../foundations/f-02-content-model.md)).

### Overlays become marks, keyed on type *and* span

Stand-off overlays become colour-coded, tooltipped marks. Three functions do the
work, and each embodies a decision.

**The accent key is the effective category.** A voice-vocabulary violation rides
on the generic `qa` overlay type, so an accent chosen on type alone would paint
it as an ordinary QA finding. The resolver therefore reads the span's own
category: a `qa` span categorised as voice vocabulary resolves to the voice
accent, and everything else resolves on its type. The style table gives
terminology a violet "Vocabulary" accent, voice a pink "Voice", other QA an
amber, entities a sky blue, segmentation and alignment their own muted accents,
and redaction an entry whose label and tooltip are used while the renderer paints
a censor bar instead of a background. Unknown keys fall back to a neutral
"Annotation" accent with a title-cased label, so a new overlay type renders
sensibly before anyone teaches the table about it.

**Spans are located by their text.** Overlays anchor to run-index ranges, but
the renderer works over the concatenated literal text, so matching by the
engine-extracted span text is robust across that projection. A per-overlay
search cursor advances past each match, so repeated identical span texts within
one overlay map to successive occurrences rather than all to the first. Spans
whose text cannot be located (a span covering only inline markup, for instance)
are dropped rather than mispositioned. One overlay type is excluded outright: a
content-memory leverage marker is a line-level fact, not a span highlight, and
the renderer styles the whole line from it.

**Overlaps flatten innermost-wins.** For each character position the narrowest
covering span owns it, and contiguous positions under the same owner emit one
non-overlapping segment, so a term inside an entity still shows as the term.

`BlockInspector` is the structural counterpart to the styled preview: a
collapsible per-block view rendering the source run sequence, each variant's row
(variant key, status, score, origin), each overlay's rows (type, side, layer, and
per-span id, run range, text, and properties), annotation rows, the properties
grid, and flag badges.

### The kit renders and inspects; it does not commit

`@neokapi/ui-primitives` is the single source of truth for the kit, exported
under `./preview`: the viewer and format preview, the file browser, the block
inspector, the structure, layout, and content-tree views, run sequences and code
views, the multimodal viewers for timed media and raster overlays, and the
supporting utilities (the render-doc normalizer, the overlay style and span
resolvers, run-range mapping, role styling, geometry flattening, and the
vocabulary registry).

| Consumer | How it uses the kit |
| --- | --- |
| Kapi Desktop | file inspection inside a project tab ([S-02](s-02-kapi-desktop.md)) |
| Kapi Lab | wraps the viewer to inspect engine output; the explorers use the tree and browser views |
| The playground and docs site | the same viewer over WASM-produced output |

The boundary is deliberate. The framework ships no production
translation-editing surface, and an application that needs one builds its commit
surface on the model's own primitive and its own persistence. Keeping the kit
free of an editing and commit dependency is exactly what lets every one of those
consumers share it as dependency-light UI.

### Edit path and round-trip

The editor-side `BlockIndex` carries a flattened, string-valued view; its
`UpdateTarget` method mutates that in-memory projection and is a **test helper**,
not a commit path.

The canonical way to commit a target edit is on the content model itself:
`model.Block.SetTargetRuns(locale, runs)` sets the variant's runs in place, with
the plain-text siblings `SetTargetText` and `SetText` for the string path
([E-03](../engine/e-03-tool-system.md)). How a host transports an edit to the
model and persists the result (the project block store, interchange files, a
database) is the host's concern.

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
  caption="The round-trip replays the source through its reader, injects the committed targets, and reconstructs byte-faithful output through the skeleton. Persistence is the host's concern."
/>

The round-trip is a framework mechanism independent of who edited: the source is
replayed through its reader, the committed targets are injected into the emitted
block parts, and the writer reconstructs the document by pairing the reader's
skeleton emitter with the writer's skeleton consumer, interleaving literal
skeleton fragments with the target runs rather than re-serializing a parse tree
([E-02](../engine/e-02-format-system.md)). The equivalent bilingual round-trip
for a translator hand-off is the extract and merge workflow
([M-01](../multilingual/m-01-bilingual-interop.md)); the equivalent for an
assistant-proposed edit is `kapi apply` ([S-03](s-03-agent-surfaces.md)).

### What persists, what is reconstructed

Within the content model and its interchange family
([M-06](../multilingual/m-06-content-packages.md)):

- **Targets** are first-class records: runs plus status, origin, and score.
- **Annotations** are the block-scoped typed carrier; the block file carries
  blocks, targets, and properties, and the JSON-Lines sidecar carries annotation
  overlays with their anchor kinds.
- **The skeleton** is the binary skeleton store ([E-02](../engine/e-02-format-system.md)).

**Overlays are reconstructed on demand** rather than serialized as positional
structure: segmentation is recomputed from the runs, and term, entity, and QA
overlays are re-attached by the tools that produce them within a session. Because
overlay spans anchor to run ranges, a source rewrite shifts or drops them through
the framework's overlay remapping; targets and annotations are unaffected.

## Consequences

- The editor has one documented contract shared by every consumer, so a new
  render kind, overlay accent, or inspector row is a contained change against a
  known seam.
- Rendering is a **pure projection** of the content model: the editor cannot
  diverge from what the engine holds, because it consumes the run-preserving
  tree rather than a separate model.
- A projection that forgets a run kind fails to compile, and a hand-written
  loop fails lint, so content cannot silently disappear from a view.
- Resolving the accent from type *and* span keeps one overlay type able to carry
  several kinds of finding without the renderer flattening them into one colour.
- Locating spans by text rather than by range makes the marks robust across the
  run-to-text projection, at the price of silently dropping a span whose text
  does not appear, which is the right trade when the alternative is a mark on
  the wrong words.
- Editing and committing stay **outside** the kit and outside the framework,
  which is what keeps the kit dependency-light and shareable.
- Faithful round-trip is a property of reader, skeleton, and writer, not of any
  editor: setting a target run sequence and replaying the source is what
  reconstructs the document.
- Overlays are ephemeral in the live preview: a durable interpretation must be
  stored as an annotation, or re-derived by re-running the tool that produced it.
- `BlockIndex.UpdateTarget` is a test helper. Relying on it as a commit path
  would be a mistake; `SetTargetRuns` is the canonical operation.

## Open questions

- **Overlay persistence.** The framework has no persistence layer for positional
  overlays; the JSON-Lines sidecar is defined but not yet read or written by the
  preview kit. Whether term, entity, and QA overlays should persist rather than
  always be re-derived is unsettled.
- **`BlockIndex` lifecycle on edit.** Whether a frontend holds its own index and
  sends delta edits, or re-derives it after each commit, is not pinned down.
- **Display-hint and content-reference scope.** Whether these are populated by
  every reader and persisted by a given host store is format- and
  application-dependent.
- **Segment span ids across edits.** Whether overlay span ids stay stable when
  source text changes, or shift with rebasing.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): parts, blocks, runs, overlays, annotations, targets, and the projections the editor renders
- [E-02: The format system](../engine/e-02-format-system.md): readers, writers, and the skeleton that makes the round-trip byte-faithful
- [E-03: The tool system](../engine/e-03-tool-system.md): capability-typed immutability, the target-edit primitives, and overlay rebasing
- [E-08: Document structure tiers](../engine/e-08-document-structure-tiers.md): the roles and geometry the Structure and Layout tabs read
- [C-01: The project model](../context/c-01-project-model.md): the block store a host persists edits through
- [M-01: Bilingual interop](../multilingual/m-01-bilingual-interop.md): the extract and merge round-trip
- [M-03: Multimodal content](../multilingual/m-03-multimodal-content.md): the media layer the multimodal viewers render
- [M-06: Content packages](../multilingual/m-06-content-packages.md): the block files and the annotation sidecar
- [S-02: Kapi Desktop](s-02-kapi-desktop.md): the application that hosts the kit
