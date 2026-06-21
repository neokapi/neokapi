---
sidebar_position: 12
title: Content preparation
description: Before content is translated, neokapi prepares it in one annotation pass — settle the source, segment it, recognize terms and entities, and check it. Each step is a run-anchored overlay on the same canonical block, so nothing is lost and everything downstream reads one settled model.
keywords: [pre-processing, authoring, content preparation, segmentation, terminology, entities, checks, source transform, localization pipeline]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# Content preparation

Most of the value in a localization pipeline is decided *before* the first word is
translated. Is the source clean and settled? Is it segmented into the units a
translator or model should work on? Are the terms and named entities recognized,
so they can be enforced, protected, or reused? Has the content been checked?

neokapi treats this as one **content-preparation pass**: a sequence of stages
that annotate the source without destroying it. Every stage adds a run-anchored
[stand-off overlay](/framework/content-model) to the same canonical block — the
source runs are written once, settled, and then read by everything downstream.

<PipelineDiagram
  channelLabel=""
  stages={[
    { label: "source", role: "io" },
    { label: "settle", sub: "transformers" },
    { label: "segment", sub: "sentence overlay", role: "annotate" },
    { label: "recognize", sub: "terms · entities", role: "annotate" },
    { label: "check", sub: "QA findings", role: "qa" },
    { label: "translate", sub: "TM · AI · MT", role: "translate" },
  ]}
/>

This page is the map; each stage links to its own concept page.

## 1. Settle the source

Some operations rewrite the source itself — [redaction](/framework/redaction)
replacing sensitive spans with placeholders, a normalizer, a simplifier. These
[transformers](/framework/flows#transformers) are ordinary ordered steps: the
framework applier performs each rewrite inline and in order, rebasing any
surviving run-anchored overlays (segments, term and entity spans) onto the new
runs, so **each transformer settles the source before later steps observe
it**. Placing a transformer early keeps that rebasing to a minimum — the
flow's placement pass warns when one sits later than its earliest valid slot
and rejects unsafe orderings outright.

## 2. Segment

[Segmentation](/framework/segmentation) marks the boundaries — usually sentences
— that translation and TM key on. It is an overlay, not a split, so a block can
carry a sentence layer for translation alongside a coarser chunk layer for an LLM,
and the unsegmented block is always recoverable. Choose a rule-based engine (SRX,
the localization standard), a Unicode baseline (UAX-29), an LLM for semantic
chunks, or the SaT ML model for text that rules segment poorly.

## 3. Recognize the named things

Two overlays capture the named things in the source — and both exist for the
outcome they enable, not as ends in themselves:

- **[Terminology](/framework/terminology)** — `term-lookup` matches the project
  [termbase](/framework/terminology) against the source and attaches the concept,
  its preferred translations, and its status. This is both a translation
  *resource* (a glossary that feeds AI translation) and the basis for enforcement.
- **Entity detection** — people, organizations, products, locations, dates and
  more are recognized automatically (a fast local model, an LLM, or both). You
  never run this as its own task: it is the detection that powers
  [redaction](/framework/redaction) (protect sensitive spans) and
  entity-generalized [translation-memory](/framework/translation-memory) reuse
  (match across every value of a name). Detection skips terms the termbase
  already covers, so the two passes complement rather than duplicate each other.

## 4. Check

[Checks](/framework/checks) are tests for content: deterministic verifiers that
read the source (and, after translation, the target) and report
[findings](/framework/checks) without modifying anything. In the preparation
pass, source-side checks catch problems early — doubled words, suspicious
patterns, off-vocabulary brand terms — and the same engine runs the bilingual
checks (placeholder integrity, do-not-translate survival, terminology
enforcement) after translation. Run as a gate, `kapi check` exits non-zero so CI
or an assistant's fix-loop acts on the findings.

## One settled model, many readers

The point of doing all of this as overlays on one block is that **every
downstream reader sees the same canonical source**:

- [Translation memory](/framework/translation-memory) matches on the segment
  layer and can generalize over entity spans.
- [AI translation](/framework/ai-translation) and [MT](/framework/mt-services)
  translate per segment, with the matched terminology injected as guidance and
  do-not-translate entities protected.
- Checks point findings at the exact run range that broke — a sentence, a term, a
  placeholder.

Nothing is re-parsed or re-derived between stages, and removing any overlay
returns the block to its prior state.

## Putting it in a flow

The preparation pass is an ordinary [flow](/framework/flows): one ordered list
of steps — a transformer to settle the model, then annotation steps, then
translation, then a check gate.

```yaml
steps:
  - tool: redact              # settle the source first (optional)
  - tool: segmentation        # sentence boundaries
    config: { engine: srx }
  - tool: term-lookup         # match the termbase
  - tool: entity-extract   # recognize entities
  - tool: tm-leverage         # reuse prior segment translations
  - tool: translate           # translate the remainder
  - tool: qa                  # gate on findings
```

In a [`.kapi` project](/reference/project-file) this lives as a named flow so
every run prepares content the same way and the overlays feed the project-local
TM and termbase. For a runnable, step-by-step version, see the
[Prepare content for translation](/kapi/recipes/prepare-content) recipe.
