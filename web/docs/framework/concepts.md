---
id: concepts
title: Concepts
sidebar_label: Concepts
sidebar_position: 2
description: "A one-line definition of every core neokapi term (Part, Layer, Block, Run, Edition, Overlay, EditionKey, and the surrounding vocabulary), linked from first use across the framework docs."
---

# Concepts

Each core framework term has a one-line definition, an analogy where one helps,
and a link to the page that covers it in full. The concept pages link here when
they first use a term.

## The content model

These are the types that flow through the [pipeline](/framework/pipeline) and
make up the [content model](/framework/content-model).

| Term | Definition | Analogy | Covered in |
| --- | --- | --- | --- |
| **Part** | The streaming unit. Every document is read as a stream of Parts: layers starting and ending, blocks, data, media. | An event in an event stream | [Content Model](/framework/content-model) |
| **Layer** | A structural grouping: a document, a section, or embedded content. Layers nest: HTML inside a JSON string becomes a child Layer with its own format. | A node in the document tree | [Content Model](/framework/content-model) |
| **Block** | A unit of content: its Editions, each a flat sequence of Runs, and any stand-off Overlays. Segments are an Overlay on the Block's Runs, and the Block stays whole. | A paragraph or a message | [Content Model](/framework/content-model) |
| **Run** | The inline unit inside a Block: a discriminated union of Text and inline codes (placeholders, paired open/close codes, sub-flows, plural and select structures). Inline markup lives in Runs, never in the text itself. | A text node or a tag in an HTML fragment | [Inline Formatting](/framework/inline-formatting) |
| **Edition** | One rendition of a Block's content, keyed by an EditionKey: the edition the document is written in, and one per translation, tone or channel. Editions are peers. The others derive from the Edition in the project's source language, or in the document's own language outside a project. | One language version of a page | [Content Model](/framework/content-model) |
| **EditionKey** | The key that identifies an Edition: a locale plus optional tone or channel, such as `fr`, `fr;tone=formal` or `en;channel=short`. | The address of one rendition | [Content Model](/framework/content-model) |
| **Overlay** | Stand-off annotation anchored to a range of Run indices: segmentation, terms, entities, check findings, alignment. The source stays as it is. | A margin note pinned to a span of text | [Content Model](/framework/content-model) |
| **Anchor** | Where an Overlay or a Finding attaches: a path into the Block's Runs with a start and end offset, or a key. The annotation survives edits to the text around it and can be re-resolved after the Runs change. | | [Content Model](/framework/content-model) |
| **Resource** | The payload a Part carries: a Block, Data, or Media. | The Part is the envelope; the Resource is the letter | [Content Model](/framework/content-model) |
| **Data** | Structure that is never translated but must survive the round trip, such as keys, attributes and layout. | | [Content Model](/framework/content-model) |
| **Media** | Binary content, such as an image or an audio or video clip, carried through the pipeline. | | [Content Model](/framework/content-model) |

## Vocabularies and inline codes

| Term | Definition | Covered in |
| --- | --- | --- |
| **Vocabulary** | The typed catalogue of inline-code meanings (formatting, links, code spans, placeholders) that a Run's type draws from, with rules for what may be deleted, cloned or reordered. | [Vocabularies](/framework/vocabularies) |
| **Semantic type** | The meaning attached to an inline code, for example "bold", "hyperlink" or "variable", the same in every format. | [Vocabularies](/framework/vocabularies) |

## Processing

| Term | Definition | Analogy | Covered in |
| --- | --- | --- | --- |
| **Format** (reader / writer) | The paired components that parse a byte stream into Parts and write Parts back out byte-for-byte. | A codec | [Formats](/framework/formats) |
| **Tool** | A single processing step that reads Parts and writes Parts. Tools compose. | A stage in a shell pipeline | [Tools](/framework/tools) |
| **Flow** | A named composition of tools. | A saved pipeline definition | [Flows](/framework/flows) |
| **Pipeline / Executor** | The concurrent engine that runs a flow's tools as goroutines connected by channels. | | [Pipeline](/framework/pipeline) |
| **Round trip** | Reading a document into the content model and writing it back out so that untouched content is byte-for-byte identical. Format tests check this property for every format. | | [Formats](/framework/formats) |

## Stores and context

| Term | Definition | Covered in |
| --- | --- | --- |
| **Content memory** | A store of previously settled content: source segments paired with the targets produced for them, matched exactly or fuzzily to reuse past work. | [Content memory](/framework/content-memory) |
| **Terms store** | A store of approved terminology that keeps term renderings consistent. | [Terminology](/framework/terminology) |
| **Voice profile** | The tone, style measures and pattern rules content is checked and produced against (`core/profile`). Word rules are terms. | [Voice](/framework/checks/voice) |
| **Point** (coordinates) | Where a piece of content sits in a project's context space, on axes such as brand and product. A voice profile, a terms store and gates are bound at a point, and all content at that point follows them. | [Projects](/kapi/projects) |

## Checks

| Term | Definition | Covered in |
| --- | --- | --- |
| **Check** | A test that runs over content and emits findings, the content analogue of a unit test. | [Checks](/framework/checks) |
| **Finding** | A single issue a check reports, anchored to where it occurs and marked as failing the check or as report-only. Findings travel with the content as overlays. | [Checks](/framework/checks) |

:::note Project vocabulary
The recipe (`kapi.yaml`) and its model are framework code (`core/project`), as
are the context, gate and unit-state packages beside it; the verbs that drive a
project (`up`, `status`, `apply`) are kapi commands. Ad-hoc and project modes,
bindings, and the `.kpz`/`.kbf.json` formats are defined under
[Projects](/kapi/projects).
:::

## Related

- [Architecture](/framework/architecture): how these pieces fit together.
- [Content Model](/framework/content-model): the types above in full.
- [Go quickstart](/framework/go-quickstart): the concepts in a few lines of Go.
