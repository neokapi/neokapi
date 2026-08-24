---
sidebar_position: 1
title: Architecture
description: An overview of the neokapi framework architecture — the streaming pipeline, content model, format readers and writers, composable tools, and the multi-module Go structure.
keywords: [neokapi, architecture, streaming pipeline, content model, content engine, multilingual content, go modules]
---

import { ArchitectureDiagram } from "@neokapi/docs-shared";

# ▒ ñéöķàþî: Àŕçĥîţéçţüŕé ▒

▒ ñéöķàþî îš àñ öþéñ-šöüŕçé, ƒöŕḿàţ-àŵàŕé **çöñţéñţ éñĝîñé** ƃüîļţ îñ Ĝö. Îţ þàŕšéš
àñý ƒöŕḿàţ îñţö öñé üñîƒîéđ çöñţéñţ ḿöđéļ, éđîţš ţĥé çöñţéñţ îñšîđé îţ, çĥéçķš îţ,
àñđ ŵŕîţéš îţ ƃàçķ — ƃýţé-ƒöŕ-ƃýţé. Ĝéţ ýöüŕ çöñţéñţ ŕîĝĥţ — éđîţ îţ, çĥéçķ îţ,
ķééþ îţ öñ ƃŕàñđ — àñđ ţĥé šàḿé éñĝîñé ḿàķéš îţ ŵöŕķ îñ éṽéŕý ļàñĝüàĝé. Îţ àļšö
šéŕṽéš ÀÎ îñĝéšţîöñ àñđ þŕöĝŕàḿḿàţîç éđîţîñĝ. Îţ þŕöṽîđéš ƒöŕḿàţ-àŵàŕé đöçüḿéñţ þàŕšîñĝ,
çöḿþöšàƃļé þŕöçéššîñĝ ţööļš, àñđ à çöñçüŕŕéñţ šţŕéàḿîñĝ þîþéļîñé. Ţĥé [`ķàþî` ÇĻÎ
àñđ đéšķţöþ àþþ](/kapi/overview) àñđ [ñéöķàþî-î18ñ](/react/introduction) àŕé
šüŕƒàçéš ƃüîļţ öñ ţöþ öƒ ţĥîš éñĝîñé — ƃüţ ţĥé çöñţéñţ ḿöđéļ, ƒöŕḿàţ ŕéàđéŕš àñđ
ŵŕîţéŕš, ţööļš, àñđ þîþéļîñé àŕé éǫüàļļý à Ĝö ļîƃŕàŕý ýöü çàñ îḿþöŕţ àñđ đŕîṽé
đîŕéçţļý. Îƒ ýöü ŵàñţ ţö šţàŕţ ŵîţĥ ŕüññîñĝ çöđé, ĵüḿþ ţö ţĥé
[Ĝö ǫüîçķšţàŕţ](/framework/go-quickstart); ƒöŕ ţĥé ŕéàšöñîñĝ ƃéĥîñđ éàçĥ ḿàĵöŕ
đéšîĝñ çĥöîçé, šéé ţĥé [Àŕçĥîţéçţüŕé Đéçîšîöñš](/contribute/architecture/foundations/f-01-framework-and-modules). ▒

## ▒ Þŕöçéššîñĝ Þîþéļîñé ▒

<ArchitectureDiagram />

▒ Ţĥé éđĝéš àŕé ţĥé ƒļöŵ'š **šöüŕçé** àñđ **šîñķ** — ƃîñđîñĝš ţĥàţ đéçîđé ŵĥéŕé
çöñţéñţ éñţéŕš àñđ ļéàṽéš. Ţĥé đéƒàüļţ, šĥöŵñ àƃöṽé, îš ţĥé **ƒîļé ƃîñđîñĝ**: à
[ŕéàđéŕ](/framework/formats) ţüŕñš šöüŕçé ƒîļéš öƒ àñý ƒöŕḿàţ îñţö à šţŕéàḿ öƒ
[Þàŕţš](/framework/content-model) àñđ à [ŵŕîţéŕ](/framework/formats) ţüŕñš ţĥé
šţŕéàḿ ƃàçķ îñţö ƒîļéš, ƃýţé-ƒöŕ-ƃýţé. Ţĥé šàḿé ƒļöŵ çàñ îñšţéàđ ƃîñđ ţö ţĥé þŕöĵéçţ
šţöŕé, à `.ķþž` ŵöŕķšþàçé, öŕ àñ îñţéŕçĥàñĝé ƒîļé — ŵîţĥ ñö ŕéàđéŕ öŕ ŵŕîţéŕ
([ƒļöŵš: šöüŕçé àñđ šîñķ](/framework/flows#source-and-sink-the-flows-ends)).
Ƃéţŵééñ ţĥé éđĝéš ŕüñš à [ƒļöŵ](/framework/flows): à šéŕîàļ çĥàîñ öƒ
[ţööļš](/framework/tools) çöññéçţéđ ƃý ƃüƒƒéŕéđ çĥàññéļš öƒ Þàŕţš. Ţĥé ţööļš đîṽîđé ƃý çàþàƃîļîţý — **àññöţàţöŕš** àţţàçĥ šţàñđ-öƒƒ
[öṽéŕļàýš àñđ àññöţàţîöñš](/framework/content-model#two-ways-to-annotate-a-block)
(šéĝḿéñţàţîöñ, ţéŕḿîñöļöĝý, éñţîţîéš, ǪÀ ƒîñđîñĝš, àñàļýšîš ŕéšüļţš),
**ţŕàñšļàţöŕš** ƒîļļ îñ ţàŕĝéţš, àñđ **ǪÀ** ţööļš çĥéçķ àñđ éñƒöŕçé — ŵĥîļé
[çöñţéñţ ḿéḿöŕý](/framework/content-memory) àñđ ţĥé
[ţéŕḿš šţöŕé](/framework/terminology) ƒééđ ţĥé ŕéļéṽàñţ šţàĝéš. ▒

▒ Çöñçüŕŕéñçý ŕüñš àţ ţĥŕéé ļéṽéļš àţ öñçé: éàçĥ šţàĝé îš îţš öŵñ ĝöŕöüţîñé ĵöîñéđ
ƃý çĥàññéļš ŵîţĥ àüţöḿàţîç ƃàçķþŕéššüŕé; à ƃļöçķ-ĥàñđļîñĝ šţàĝé šüçĥ àš
ţŕàñšļàţîöñ çàñ **ƒàñ öüţ** àçŕöšš Ñ ĝöŕöüţîñéš ŵîţĥ àñ öŕđéŕéđ ƒàñ-îñ; àñđ ţĥé
éẋéçüţöŕ ŕüñš ḿàñý đöçüḿéñţš îñ þàŕàļļéļ, ƃöüñđéđ ƃý `ḾàẋÇöñçüŕŕéñçý`. Çöñţéẋţ
çàñçéļļàţîöñ þŕöþàĝàţéš ţö éṽéŕý šţàĝé. Ŕéàđéŕš, ŵŕîţéŕš, àñđ ţööļš çàñ ƃé
šüþþļîéđ ƃý [þļüĝîñš](/contribute/implementation/engine/plugin-model) — ţĥé
[`ķàþî-šàţ`](/contribute/architecture/multilingual/m-02-segmentation) šéĝḿéñţéŕ, ţĥé
[`ķàþî-þđƒîüḿ`](/contribute/architecture/engine/e-08-document-structure-tiers) ÞĐƑ
ŕéàđéŕ, öŕ àñý ŕéḿöţé þļüĝîñ — đîšþàţçĥéđ àš šüƃþŕöçéššéš öṽéŕ ĝŔÞÇ. Šéé
[Ƒ-01](/contribute/architecture/foundations/f-01-framework-and-modules) àñđ
[É-01](/contribute/architecture/engine/e-01-processing-engine). ▒

## ▒ Þàçķàĝé Ļàýöüţ ▒

```
neokapi/
├── go.mod                           # module github.com/neokapi/neokapi
├── go.work                          # coordinates the framework + CLI + app modules
│
├── core/                            # Platform-agnostic framework packages
│   ├── model/                       # Part, Block, Layer, Run, Target, Overlay, Data, Media
│   ├── format/                      # DataFormatReader/Writer interfaces, detection
│   ├── tool/                        # Tool interface, BaseTool dispatch
│   ├── flow/                        # Executor, Builder, FlowDefinition
│   ├── registry/                    # FormatRegistry, ToolRegistry
│   ├── encoding/                    # Text encoding utilities
│   ├── locale/                      # BCP-47 locale handling
│   ├── editor/                      # Block index serialization and preview generation
│   ├── version/                     # Build version info
│   ├── formats/                     # Built-in format implementations
│   │   └── …                        # one package each (reader.go, writer.go, config.go)
│   ├── ai/                          # AI pipeline tools, NER, prompt assembly
│   ├── mt/                          # Machine-translation pipeline tools
│   ├── profile/                     # Voice profiles, scoring, starter packs
│   ├── tools/                       # Utility tools (wordcount, pseudo, segmentation, …)
│   ├── storage/                     # Shared SQLite infrastructure (Open, Migrate)
│   ├── project/                     # kapi.yaml recipe format (Load, Save, Validate)
│   ├── plugin/                      # Plugin system (gRPC, loader, bridge, registry)
│   └── internal/testutil/           # Shared test helpers
│
├── memory/                          # Content memory (interface, in-memory, SQLite)
├── terms/                           # Terminology (interface, in-memory, SQLite)
├── providers/
│   ├── ai/                          # package aiprovider — LLM backends
│   └── mt/                          # package mtprovider — MT backends
│
├── host/                            # Cobra-free runtime + services (module: …/host)
├── cli/                             # Thin Cobra shell over host (module: …/cli)
├── kapi/                            # Kapi standalone CLI (module: …/kapi)
├── apps/kapi-desktop/               # Kapi Desktop (Wails v3; module: …/kapi-desktop)
├── packages/
│   ├── ui/                          # @neokapi/ui-primitives — shared shadcn/ui primitives
│   └── flow-editor/                 # @neokapi/flow-editor — shared React flow editor
└── docs/                            # Repo internals (format ops, testing, runbooks)
```

▒ Ţĥé ƒŕàḿéŵöŕķ ḿöđüļé (ŕéþö ŕööţ) šţàýš þļàţƒöŕḿ-àĝñöšţîç. `ḿéḿöŕý/`,
`ţéŕḿš/`, àñđ `þŕöṽîđéŕš/` àŕé ţöþ-ļéṽéļ ƒŕàḿéŵöŕķ þàçķàĝéš — ñöţ ñéšţéđ
üñđéŕ `çöŕé/`. Ƒŕöñţ-éñđš šüçĥ àš ţĥé ÇĻÎ àñđ ţĥé đéšķţöþ àþþ, àñđ àñý öţĥéŕ
çöñšüḿéŕ, àţţàçĥ ţĥŕöüĝĥ ţĥé þļüĝîñ àñđ éẋţéñšîöñ ŕéĝîšţŕîéš ŕàţĥéŕ ţĥàñ ƃý
đîŕéçţ îḿþöŕţš, šö ţĥé ƒŕàḿéŵöŕķ ñéṽéŕ đéþéñđš öñ à þàŕţîçüļàŕ þļàţƒöŕḿ. ▒

## ▒ Ţĥé ƒŕàḿéŵöŕķ çöñçéþţš ▒

▒ Ţö šéé ţĥéšé çöñçéþţš ŵöŕķîñĝ ţöĝéţĥéŕ îñ à ƒéŵ ļîñéš öƒ Ĝö — ŕéĝîšţéŕ ţĥé
ƒöŕḿàţš, ŕéàđ à ƒîļé îñţö ţĥé çöñţéñţ ḿöđéļ, ŕüñ à ţööļ, àñđ ŵŕîţé ţĥé ŕéšüļţ —
šţàŕţ ŵîţĥ ţĥé [Ĝö ǫüîçķšţàŕţ](/framework/go-quickstart). Ţĥé ƒŕàḿéŵöŕķ ŕéšţš öñ
à ƒéŵ çöñçéþţš, éàçĥ ŵîţĥ îţš öŵñ þàĝé: ▒

- ▒ **[Çöñţéñţ Ḿöđéļ](/framework/content-model)** — ţĥé ƒöŕḿàţ-îñđéþéñđéñţ
  ŕéþŕéšéñţàţîöñ. À đöçüḿéñţ ƃéçöḿéš à šţŕéàḿ öƒ `Þàŕţ`š çàŕŕýîñĝ ļàýéŕš, ƃļöçķš,
  ƒŕàĝḿéñţš, šþàñš, đàţà, àñđ ḿéđîà. Éḿƃéđđéđ çöñţéñţ (ĤŢḾĻ îñšîđé ĴŠÖÑ, ÇĐÀŢÀ îñ
  ẊḾĻ) îš ḿöđéļéđ àš ñéšţéđ ļàýéŕš, éàçĥ ŵîţĥ îţš öŵñ ƒöŕḿàţ. ▒
- ▒ **[Ƒöŕḿàţš](/framework/formats)** — þàîŕéđ ŕéàđéŕš àñđ ŵŕîţéŕš ţĥàţ þŕöđüçé àñđ
  çöñšüḿé ţĥé çöñţéñţ ḿöđéļ. ▒
- ▒ **[Ţööļš](/framework/tools)** — ţĥé þŕöçéššîñĝ üñîţš. Éàçĥ ŕéàđš Þàŕţš ƒŕöḿ à
  çĥàññéļ, ţŕàñšƒöŕḿš ţĥéḿ, àñđ ŵŕîţéš ţĥéḿ öüţ. ▒
- ▒ **[Ƒļöŵš](/framework/flows)** — ñàḿéđ, öŕđéŕéđ çöḿþöšîţîöñš öƒ ţööļš. ▒
- ▒ **[Þîþéļîñé](/framework/pipeline)** — ţĥé çöñçüŕŕéñţ éẋéçüţöŕ ţĥàţ ŕüñš à ƒļöŵ:
  ĝöŕöüţîñéš, ƃüƒƒéŕéđ çĥàññéļš, àñđ çöñţéẋţ-đŕîṽéñ çàñçéļļàţîöñ. ▒

▒ Ƒöŕ ţĥé çöñçŕéţé Ĝö îñţéŕƒàçéš àñđ ḿéţĥöđ šîĝñàţüŕéš ƃéĥîñđ ţĥéšé çöñçéþţš, šéé
ţĥé [Îñţéŕƒàçé Ŕéƒéŕéñçé](/contribute/interfaces). Ƒöŕ ţĥé đéšîĝñ ŕàţîöñàļé, šéé
ţĥé [Àŕçĥîţéçţüŕé Đéçîšîöñš](/contribute/architecture/foundations/f-01-framework-and-modules). ▒
