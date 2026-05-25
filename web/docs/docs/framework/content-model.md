---
sidebar_position: 2
title: Content Model
description: The neokapi content model — how documents are represented as a stream of Parts (Layer, Block, Segment, Run, Data, Media) so that tools and translations work independently of the source file format.
keywords: [content model, Part, Block, Segment, Run, Layer, localization, format-independent]
---

import { BlockPreview } from "@site/src/components/curated";
import { AnatomyExplorer } from "@site/src/components/Lab";

# Content Model

The content model is the vocabulary every part of neokapi shares. Whatever the
input format — JSON, XLIFF, HTML, DOCX — a [reader](/framework/formats) turns it
into the same handful of types, so [tools](/framework/tools),
[flows](/framework/flows), [translation memory](/framework/translation-memory),
and editors all work against one representation rather than against each format's
quirks. It is a deliberate, format-independent abstraction over localizable
content, modeled on the Okapi Framework's resource hierarchy.

:::tip Try it — anatomy of a file
Pick a sample or drop in your own file and see exactly how a reader decomposes it
into Layers, Groups, Blocks, and **Runs**. Notice that an HTML `<strong>` becomes
a paired inline code inside a block's run sequence, while a JSON `{name}` stays
literal text — that is format-awareness in action. This runs the real `kapi`
reader in your browser via WebAssembly.
:::

<AnatomyExplorer defaultSampleId="page-html" />

## The Part is the streaming unit

A document is not loaded as a tree and handed around whole. It flows through the
[pipeline](/framework/pipeline) as a stream of **Parts**, the indivisible unit
that travels over the channels between stages. Each Part carries a type
discriminator and a resource payload: a layer start or end, a translatable block,
non-translatable structural data, or media. A reader emits Parts as it parses;
tools transform the Parts they care about and relay the rest; a writer
reconstructs the document from the stream.

A typical small JSON document with one embedded HTML value produces a stream like
this:

```
Read(ctx) ─▶ PartLayerStart  (format = "json")
          ─▶ PartBlock        ("title")
          ─▶ PartLayerStart  (format = "html")   ← embedded child layer
          ─▶ PartBlock        ("Hello <b>world</b>")
          ─▶ PartLayerEnd    (format = "html")
          ─▶ PartBlock        ("footer")
          ─▶ PartLayerEnd    (format = "json")
          ─▶ (channel closed)
```

Streaming is why the model is shaped around a Part rather than a document tree:
it keeps memory bounded and lets stages run concurrently. The mechanics are
covered in [Pipeline](/framework/pipeline).

## The resource types

The payload a Part carries is one of a few resource types. Together they describe
both the content a translator works on and the structure that surrounds it.

```mermaid
classDiagram
    class Layer {
        +string Format
        +Layer Parent
    }
    class Block {
        +bool Translatable
        +[]Segment Source
        +map~Locale,[]Segment~ Targets
    }
    class Segment {
        +[]Run Runs
    }
    class Run {
        +TextRun Text
        +PlaceholderRun Ph
        +PcOpenRun PcOpen
        +PcCloseRun PcClose
    }
    Layer --> Layer : child Layers (embedded content)
    Layer --> Block : contains
    Block --> Segment : Source, Targets
    Segment --> Run : flat run sequence
```

- **Layer** — a structural grouping: a whole document, a section, or embedded
  content. Layers nest. Embedded content — HTML inside a JSON value, CDATA inside
  XML — becomes a **child layer** with its own format, so the right reader handles
  it and inline markup is preserved at every level rather than being flattened.
- **Block** — the primary translatable unit (Okapi's _TextUnit_). A block holds a
  source and, per target locale, a translation. It carries a `Translatable` flag,
  arbitrary properties, and **annotations** — the shared channel through which
  [TM matches](/framework/translation-memory),
  [terminology](/framework/terminology), [brand-voice](/framework/brand-voice)
  findings, and [QA](/framework/qa-checks) results all attach to content without
  colliding.
- **Segment** — a block's source or target is a list of segments (typically
  sentences after [segmentation](/framework/tools)), each carrying a flat `Runs`
  sequence.
- **Run** — one element of a segment's inline content: a chunk of text, an
  opening or closing inline tag, a self-closing placeholder, or a structured
  plural/select construct (see below).
- **Data** and **Media** — non-translatable document structure and binary
  content, which flow through so the writer can reconstruct a faithful output.

## Runs keep inline markup out of the way

The Run sequence is where neokapi solves a hard problem: how to let a tool, a
translation engine, or a TM operate on the words while keeping inline markup like
`<b>`, `**`, or `{count}` intact. A segment's content is a flat `[]Run` — a
discriminated union where each run is exactly one of:

| Run kind        | Field      | Represents                                     |
| --------------- | ---------- | ---------------------------------------------- |
| Text            | `Text`     | a plain text chunk                             |
| Placeholder     | `Ph`       | a self-closing token (`<br/>`, `<img>`, `{n}`) |
| Paired open     | `PcOpen`   | the opening half of a paired code (`<b>`, `<a>`) |
| Paired close    | `PcClose`  | the closing half of a paired code (`</b>`, `</a>`) |
| Sub             | `Sub`      | a reference to a nested sub-block (subfilter output) |
| Plural / Select | `Plural` / `Select` | a structured ICU construct with per-form runs |

Bold text becomes a `PcOpen` / text / `PcClose` triple; a `<br/>` or a variable
becomes a single `Ph`. The original markup is carried in the run's `Data` field,
so the writer can replay it verbatim:

```
Source HTML: Click <b>here</b> for info

Segment.Runs:
  - {Text: "Click "}
  - {PcOpen:  {ID: "1", Type: "fmt:bold", Data: "<b>"}}
  - {Text: "here"}
  - {PcClose: {ID: "1", Type: "fmt:bold", Data: "</b>"}}
  - {Text: " for info"}
```

A tool can project the runs to plain text (`Segment.Text()` returns
`"Click here for info"`); a translation engine sees text with opaque tokens it
must preserve; and the writer re-emits each run's `Data` at its position to
reconstruct the source faithfully — attributes and all. Because the same `<b>`,
Markdown `**`, and DOCX `<w:b/>` all reduce to a `PcOpen`/`PcClose` pair of the
same semantic `Type`, the representation is format-independent.
[Inline Formatting](/framework/inline-formatting) and
[Vocabularies](/framework/vocabularies) cover how runs are classified and what
metadata they carry.

> A coded-text exchange form (a string with private-use-area markers and a
> parallel `Span` list, mirroring Okapi's `TextFragment`) historically backed
> inline content. It has been removed; `[]Run` is the canonical representation.

## See it on a real file

The clearest way to understand the content model is to watch a reader produce it.
Below, kapi parses a small JSON localization file into blocks — each with an
identifier and its source text:

<BlockPreview
  sample="messages.json"
  caption="A JSON file parsed into the content model: blocks, ids, and source text."
/>

The same parser run against an HTML page shows runs with inline codes (the
chips mark the `PcOpen`/`PcClose`/`Ph` runs lifted out of the text):

<BlockPreview
  sample="page.html"
  caption="An HTML page — note the span markers where inline elements were extracted."
/>

## Reconstruction with skeletons

Translatable blocks are only part of a document; the rest is structure —
surrounding tags, whitespace, keys, attributes. A **skeleton** captures that
non-translatable structure interleaved with references to block content, so the
writer can rebuild the document exactly, substituting translated content where a
target exists and falling back to source where it does not. This is what gives
neokapi roundtrip fidelity: read a file and write it back unchanged, or write it
back with only the translated text differing.

## Mapping from Okapi

For readers familiar with the Okapi Framework, the model maps directly:

| Okapi (Java)                    | neokapi (Go)               |
| ------------------------------- | -------------------------- |
| Filter                          | DataFormat                 |
| TextUnit                        | Block                      |
| TextFragment                    | Segment (`[]Run`)          |
| Code                            | Run (`PcOpen`/`PcClose`/`Ph`) |
| StartSubDocument/StartSubFilter | Child Layer                |
| Event                           | Part                       |

## Related reading

- [Formats](/framework/formats) — the readers and writers that produce and consume the model.
- [Inline Formatting](/framework/inline-formatting) and [Vocabularies](/framework/vocabularies) — how inline-code runs are represented and classified.
- [Pipeline](/framework/pipeline) — how Parts stream through the executor.
- [Interface Reference](/contribute/interfaces) — the concrete Go types and method signatures.
- [AD-002: Content Model](/contribute/architecture/002-content-model) — the design rationale.
