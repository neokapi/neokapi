---
id: concepts
title: Concepts
sidebar_label: Concepts
sidebar_position: 2
description: "A one-line definition of every core neokapi term (Part, Layer, Block, Run, Target, Overlay, VariantKey, and the surrounding vocabulary), linked from first use across the framework docs."
---

# Concepts

The framework has a small, precise vocabulary. This page defines each term once,
in one place, so the concept pages can link here from first use instead of
re-explaining. Each entry gives a one-line definition, a quick analogy, and a
link to where the idea is developed in full.

## The content model

These are the types that flow through the [pipeline](/framework/pipeline) and
make up the [content model](/framework/content-model).

- **Part**: the fundamental streaming unit. Every document is read as a stream
  of Parts (layers starting and ending, blocks, data, media). *Analogy:* an
  event in an event stream. See [Content Model](/framework/content-model).

- **Layer**: a structural grouping: a document, a section, or a piece of
  embedded content. Layers nest: HTML inside a JSON string becomes a child
  Layer with its own format. *Analogy:* a node in the document tree.

- **Block**: a unit of translatable content: a flat sequence of Runs (the
  source), its Targets, and any stand-off Overlays. *Analogy:* a paragraph or a
  message. Segmentation is an **Overlay** (defined below) rather than a
  separate segment type or a structural split.

- **Run**: the inline unit inside a Block: a discriminated union of Text and
  inline codes (placeholders, paired open/close codes, sub-flows, plural and
  select structures). Inline markup lives in Runs, never in the text itself.
  *Analogy:* a text node or a tag in an HTML fragment. See
  [Inline Formatting](/framework/inline-formatting).

- **Target**: the translated (or otherwise produced) counterpart of a Block's
  source, keyed by **VariantKey** (defined below). A Block can carry many Targets at
  once.

- **Overlay**: stand-off annotation anchored to a range of Run indices:
  segmentation, terms, entities, check findings, alignment. Overlays describe the
  content without altering the source. *Analogy:* a margin note pinned to a span
  of text. See [Content Model](/framework/content-model).

- **Anchor**: where an Overlay or a Finding attaches: a path into the Block's
  Runs with a start and end offset, or a key, so the annotation survives edits
  to the text around it and can be re-resolved after the Runs change. See
  [Content Model](/framework/content-model).

- **VariantKey**: the key that identifies a Target: a locale plus optional tone
  or channel. *Analogy:* the address of one rendition of a Block (e.g. `fr`,
  or `fr` + "formal").

- **Resource**: the payload a Part carries (a Block, Data, or Media). The Part
  is the envelope; the Resource is the content.

- **Data**: non-translatable structure that must survive the round trip (keys,
  attributes, layout) but is never translated.

- **Media**: binary content (an image, an audio or video clip) carried through
  the pipeline.

## Vocabularies & inline codes

- **Vocabulary**: the typed catalogue of inline-code meanings (formatting,
  links, code spans, placeholders) that a Run's type draws from, with rules for
  what may be deleted, cloned, or reordered. See
  [Vocabularies](/framework/vocabularies).

- **Semantic type**: the meaning attached to an inline code (for example
  "bold", "hyperlink", "variable"), independent of how any one format spells it.

## Processing

- **Format** (reader / writer): the paired components that parse a byte stream
  into Parts and write Parts back out byte-for-byte. *Analogy:* a codec: decode
  in, encode back out. See [Formats](/framework/formats).

- **Tool**: a single processing step that reads Parts and writes Parts. Tools
  compose. *Analogy:* a stage in a shell pipeline. See [Tools](/framework/tools).

- **Flow**: a named composition of tools. *Analogy:* a saved, shareable
  pipeline definition. See [Flows](/framework/flows).

- **Pipeline / Executor**: the concurrent engine that runs a flow's tools as
  goroutines connected by channels. See [Pipeline](/framework/pipeline).

- **Round trip**: reading a document into the content model and writing it back
  out so that untouched content is byte-for-byte identical. The fidelity
  guarantee the whole engine is built around. See [Formats](/framework/formats).

## Knowledge stores

- **Content memory**: a store of previously settled content: source segments
  paired with the targets produced for them, matched exactly or fuzzily to reuse
  past work. See [Content memory](/framework/content-memory).

- **Terms store**: a store of approved terminology, used to keep term
  renderings consistent. See [Terminology](/framework/terminology).

- **Voice profile**: the tone, wording and vocabulary rules content is checked
  and produced against (`core/profile`). See [Voice](/framework/checks/voice).

## Context

- **Point** (coordinates): where a piece of content sits in a project's context
  space, on axes such as brand, product and channel. A voice profile, a terms
  store and gates are bound at a point, so content at the same point is governed
  the same way. See [Projects](/kapi/projects).

## Checks

- **Check**: a test that runs over content and emits findings (the content
  analogue of a unit test). See [Checks](/framework/checks).

- **Finding**: a single issue a check reports, anchored to where it occurs and
  carrying a severity. Findings travel with the content as overlays.

:::note Project vocabulary
The recipe (`kapi.yaml`) and its model are framework code (`core/project`), as
are the context, gate and unit-state packages beside it; the verbs that drive a
project (`up`, `status`, `apply`) are Kapi's. Ad-hoc vs. project modes,
bindings, and the `.kpz`/`.kbf.json` formats are defined under
[Projects](/kapi/projects).
:::
