---
id: m-02-segmentation
sidebar_position: 2
title: "M-02: Segmentation"
description: "A segment is a run-anchored stand-off overlay over a block's existing runs, produced by an engine selected from a registry: rule-based SRX, the Unicode baseline, an LLM chunker, or a plugin-declared ML segmenter driven over the daemon transport."
keywords: [neokapi, architecture decision, segmentation, SRX, UAX-29, SaT, overlay, sentence boundary, segment engine]
---

import { StreamDiagram, PipelineDiagram } from "@neokapi/docs-shared";

# M-02: Segmentation

## Summary

Segmentation is a means to two ends: it is what makes prior translations
reusable and what lets translation and checks operate on a unit small enough to
be right about. In neokapi it is a **run-anchored stand-off overlay**. The
boundaries are recorded as spans over a block's existing runs, and the runs are
never rewritten.

Which boundaries those are is decided by a **segment engine** selected by name
from a registry that mirrors the model-provider registries
([E-07](../engine/e-07-model-providers.md)). The framework ships rule and
Unicode-baseline engines; the AI tools contribute a chunker; a plugin
contributes an ML segmenter declared in its manifest and driven over the plugin
daemon transport. An engine that is not linked into a given binary is absent
from the registry, and selecting it reports an actionable error rather than
failing a build.

## A segment is an overlay

A block's content is a flat `[]Run` per locale
([F-02](../foundations/f-02-content-model.md)). Every interpretation *of* that
content is a typed overlay layered over the runs. Segmentation is one such
overlay: an ordered, non-overlapping list of spans, each anchored by an
`Anchor`, a start and end run position, each a rune offset into the text run
at each end, half-open.

<StreamDiagram
  title="segmentation overlay (source runs unchanged)"
  items={[
    { kind: "Block source", detail: '"Dr. Smith arrived. He was late."', role: "block" },
    { kind: "Overlay", detail: 'type = "segmentation", layer = primary', role: "meta", note: "anchored to run ranges" },
    { kind: "Span", detail: 's1 · "Dr. Smith arrived."', depth: 1, role: "layer" },
    { kind: "Span", detail: 's2 · "He was late."', depth: 1, role: "layer" },
  ]}
  ariaLabel="A block's runs with a segmentation overlay of two spans layered over them"
  caption="Dropping the overlay restores the unsegmented block exactly."
/>

Three properties follow from choosing an overlay over a structural split:

- **Reversible.** Removing the overlay restores the block. A structural split
  forces the choice at parse time and makes re-joining fiddly.
- **Multi-layer.** `Overlay.Layer` names a granularity, so several coexist over
  the same runs. The empty string is the **primary sentence** layer, the one
  bilingual formats project to and from; named layers (`llm-chunk`, `clause`)
  are additional on-demand interpretations.
- **Identity-preserving.** The block content hash is computed over the runs
  ([F-03](../foundations/f-03-identity.md)), which segmentation does not touch.
  A project can turn segmentation on or off between extractions without
  invalidating the memory, the check overlays, or a merge's join key
  ([M-01](m-01-bilingual-interop.md)).

Runs that no span covers are implicit inter-segment material. A span can also be
marked **ignorable** (non-content structural material such as inter-sentence
whitespace or a plural selector), in which case a bilingual round trip preserves
its target verbatim instead of translating it, while the span still occupies its
range so neighbouring positions stay aligned.

Segmentation is produced *after* any source transforms have settled the source,
so every boundary anchors to the canonical runs that translation and the content
memory will also see. When a later transform does rewrite the source, the
applier rebases the boundaries onto the new runs.

## The engine registry

An engine registers an `EngineDescriptor` under a short name. The descriptor is
self-describing: an identity and ordering for the engine selector, the engine's
own parameter schema (or nil when it takes none), and a builder that receives
the shared `BaseConfig` plus the subset of the config map that engine
understands.

That split is what keeps the umbrella `segmentation` tool free of
engine-specific knowledge. Shared concerns (how inline codes are masked before
boundary detection, how a break adjacent to a code resolves, a locale override)
live in `BaseConfig`. Everything else (an SRX ruleset path, an LLM provider, a
model name and threshold) belongs to the engine and travels through its own
schema, so an engine can evolve its parameters without touching the tool.

Registration comes in two flavours. `Register` panics on a duplicate, matching
the framework's other init-time registries. `RegisterIfAbsent` is the host's
idempotent path for plugin-provided engines, which are re-scanned whenever the
installed plugin set changes and must never clobber a built-in or panic on a
repeat scan.

Building an engine that no linked package registered returns
`ErrEngineUnavailable`, listing the names that are available.

## The engines

| Engine | Boundaries from | Requires | Layer |
| --- | --- | --- | --- |
| `srx` (default) | SRX 2.0 rules, over a Unicode base where ICU is linked | nothing (pure Go) | sentence |
| `uax29` | Unicode UAX-29 sentence rules | cgo + ICU natively; an ICU4X bridge in the browser | sentence |
| `intl` | the browser's `Intl.Segmenter` | a browser host (WASM builds only) | sentence |
| `llm` | a model asked to chunk semantically | a configured model provider | `llm-chunk` |
| plugin-declared | whatever the plugin implements | the plugin installed | sentence |

### SRX, the default

SRX (Segmentation Rules eXchange) is the standard rule format: an ordered list
of break and no-break rules, scoped by language. neokapi ships a pure-Go SRX 2.0
engine, so the default runs everywhere with no native dependency, including in
the browser.

SRX in practice is a **hybrid**. The default ruleset is overwhelmingly no-break
rules with only a handful of break rules, because it is written to be applied as
*exceptions* on top of a Unicode base rather than as a complete boundary
algorithm. The framework implements that directly through a `BaseBreaker`
seam: the ICU-backed engine registers a base breaker at init, and the SRX engine
asks the registry at runtime whether one is available.

- **With a base breaker** (every shipped native binary, and a browser page that
  has loaded the ICU4X bridge), `srx` loads the full ruleset and runs the base +
  exception hybrid: a break rule adds a boundary, a no-break rule suppresses
  one, and the first rule to decide at a position wins.
- **Without one** (a pure-Go build with no ICU, a browser page without the
  bridge), the same engine falls back to a reduced self-contained ruleset with
  explicit break rules. Lighter than the full set, but it still handles the
  common abbreviations, decimals and initials.

There is no engine choice to make here: `srx` picks the path the build supports.
The seam is resolved at runtime through the registry rather than by an import,
which is precisely what keeps the pure-Go SRX package free of any cgo or ICU
dependency.

An explicit ruleset path overrides the adaptive default in either mode. One file
serves both sides, because SRX rules are keyed by language: the same ruleset
supplies the source rules and, when target segmentation is on, the target rules.

### The Unicode baseline

`uax29` is the bare Unicode default sentence boundaries with no exception rules.
Natively it is ICU over cgo; in the browser the same name is registered by a
bridge to ICU4X running as a companion WebAssembly module, so `engine: uax29`
selects the same concept in both places. `intl` is the browser-only alternative
that calls the platform's own `Intl.Segmenter` and downloads nothing.

### The chunker

`llm` asks a configured model to chunk semantically and produces the
`llm-chunk` layer rather than the sentence layer, so a coarser interpretation
for long-form prose can sit beside the sentence segmentation instead of
replacing it.

## Selecting an engine

<PipelineDiagram
  stages={[
    { label: "segmentation tool", sub: "engine: <name>", role: "tool", note: "shared config: mask, trim, scope" },
    { label: "segment registry", sub: "Lookup / Build", role: "annotate", note: "descriptor + parameter schema" },
    {
      label: "engine",
      parallelLabel: "one of the registered engines",
      lanes: [
        { label: "srx", sub: "in-process" },
        { label: "uax29 / intl", sub: "in-process" },
        { label: "llm", sub: "provider call" },
        { label: "plugin", sub: "daemon RPC" },
      ],
      role: "annotate",
    },
    { label: "overlay", sub: "run-anchored spans", role: "io" },
  ]}
  channelLabel=""
  caption="Every engine writes the same overlay; they differ only in how they find boundaries."
/>

The `segmentation` tool (aliased `segment`) carries the shared configuration:
which engine to run, which overlay layer to write, whether to segment the source
side, the target side, or both, whether to overwrite an existing overlay, and
how boundaries treat isolated inline codes and surrounding whitespace. Segments
are trimmed of leading and trailing whitespace by default, so a segment is the
clean sentence and the inter-sentence whitespace is left uncovered, which keeps
memory keys stable regardless of which engine ran.

Selection happens at three altitudes, each narrower than the last:

- **A flow step** names the engine in its config, which is the normal case:
  segmentation is an ordinary annotation stage placed ahead of recycling and
  translation.
- **A project** pins per-tool defaults under `defaults.tools`, so a recipe can
  fix the engine and its parameters once for every flow in the project.
- **A single invocation** overrides both: `kapi exec segmentation <file>
  --engine srx --rules-path .kapi/rules.srx`.

Separately, `defaults.segmentation` in the recipe is the **extract-side toggle**
rather than an engine selector: `source` turns the opt-in segmentation overlay
on for `kapi extract`, and `srx` optionally points at a ruleset file
([M-01](m-01-bilingual-interop.md)).

For a quick file-free tweak the tool also accepts an inline list of break and
no-break regex pairs; an inline list overrides the engine selection. Beyond a
rule or two, a real SRX file is the portable form.

## A plugin-declared engine

A plugin declares its segmenters in `capabilities.segmenters` in its manifest:
a name, a display name and description for the engine selector, an ordering, and
a path to a JSON parameter schema relative to the plugin directory. The host
walks every daemon-transport plugin it discovered, registers each declared
engine into the segment registry with the schema loaded from that file, and
routes it to a generic bridge segmenter.

**There is no per-plugin code in the host.** The bridge flattens the runs under
the shared mask options, sends the masked text and the config parameters to the
plugin's `Segment` RPC, and projects the returned interior boundaries back to
run-anchored spans through the same flattening it used on the way out, exactly
what an in-process engine does. The engine name, the plugin route and the
parameters are all data captured at registration. Adding a segmenter plugin
requires a manifest entry and, optionally, a schema file.

Boundaries cross the wire as **interior rune offsets** into the exact text that
was sent: an offset `i` means a new sentence begins at `text[i]`, and the ends
are never emitted. Rune offsets rather than bytes, because that is the unit the
overlay projection works in and the one a non-Go implementation is least likely
to get wrong.

### The SaT segmenter

`kapi-sat` is the reference implementation: it runs the
[SaT / wtpsplit](https://github.com/segment-any-text/wtpsplit) *Segment any
Text* models, XLM-RoBERTa-based ONNX models that segment any language the
tokenizer covers without per-language rules. It is the right choice for text
rule engines handle poorly: languages without reliable sentence punctuation,
transcribed or user-generated text, mixed-script content.

Three requirements put it outside the portable binary, and they are the same
ones that put the Okapi bridge there
([E-05](../engine/e-05-plugin-system.md)):

- **A native ML stack.** Inference needs the ONNX Runtime shared library and a
  tokenizer linked through cgo. Linking either into `kapi` would force every
  install to carry the ONNX ABI, defeat pure-Go cross-compilation, and inflate
  the binary for a capability most invocations never use.
- **Large model assets.** The models run to hundreds of megabytes. That is the
  segmenter's runtime concern, not the CLI's.
- **A warm process.** Loading an ONNX session per block is prohibitively slow;
  the model must load once and stay resident across a run. The daemon transport
  already provides exactly that, a pooled, long-lived subprocess with an idle
  timeout, which is why the segmenter uses it rather than a bespoke protocol.

The plugin is its own Go module, isolated so its native dependencies never enter
another module's build graph, and it builds two ways from one source tree. The
**default build** links no native libraries and is safe on any machine: the
daemon still serves, the handshake and capability probing still answer, and a
segment request reports the build limitation instead of crashing. The **ONNX
build** links the tokenizer archive and loads the ONNX Runtime shared library at
runtime; that is the configuration shipped in release archives. Because the
inference algorithm (the block/recombine windowing, the half-precision
conversion, the rune mapping) is pure Go, its tests build and run with no
native dependency.

Models are **not bundled**. The manifest pins each model's files by URL and
SHA-256, and the host's model-asset machinery downloads them on first use into
a shared cache, so a model is fetched once and verified before it is used. The
ONNX Runtime shared library *is* shipped beside the binary and resolved from
there, so an installed plugin needs no environment configuration.

The manifest also sets `capabilities.selfcheck`, which advertises the standard
self-check that `kapi plugin doctor` runs: it constructs the engine and lists
the models it supports, so "installed but no working ONNX backend" is a
diagnosable state rather than a mystery at the first segment.

## Consequences

- The portable `kapi` binary stays pure-Go, small and cross-compilable. Every
  heavy engine is a separately built, separately installed plugin, and selecting
  one that is not installed is an actionable error rather than a build failure.
- Segmentation quality is a configuration choice at the point of use, not a
  property of the build: the same `engine:` selector spans a rule engine, the
  Unicode baseline, a model chunker and an ML segmenter.
- A first-segment call through a plugin pays a one-time model download.
  Integrators warm the model with an explicit segment run or surface the
  plugin's progress output.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): overlays, spans, and run ranges
- [F-03: Identity](../foundations/f-03-identity.md): why a segmentation toggle does not move a block's hash
- [E-03: The tool system](../engine/e-03-tool-system.md): the `segmentation` tool and its composed schema
- [E-05: The plugin system](../engine/e-05-plugin-system.md): manifest capabilities, the daemon transport, signed distribution
- [E-07: Model and translation providers](../engine/e-07-model-providers.md): the registry pattern the engine registry mirrors
- [M-01: Bilingual format interop](m-01-bilingual-interop.md): how spans project into a bilingual file and back
- [C-09: Content memory](../context/c-09-content-memory.md): per-segment lookup, the reason segmentation exists
- [Segmentation](/framework/segmentation): the configuration guide, with flags and worked recipes
