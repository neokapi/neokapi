---
sidebar_position: 0
title: Implementation Notes Overview
slug: index
description: Index of Implementation Notes for the neokapi framework — SQL schemas, wire protocols, algorithm pseudocode, Go interface signatures, and other tactical reference material, grouped by the architecture series each note details.
keywords: [implementation notes, neokapi, reference, SQL schema, wire protocol, algorithms, Go interfaces]
---

# ▒ Îḿþļéḿéñţàţîöñ Ñöţéš — ñéöķàþî Ƒŕàḿéŵöŕķ ▒

▒ Îḿþļéḿéñţàţîöñ ñöţéš çöñţàîñ ţàçţîçàļ đéţàîļš ƒöŕ ţĥé ñéöķàþî ƒŕàḿéŵöŕķ
(Àþàçĥé-2.0): ŠǪĻ šçĥéḿàš, ŵîŕé þŕöţöçöļš, àļĝöŕîţĥḿ þšéüđöçöđé, Ĝö îñţéŕƒàçé
šîĝñàţüŕéš, àñđ öţĥéŕ ŕéƒéŕéñçé ḿàţéŕîàļ. Ţĥéý éẋîšţ àļöñĝšîđé ţĥé
[Àŕçĥîţéçţüŕé Đéçîšîöñš](/contribute/architecture/index), ŵĥîçĥ đéšçŕîƃé ţĥé
đéšîĝñ çĥöîçéš; ñöţéš đéšçŕîƃé ţĥé îḿþļéḿéñţàţîöñ. ▒

▒ Ţĥé ñöţéš àŕé ĝŕöüþéđ îñţö ţĥé šàḿé šéŕîéš àš ţĥé đéçîšîöñš — **Ƒ**
ƒöüñđàţîöñš, **É** éñĝîñé, **Ç** çöñţéẋţ, **Š** šüŕƒàçéš, **Ḿ** ḿüļţîļîñĝüàļ —
šö à ñöţé šîţš üñđéŕ ţĥé ĥéàđîñĝ öƒ ţĥé ÀĐ îţ đéţàîļš. À ƒîñàļ ĝŕöüþ ĥöļđš ţĥé
ñöţéš ţĥàţ đéšçŕîƃé ţĥîš ŕéþöšîţöŕý'š öŵñ îñƒŕàšţŕüçţüŕé ŕàţĥéŕ ţĥàñ à
šüƃšýšţéḿ öƒ ţĥé éñĝîñé. ▒

## ▒ Ƒ — Ƒöüñđàţîöñš ▒

| Note                                                                        | Parent AD                                                                                                                                 | Content                                                                                                                                        |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| [Content-Model Parity Across Wire Projections](foundations/content-parity.md) | [F-04](/contribute/architecture/foundations/f-04-wire-schema) · [F-02](/contribute/architecture/foundations/f-02-content-model)              | The lossless model↔proto↔store round-trip invariant, what must round-trip, and the extend-without-breaking checklist gated by a conformance test |

## ▒ É — Éñĝîñé ▒

| Note                                                              | Parent AD                                                                                                                  | Content                                                                                                                                          |
| ------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Implementing Formats](engine/implementing-formats.md)             | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | Step-by-step guide for new format readers and writers                                                                                              |
| [Skeleton Store](engine/skeleton-store.md)                         | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | SkeletonStore binary format, streaming HTML support, sub-skeleton                                                                                  |
| [Streaming Tree Formats](engine/streaming-tree-formats.md)         | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | How the whole-document readers reach bounded memory through an ancestor-only walk, and which formats stay buffered                                 |
| [Content-Fidelity Surfacing](engine/content-fidelity.md)           | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | Surfacing non-translatable context: the inverted toggle, channels, parity force-off                                                                |
| [Flow Steps Format](engine/flow-steps-format.md)                   | [E-04](/contribute/architecture/engine/e-04-flows-and-io-binding) · [E-03](/contribute/architecture/engine/e-03-tool-system)  | YAML step list, fan-out, script steps                                                                                                              |
| [Session-Scoped Tool Authoring](engine/session-tool-authoring.md)  | [E-03](/contribute/architecture/engine/e-03-tool-system)                                                                      | Guide for writing tools against BlockStore                                                                                                         |
| [Tool & Data Model Rationale](engine/tool-data-model-redesign.md)  | [E-03](/contribute/architecture/engine/e-03-tool-system) · [F-02](/contribute/architecture/foundations/f-02-content-model)    | Why stand-off overlays + annotations, a typed consumes/produces IO contract, a uniform unit iterator, and typed source/sink bindings                |
| [Plugin Model](engine/plugin-model.md)                             | [E-05](/contribute/architecture/engine/e-05-plugin-system)                                                                    | In-process registry contract for plugin binaries                                                                                                   |
| [Plugin protocol v1](engine/plugin-protocol-v1.md)                 | [E-05](/contribute/architecture/engine/e-05-plugin-system)                                                                    | The versioned plugin contract: manifest rules, the three transports, the Mode-C gRPC surface and wire format, and the conformance suite             |

## ▒ Ç — Çöñţéẋţ ▒

| Note                                                                     | Parent AD                                                        | Content                                                                                        |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| [kapi.yaml Project File](context/kapi-project-file.md)                    | [C-01](/contribute/architecture/context/c-01-project-model)        | `kapi.yaml` recipe schema and examples                                                           |
| [Terminology Data Model](context/terminology-data-model.md)               | [C-08](/contribute/architecture/context/c-08-terms)                | Go structs, Terminology interface                                                                |
| [Content Memory Matching Algorithm](context/memory-matching-algorithm.md) | [C-09](/contribute/architecture/context/c-09-content-memory)       | Tiered matching, TMX mapping                                                                     |

## ▒ Š — Šüŕƒàçéš ▒

| Note                                                    | Parent AD                                                            | Content                                                                                            |
| --------------------------------------------------------- | ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| [CLI Conventions](surfaces/cli-conventions.md)           | [S-01](/contribute/architecture/surfaces/s-01-kapi-cli)                | Input/output/exit-code/project contracts, per-command surface table                                  |
| [MCP Tools Reference](surfaces/mcp-tools-reference.md)   | [S-03](/contribute/architecture/surfaces/s-03-agent-surfaces)          | Where the MCP tool handlers live, how a tool reaches the server, and what shape its result takes     |
| [WASM Engine ABI](surfaces/wasm-engine-abi.md)           | [S-01](/contribute/architecture/surfaces/s-01-kapi-cli)                | The JS contract the browser build of the CLI exposes to `@neokapi/engine`, and its reverse bridges   |

## ▒ Ḿ — Ḿüļţîļîñĝüàļ ▒

| Note                                                        | Parent AD                                                                     | Content                                                                                    |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| [Multimodal Content](multilingual/multimodal-content.md)     | [M-03](/contribute/architecture/multilingual/m-03-multimodal-content)           | The two axes of adaptation, and the timing/geometry/recognition annotations that carry them  |
| [OMML Math Conversion](multilingual/omml-math.md)            | [M-04](/contribute/architecture/multilingual/m-04-math-and-equations)           | core/math Exp AST, OMML reader, nor-splice algorithm, coverage ledger                        |

## ▒ Ŕéþö îñƒŕàšţŕüçţüŕé ▒

▒ Ţĥéšé ţŵö đéšçŕîƃé ĥöŵ ţĥîš ŕéþöšîţöŕý ƃüîļđš àñđ þŕéšéñţš îţšéļƒ ŕàţĥéŕ ţĥàñ
ĥöŵ ţĥé éñĝîñé ŵöŕķš, šö ţĥéý àñšŵéŕ ţö ñö àŕçĥîţéçţüŕé đéçîšîöñ. ▒

| Note                                        | Content                                                                                        |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| [CDN asset offloading](repo/cdn-assets.md)   | Why the large immutable docs assets are served from S3 + CloudFront, and how they are published  |
| [Markdown in the UI](repo/markdown-in-ui.md) | Which metadata fields carry markdown, and the shared `Markdown` typeset primitive that renders them |
