---
sidebar_position: 12
title: Content preparation
description: "Before content is translated, neokapi prepares it in one annotation pass: settle the source, segment it, recognize terms and entities, and check it. Each step is a run-anchored overlay on the same canonical block, and every later stage reads one settled model."
keywords: [pre-processing, authoring, content preparation, segmentation, terminology, entities, checks, source transform, translation pipeline]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# Content preparation

Four questions come *before* the first word is translated. Is the source clean
and settled? Is it segmented into the units a translator or model should work
on? Are the terms and named entities recognized, so they can be enforced,
protected or reused? Has the content been checked?

neokapi answers them in one **content-preparation pass**: a sequence of stages
that annotate the source and keep it intact. Each stage adds a run-anchored
[stand-off overlay](/framework/content-model) to the same canonical block. The
source runs are written once, settled, and then read by every later stage.

<PipelineDiagram
  channelLabel=""
  stages={[
    { label: "source", sub: "binding", role: "io" },
    { label: "settle", sub: "transformers" },
    { label: "segment", sub: "sentence overlay", role: "annotate" },
    { label: "recognize", sub: "terms · entities", role: "annotate" },
    { label: "recycle", sub: "memory reuse", role: "translate" },
    { label: "translate", sub: "AI · LLMProvider", role: "translate" },
    { label: "check", sub: "findings", role: "qa" },
    { label: "sink", sub: "binding", role: "io" },
  ]}
/>

Each stage below links to its own concept page.

## 1. Settle the source

Some operations rewrite the source itself: [redaction](/framework/redaction)
replacing sensitive spans with placeholders, a normalizer, a simplifier. These
[transformers](/framework/flows#transformers) are ordinary ordered steps: the
framework applier performs each rewrite inline and in order, rebasing any
surviving run-anchored overlays (segments, term and entity spans) onto the new
runs, so **each transformer settles the source before later steps observe
it**. Placing a transformer early keeps that rebasing to a minimum: the
flow's placement pass warns when one sits later than its earliest valid slot
and rejects unsafe orderings outright.

## 2. Segment

[Segmentation](/framework/segmentation) marks the boundaries (usually sentences)
that translation and content memory key on. Segments are an overlay on the block, so a
block can carry a sentence layer for translation alongside a coarser chunk layer
for an LLM, and the unsegmented block is always recoverable. Choose a rule-based
engine (SRX, the Segmentation Rules eXchange standard), a Unicode baseline
(UAX-29), an LLM for semantic chunks, or the SaT ML model for text that rules
segment poorly.

## 3. Recognize the named things

Two overlays capture the named things in the source:

- **[Terminology](/framework/terminology)**: the project
  [terms store](/framework/terminology) is matched against the source, and each
  hit carries the concept, its preferred translations, and its status.
  `term-check` reports the findings; `translate` reads the matching term rules
  itself. The matches serve as term guidance for AI translation and as the
  basis for enforcement.
- **Entity detection**: people, organizations, products, locations, dates and
  more are recognized automatically (a fast local model, an LLM, or both). You
  don't run it as a task of its own. It feeds
  [redaction](/framework/redaction) (protect sensitive spans) and
  entity-generalized [content-memory](/framework/content-memory) reuse
  (match across every value of a name). Detection skips terms the terms store
  already covers, so each span is recognized by one pass only.

## 4. Check

[Checks](/framework/checks) are tests for content: deterministic verifiers that
read the source (and, after translation, the target) and report
[findings](/framework/checks) without modifying anything. In the preparation
pass, source-side checks catch problems early (doubled words, suspicious
patterns, off-vocabulary terms) and the same engine runs the bilingual
checks (placeholder integrity, do-not-translate survival, terminology
enforcement) after translation. Run as a gate, `kapi check` exits non-zero so CI
or an assistant's fix-loop acts on the findings.

## What later stages read

Every stage writes overlays on one block, so **every later reader sees the same
canonical source**:

- [Content memory](/framework/content-memory) matches on the segment
  layer and can generalize over entity spans.
- [Translation](/framework/translation) runs per segment,
  with the matched terminology injected as guidance and do-not-translate
  entities protected.
- Checks point findings at the exact run range that broke: a sentence, a term, a
  placeholder.

Nothing is re-parsed or re-derived between stages, and removing any overlay
returns the block to its prior state.

## Putting it in a flow

The preparation pass is an ordinary [flow](/framework/flows): a transformer to
settle the model, then annotation steps, then translation, then a check gate.
In a [`kapi.yaml` project](/reference/project-file) it is a named flow, so every
run prepares content the same way and the overlays feed the project's content
memory and terms store.

## Related

- [Prepare content for translation](/kapi/recipes/prepare-content): the commands and the flow YAML, step by step.
- [Segmentation](/framework/segmentation) and [Terminology](/framework/terminology): the two annotation stages in full.
- [Checks](/framework/checks): the check stage and its gates.
