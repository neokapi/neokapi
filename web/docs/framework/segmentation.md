---
sidebar_position: 13
title: Segmentation
description: "Segmentation is the means that makes content-memory reuse work and lets translation and checks operate per sentence. neokapi records it as a run-anchored stand-off overlay produced by a pluggable engine (SRX, UAX-29, an LLM, or the SaT ML model), never a destructive split."
keywords: [segmentation, SRX, UAX-29, sentence boundary, SaT, wtpsplit, overlay, content memory, reuse]
---

import { StreamDiagram } from "@neokapi/docs-shared";

# Segmentation

Segmentation is a **means**: authors don't set out "to segment," they
want their past translations reused and each sentence translated and checked on
its own. Segmentation is what makes both possible:
[content memory](/framework/content-memory) is a store of *segment*
pairs, so boundaries are what let prior sentences match, and per-segment
translation and [checks](/framework/checks) keep each unit small and a finding
pinned to the sentence that broke. This page is the engine reference for the
framework users who configure that behavior.

It divides a block's source into the units a translator or model works on,
usually sentences. In neokapi segmentation is a **run-anchored stand-off
overlay** rather than a structural split: the boundaries are recorded as spans over the
existing runs, and the source runs themselves are never rewritten
([F-02: Content Model](/contribute/architecture/foundations/f-02-content-model)). A block
can carry several segmentation layers at once, and removing the overlay restores
the unsegmented block exactly.

<StreamDiagram
  title="segment (overlay only; source runs unchanged)"
  items={[
    { kind: "Block source", detail: '"Dr. Smith arrived. He was late."', role: "block" },
    { kind: "segmentation overlay", role: "meta", note: "anchored to run-index ranges" },
    { kind: "segment", detail: '[0, 18) · "Dr. Smith arrived."', depth: 1, role: "layer" },
    { kind: "segment", detail: '[19, 31) · "He was late."', depth: 1, role: "layer" },
  ]}
/>

Because segmentation is an overlay rather than an edit, it is a [check-like
annotation](/framework/content-model): it is produced after any
[transformers](/framework/flows#transformers) have settled the source, so
every boundary is anchored to the canonical runs that translation and content
memory will also see. If a later transformer does rewrite the source, the framework
applier rebases the boundaries onto the new runs.

## Why stand-off

A structural split forces a choice at parse time and is lossy: re-joining
segments to recover the original paragraph is fiddly, and inline markup that
straddles a boundary has to be duplicated. An overlay avoids both. The same
block can hold:

- a **sentence** layer for translation and memory lookup,
- a coarser **clause** or **chunk** layer for an LLM that prefers larger units,

and a tool that ignores segmentation still sees a clean, whole block. Bilingual
formats that need standalone segments (XLIFF `<mrk>`, TMX-style pairs)
materialize them from the primary layer at projection time; the in-memory model
keeps the runs intact.

## Engines

The `segmentation` tool selects a segmenter backend by name through `--engine`.
Each engine writes the same overlay; they differ in how they find boundaries and
what they cost to run.

| Engine | Boundary source | Needs | Use it when |
| --- | --- | --- | --- |
| `srx` (default) | SRX 2.0 rules: the full ruleset over a UAX-29 base where ICU is linked, a reduced pure-Go ruleset otherwise | nothing (pure Go); uses ICU when present | You want deterministic, language-tunable sentence boundaries, the industry-standard rule format. |
| `uax29` | Unicode UAX-29 sentence rules (ICU) | cgo + ICU | You want the bare Unicode baseline with no exception rules and ICU is available. |
| `llm` | An LLM asked to chunk semantically | an [LLM provider](/framework/translation) | You want meaning-aware chunks (long-form prose, mixed content) rather than sentence boundaries. |
| `sat` | The wtpsplit *Segment any Text* ONNX model | the `kapi-sat` plugin | You need robust multilingual or unpunctuated-text segmentation that rules handle poorly. |

`srx`, `uax29` and `sat` produce a **sentence** layer; `llm` produces an
`llm-chunk` layer. Leave `--layer` empty to accept the engine's natural layer, or
set it to keep several layers side by side.

### SRX: the default

SRX (Segmentation Rules eXchange) is the GALA/LISA standard for sentence
segmentation: an ordered list of break and no-break rules, scoped by language.
neokapi ships a faithful pure-Go SRX 2.0 rule engine, so it runs everywhere with
no native dependencies, including in the browser.

#### Hybrid by default, pure-Go everywhere

SRX segmentation in practice is a **hybrid**: ICU's UAX-29 breaker proposes the
sentence boundaries and the SRX ruleset is applied on top as *exceptions*: the
default ruleset is overwhelmingly no-break rules, scoped per language, with only
a handful of break rules. neokapi implements this hybrid directly:

- **Where ICU is linked** (every shipped native binary: CLI, desktop, server),
  the `srx` default loads the full default ruleset and runs the ICU-base +
  SRX-exception hybrid, verified against a per-language golden corpus.
- **Where ICU is not linked** (the browser/WASM build, pure-Go source builds),
  the same `srx` engine falls back to a reduced, self-contained ruleset with
  explicit break rules, with no ICU needed. It is lighter than the full set but
  still handles the common abbreviations, decimals, and initials, and it is the
  only segmenter that runs in the browser.

You don't choose between these: the `srx` engine picks the right path for the
build. The result is full-fidelity SRX segmentation where ICU is available, and
a pure-Go approximation everywhere else.

To tune boundaries (protect a domain abbreviation, split on a custom marker),
point the engine at your own SRX file (an explicit `--rules-path` overrides the
adaptive default in either mode):

```bash
kapi exec segmentation src/locales/en.json --engine srx \
  --rules-path .kapi/rules.srx
```

One file serves both sides: SRX rules are keyed by language, so the same ruleset
supplies the source rules and, when `--segment-target` is set, the target rules.

For a quick, file-free tweak the tool also accepts an inline `rules:` list in
its config (break / no-break regex pairs); an inline list overrides the engine
selection. For anything beyond a rule or two, prefer a real SRX file with
`--rules-path`, so the rules are portable and shareable.

### SaT: the ML segmenter

The `sat` engine runs wtpsplit *Segment any Text* models (XLM-RoBERTa-based ONNX)
through the out-of-process `kapi-sat` plugin, so its native ML stack never enters
the `kapi` binary. It is the right choice for text that rule engines segment
poorly: languages without reliable sentence punctuation, user-generated or
transcribed text, and mixed-script content. The plugin must be installed
(`kapi plugin install kapi-sat`) before the engine is available; once it is,
the plugin's own options appear on `kapi exec segmentation`: `--sat-model`
selects a model (e.g. `sat-3l-sm` for speed, `sat-12l-sm` for accuracy) and
`--threshold` makes boundaries more or less eager. See
[M-02: SaT Segmenter Plugin](/contribute/architecture/multilingual/m-02-segmentation)
for the protocol and isolation design.

## CLI usage

`kapi exec segmentation` annotates files in place with a segmentation overlay:

```bash
# Sentence-segment the source with the default SRX engine
kapi exec segmentation src/locales/en.json

# Use a custom SRX rule file
kapi exec segmentation README.md --engine srx --rules-path .kapi/rules.srx

# Semantic chunks via an LLM provider
kapi exec segmentation docs/guide.md --engine llm --provider anthropic

# ML segmentation with SaT (options supplied by the kapi-sat plugin)
kapi exec segmentation transcript.txt --engine sat --sat-model sat-3l-sm
```

Useful flags: `--segment-source` (default true) / `--segment-target` to choose
which side to segment, and `--overwrite-segmentation` to re-segment blocks that
already carry an overlay. Each segment is **trimmed of leading/trailing
whitespace by default**, so a segment is the clean sentence and the
inter-sentence whitespace is left uncovered (keeping memory keys stable, regardless
of which engine ran); pass `--trim-leading-whitespace=false` /
`--trim-trailing-whitespace=false` to keep the raw surrounding whitespace.
`kapi stats` reports segment totals without changing the content; for per-block
segment detail, use `kapi inspect`. For every flag, see the
[command reference](/commands).

## In a flow and a recipe

Segmentation is a normal annotation stage. Put it ahead of translation so each
segment translates as its own unit and memory lookup keys on sentences:

```yaml
steps:
  - tool: segmentation       # mark sentence boundaries
    config:
      engine: srx
  - tool: recycle        # reuse prior sentence translations
  - tool: translate          # translate the remainder
```

In a [`kapi.yaml` project](/reference/project-file) the same stage lives in a named
flow, so every run segments consistently and the boundaries feed the
project's content memory.

## How segmentation feeds the rest of the pipeline

- **Content memory**: memory is a store of *segment* pairs, so segmentation is
  what makes prior sentence translations reusable. Segment, then
  [`recycle`](/framework/content-memory) matches **sentence by sentence**:
  when a block carries a multi-segment overlay, each sentence is looked up
  against memory and the block target is assembled from the per-segment matches,
  so a paragraph whose sentences were translated before is leveraged even though
  the paragraph as a whole was never a single entry.
- **AI translation**: translating per segment keeps each unit small and
  self-contained, which improves consistency and lets partial matches and
  full-segment matches coexist in one block.
- **Checks**: length, consistency, and other [checks](/framework/checks)
  operate per segment when an overlay is present, so a finding points at the
  offending sentence rather than the whole block.

Segmentation and [terminology](/framework/terminology) are run-anchored overlays
that prepare a source for translation; see
[Content preparation](/framework/content-preparation) for how they compose into
one authoring pass.
