---
id: m-02-segmentation
sidebar_position: 2
title: "M-02: Segmentation"
description: "A segment is a run-anchored stand-off overlay over a block's existing runs, produced by an engine selected from a registry — rule-based SRX, the Unicode baseline, an LLM chunker, or a plugin-declared ML segmenter driven over the daemon transport."
keywords: [neokapi, architecture decision, segmentation, SRX, UAX-29, SaT, overlay, sentence boundary, segment engine]
---

import { StreamDiagram, PipelineDiagram } from "@neokapi/docs-shared";

# ▒ Ḿ-02: Šéĝḿéñţàţîöñ ▒

## ▒ Šüḿḿàŕý ▒

▒ Šéĝḿéñţàţîöñ îš à **ḿéàñš**, ñöţ àñ éñđ: îţ îš ŵĥàţ ḿàķéš þŕîöŕ ţŕàñšļàţîöñš
ŕéüšàƃļé àñđ ŵĥàţ ļéţš ţŕàñšļàţîöñ àñđ çĥéçķš öþéŕàţé öñ à üñîţ šḿàļļ éñöüĝĥ ţö
ƃé ŕîĝĥţ àƃöüţ. Îñ ñéöķàþî îţ îš à **ŕüñ-àñçĥöŕéđ šţàñđ-öƒƒ öṽéŕļàý** — ţĥé
ƃöüñđàŕîéš àŕé ŕéçöŕđéđ àš šþàñš öṽéŕ à ƃļöçķ'š éẋîšţîñĝ ŕüñš, àñđ ţĥé ŕüñš àŕé
ñéṽéŕ ŕéŵŕîţţéñ. ▒

▒ Ŵĥîçĥ ƃöüñđàŕîéš ţĥöšé àŕé îš đéçîđéđ ƃý à **šéĝḿéñţ éñĝîñé** šéļéçţéđ ƃý ñàḿé
ƒŕöḿ à ŕéĝîšţŕý ţĥàţ ḿîŕŕöŕš ţĥé ḿöđéļ-þŕöṽîđéŕ ŕéĝîšţŕîéš
([É-07](../engine/e-07-model-providers.md)). Ţĥé ƒŕàḿéŵöŕķ šĥîþš ŕüļé àñđ
Üñîçöđé-ƃàšéļîñé éñĝîñéš; ţĥé ÀÎ ţööļš çöñţŕîƃüţé à çĥüñķéŕ; à þļüĝîñ
çöñţŕîƃüţéš àñ ḾĻ šéĝḿéñţéŕ đéçļàŕéđ îñ îţš ḿàñîƒéšţ àñđ đŕîṽéñ öṽéŕ ţĥé þļüĝîñ
đàéḿöñ ţŕàñšþöŕţ. Àñ éñĝîñé ţĥàţ îš ñöţ ļîñķéđ îñţö à ĝîṽéñ ƃîñàŕý îš àƃšéñţ
ƒŕöḿ ţĥé ŕéĝîšţŕý, àñđ šéļéçţîñĝ îţ ŕéþöŕţš àñ àçţîöñàƃļé éŕŕöŕ ŕàţĥéŕ ţĥàñ
ƒàîļîñĝ à ƃüîļđ. ▒

## ▒ À šéĝḿéñţ îš àñ öṽéŕļàý ▒

▒ À ƃļöçķ'š çöñţéñţ îš à ƒļàţ `[]Ŕüñ` þéŕ ļöçàļé
([Ƒ-02](../foundations/f-02-content-model.md)). Éṽéŕý îñţéŕþŕéţàţîöñ *öƒ* ţĥàţ
çöñţéñţ îš à ţýþéđ öṽéŕļàý ļàýéŕéđ öṽéŕ ţĥé ŕüñš. Šéĝḿéñţàţîöñ îš öñé šüçĥ
öṽéŕļàý: àñ öŕđéŕéđ, ñöñ-öṽéŕļàþþîñĝ ļîšţ öƒ šþàñš, éàçĥ àñçĥöŕéđ ƃý à
`ŔüñŔàñĝé` — à šţàŕţ àñđ éñđ ŕüñ þöšîţîöñ þļüš à ŕüñé öƒƒšéţ îñţö ţĥé ţéẋţ ŕüñ
àţ éàçĥ éñđ, ĥàļƒ-öþéñ. ▒

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

▒ Ţĥŕéé þŕöþéŕţîéš ƒöļļöŵ ƒŕöḿ çĥööšîñĝ àñ öṽéŕļàý öṽéŕ à šţŕüçţüŕàļ šþļîţ: ▒

- ▒ **Ŕéṽéŕšîƃļé.** Ŕéḿöṽîñĝ ţĥé öṽéŕļàý ŕéšţöŕéš ţĥé ƃļöçķ. À šţŕüçţüŕàļ šþļîţ
  ƒöŕçéš ţĥé çĥöîçé àţ þàŕšé ţîḿé àñđ ḿàķéš ŕé-ĵöîñîñĝ ƒîđđļý. ▒
- ▒ **Ḿüļţî-ļàýéŕ.** `Öṽéŕļàý.Ļàýéŕ` ñàḿéš à ĝŕàñüļàŕîţý, šö šéṽéŕàļ çöéẋîšţ öṽéŕ
  ţĥé šàḿé ŕüñš. Ţĥé éḿþţý šţŕîñĝ îš ţĥé **þŕîḿàŕý šéñţéñçé** ļàýéŕ — ţĥé öñé
  ƃîļîñĝüàļ ƒöŕḿàţš þŕöĵéçţ ţö àñđ ƒŕöḿ; ñàḿéđ ļàýéŕš (`ļļḿ-çĥüñķ`, `çļàüšé`)
  àŕé àđđîţîöñàļ öñ-đéḿàñđ îñţéŕþŕéţàţîöñš. ▒
- ▒ **Îđéñţîţý-þŕéšéŕṽîñĝ.** Ţĥé ƃļöçķ çöñţéñţ ĥàšĥ îš çöḿþüţéđ öṽéŕ ţĥé ŕüñš
  ([Ƒ-03](../foundations/f-03-identity.md)), ŵĥîçĥ šéĝḿéñţàţîöñ đöéš ñöţ ţöüçĥ.
  À þŕöĵéçţ çàñ ţüŕñ šéĝḿéñţàţîöñ öñ öŕ öƒƒ ƃéţŵééñ éẋţŕàçţîöñš ŵîţĥöüţ
  îñṽàļîđàţîñĝ ţĥé ḿéḿöŕý, ţĥé ǪÀ öṽéŕļàýš, öŕ à ḿéŕĝé'š ĵöîñ ķéý
  ([Ḿ-01](m-01-bilingual-interop.md)). ▒

▒ Ŕüñš ţĥàţ ñö šþàñ çöṽéŕš àŕé îḿþļîçîţ îñţéŕ-šéĝḿéñţ ḿàţéŕîàļ. À šþàñ çàñ àļšö ƃé
ḿàŕķéđ **îĝñöŕàƃļé** — ñöñ-çöñţéñţ šţŕüçţüŕàļ ḿàţéŕîàļ šüçĥ àš îñţéŕ-šéñţéñçé
ŵĥîţéšþàçé öŕ à þļüŕàļ šéļéçţöŕ — îñ ŵĥîçĥ çàšé à ƃîļîñĝüàļ ŕöüñđ ţŕîþ þŕéšéŕṽéš
îţš ţàŕĝéţ ṽéŕƃàţîḿ îñšţéàđ öƒ ţŕàñšļàţîñĝ îţ, ŵĥîļé ţĥé šþàñ šţîļļ öççüþîéš îţš
ŕàñĝé šö ñéîĝĥƃöüŕîñĝ þöšîţîöñš šţàý àļîĝñéđ. ▒

▒ Šéĝḿéñţàţîöñ îš þŕöđüçéđ *àƒţéŕ* àñý šöüŕçé ţŕàñšƒöŕḿš ĥàṽé šéţţļéđ ţĥé šöüŕçé,
šö éṽéŕý ƃöüñđàŕý àñçĥöŕš ţö ţĥé çàñöñîçàļ ŕüñš ţĥàţ ţŕàñšļàţîöñ àñđ ţĥé çöñţéñţ
ḿéḿöŕý ŵîļļ àļšö šéé. Ŵĥéñ à ļàţéŕ ţŕàñšƒöŕḿ đöéš ŕéŵŕîţé ţĥé šöüŕçé, ţĥé
àþþļîéŕ ŕéƃàšéš ţĥé ƃöüñđàŕîéš öñţö ţĥé ñéŵ ŕüñš. ▒

## ▒ Ţĥé éñĝîñé ŕéĝîšţŕý ▒

▒ Àñ éñĝîñé ŕéĝîšţéŕš àñ `ÉñĝîñéĐéšçŕîþţöŕ` üñđéŕ à šĥöŕţ ñàḿé. Ţĥé đéšçŕîþţöŕ îš
šéļƒ-đéšçŕîƃîñĝ: àñ îđéñţîţý àñđ öŕđéŕîñĝ ƒöŕ ţĥé éñĝîñé šéļéçţöŕ, ţĥé éñĝîñé'š
öŵñ þàŕàḿéţéŕ šçĥéḿà (öŕ ñîļ ŵĥéñ îţ ţàķéš ñöñé), àñđ à ƃüîļđéŕ ţĥàţ ŕéçéîṽéš
ţĥé šĥàŕéđ `ƂàšéÇöñƒîĝ` þļüš ţĥé šüƃšéţ öƒ ţĥé çöñƒîĝ ḿàþ ţĥàţ éñĝîñé
üñđéŕšţàñđš. ▒

▒ Ţĥàţ šþļîţ îš ŵĥàţ ķééþš ţĥé üḿƃŕéļļà `šéĝḿéñţàţîöñ` ţööļ ƒŕéé öƒ
éñĝîñé-šþéçîƒîç ķñöŵļéđĝé. Šĥàŕéđ çöñçéŕñš — ĥöŵ îñļîñé çöđéš àŕé ḿàšķéđ ƃéƒöŕé
ƃöüñđàŕý đéţéçţîöñ, ĥöŵ à ƃŕéàķ àđĵàçéñţ ţö à çöđé ŕéšöļṽéš, à ļöçàļé öṽéŕŕîđé —
ļîṽé îñ `ƂàšéÇöñƒîĝ`. Éṽéŕýţĥîñĝ éļšé (àñ ŠŔẊ ŕüļéšéţ þàţĥ, àñ ĻĻḾ þŕöṽîđéŕ, à
ḿöđéļ ñàḿé àñđ ţĥŕéšĥöļđ) ƃéļöñĝš ţö ţĥé éñĝîñé àñđ ţŕàṽéļš ţĥŕöüĝĥ îţš öŵñ
šçĥéḿà, šö àñ éñĝîñé çàñ éṽöļṽé îţš þàŕàḿéţéŕš ŵîţĥöüţ ţöüçĥîñĝ ţĥé ţööļ. ▒

▒ Ŕéĝîšţŕàţîöñ çöḿéš îñ ţŵö ƒļàṽöüŕš. `Ŕéĝîšţéŕ` þàñîçš öñ à đüþļîçàţé, ḿàţçĥîñĝ
ţĥé ƒŕàḿéŵöŕķ'š öţĥéŕ îñîţ-ţîḿé ŕéĝîšţŕîéš. `ŔéĝîšţéŕÎƒÀƃšéñţ` îš ţĥé ĥöšţ'š
îđéḿþöţéñţ þàţĥ ƒöŕ þļüĝîñ-þŕöṽîđéđ éñĝîñéš, ŵĥîçĥ àŕé ŕé-šçàññéđ ŵĥéñéṽéŕ ţĥé
îñšţàļļéđ þļüĝîñ šéţ çĥàñĝéš àñđ ḿüšţ ñéṽéŕ çļöƃƃéŕ à ƃüîļţ-îñ öŕ þàñîç öñ à
ŕéþéàţ šçàñ. ▒

▒ Ƃüîļđîñĝ àñ éñĝîñé ţĥàţ ñö ļîñķéđ þàçķàĝé ŕéĝîšţéŕéđ ŕéţüŕñš
`ÉŕŕÉñĝîñéÜñàṽàîļàƃļé`, ļîšţîñĝ ţĥé ñàḿéš ţĥàţ àŕé àṽàîļàƃļé. ▒

## ▒ Ţĥé éñĝîñéš ▒

| Engine | Boundaries from | Requires | Layer |
| --- | --- | --- | --- |
| `srx` (default) | SRX 2.0 rules, over a Unicode base where ICU is linked | nothing (pure Go) | sentence |
| `uax29` | Unicode UAX-29 sentence rules | cgo + ICU natively; an ICU4X bridge in the browser | sentence |
| `intl` | the browser's `Intl.Segmenter` | a browser host (WASM builds only) | sentence |
| `llm` | a model asked to chunk semantically | a configured model provider | `llm-chunk` |
| plugin-declared | whatever the plugin implements | the plugin installed | sentence |

### ▒ ŠŔẊ, ţĥé đéƒàüļţ ▒

▒ ŠŔẊ (Šéĝḿéñţàţîöñ Ŕüļéš éẊçĥàñĝé) îš ţĥé šţàñđàŕđ ŕüļé ƒöŕḿàţ: àñ öŕđéŕéđ ļîšţ
öƒ ƃŕéàķ àñđ ñö-ƃŕéàķ ŕüļéš, šçöþéđ ƃý ļàñĝüàĝé. ñéöķàþî šĥîþš à þüŕé-Ĝö ŠŔẊ 2.0
éñĝîñé, šö ţĥé đéƒàüļţ ŕüñš éṽéŕýŵĥéŕé ŵîţĥ ñö ñàţîṽé đéþéñđéñçý — îñçļüđîñĝ îñ
ţĥé ƃŕöŵšéŕ. ▒

▒ ŠŔẊ îñ þŕàçţîçé îš à **ĥýƃŕîđ**. Ţĥé đéƒàüļţ ŕüļéšéţ îš öṽéŕŵĥéļḿîñĝļý ñö-ƃŕéàķ
ŕüļéš ŵîţĥ öñļý à ĥàñđƒüļ öƒ ƃŕéàķ ŕüļéš, ƃéçàüšé îţ îš ŵŕîţţéñ ţö ƃé àþþļîéđ àš
*éẋçéþţîöñš* öñ ţöþ öƒ à Üñîçöđé ƃàšé ŕàţĥéŕ ţĥàñ àš à çöḿþļéţé ƃöüñđàŕý
àļĝöŕîţĥḿ. Ţĥé ƒŕàḿéŵöŕķ îḿþļéḿéñţš ţĥàţ đîŕéçţļý ţĥŕöüĝĥ à `ƂàšéƂŕéàķéŕ`
šéàḿ: ţĥé ÎÇÜ-ƃàçķéđ éñĝîñé ŕéĝîšţéŕš à ƃàšé ƃŕéàķéŕ àţ îñîţ, àñđ ţĥé ŠŔẊ éñĝîñé
àšķš ţĥé ŕéĝîšţŕý àţ ŕüñţîḿé ŵĥéţĥéŕ öñé îš àṽàîļàƃļé. ▒

- ▒ **Ŵîţĥ à ƃàšé ƃŕéàķéŕ** (éṽéŕý šĥîþþéđ ñàţîṽé ƃîñàŕý, àñđ à ƃŕöŵšéŕ þàĝé ţĥàţ
  ĥàš ļöàđéđ ţĥé ÎÇÜ4Ẋ ƃŕîđĝé), `šŕẋ` ļöàđš ţĥé ƒüļļ ŕüļéšéţ àñđ ŕüñš ţĥé ƃàšé +
  éẋçéþţîöñ ĥýƃŕîđ — à ƃŕéàķ ŕüļé àđđš à ƃöüñđàŕý, à ñö-ƃŕéàķ ŕüļé šüþþŕéššéš
  öñé, àñđ ţĥé ƒîŕšţ ŕüļé ţö đéçîđé àţ à þöšîţîöñ ŵîñš. ▒
- ▒ **Ŵîţĥöüţ öñé** (à þüŕé-Ĝö ƃüîļđ ŵîţĥ ñö ÎÇÜ, à ƃŕöŵšéŕ þàĝé ŵîţĥöüţ ţĥé
  ƃŕîđĝé), ţĥé šàḿé éñĝîñé ƒàļļš ƃàçķ ţö à ŕéđüçéđ šéļƒ-çöñţàîñéđ ŕüļéšéţ ŵîţĥ
  éẋþļîçîţ ƃŕéàķ ŕüļéš. Ļîĝĥţéŕ ţĥàñ ţĥé ƒüļļ šéţ, ƃüţ îţ šţîļļ ĥàñđļéš ţĥé
  çöḿḿöñ àƃƃŕéṽîàţîöñš, đéçîḿàļš àñđ îñîţîàļš. ▒

▒ Ţĥéŕé îš ñö éñĝîñé çĥöîçé ţö ḿàķé ĥéŕé: `šŕẋ` þîçķš ţĥé þàţĥ ţĥé ƃüîļđ šüþþöŕţš.
Ţĥé šéàḿ îš ŕéšöļṽéđ àţ ŕüñţîḿé ţĥŕöüĝĥ ţĥé ŕéĝîšţŕý ŕàţĥéŕ ţĥàñ ƃý àñ îḿþöŕţ,
ŵĥîçĥ îš þŕéçîšéļý ŵĥàţ ķééþš ţĥé þüŕé-Ĝö ŠŔẊ þàçķàĝé ƒŕéé öƒ àñý çĝö öŕ ÎÇÜ
đéþéñđéñçý. ▒

▒ Àñ éẋþļîçîţ ŕüļéšéţ þàţĥ öṽéŕŕîđéš ţĥé àđàþţîṽé đéƒàüļţ îñ éîţĥéŕ ḿöđé. Öñé ƒîļé
šéŕṽéš ƃöţĥ šîđéš, ƃéçàüšé ŠŔẊ ŕüļéš àŕé ķéýéđ ƃý ļàñĝüàĝé: ţĥé šàḿé ŕüļéšéţ
šüþþļîéš ţĥé šöüŕçé ŕüļéš àñđ, ŵĥéñ ţàŕĝéţ šéĝḿéñţàţîöñ îš öñ, ţĥé ţàŕĝéţ ŕüļéš. ▒

### ▒ Ţĥé Üñîçöđé ƃàšéļîñé ▒

▒ `üàẋ29` îš ţĥé ƃàŕé Üñîçöđé đéƒàüļţ šéñţéñçé ƃöüñđàŕîéš ŵîţĥ ñö éẋçéþţîöñ ŕüļéš.
Ñàţîṽéļý îţ îš ÎÇÜ öṽéŕ çĝö; îñ ţĥé ƃŕöŵšéŕ ţĥé šàḿé ñàḿé îš ŕéĝîšţéŕéđ ƃý à
ƃŕîđĝé ţö ÎÇÜ4Ẋ ŕüññîñĝ àš à çöḿþàñîöñ ŴéƃÀššéḿƃļý ḿöđüļé, šö `éñĝîñé: üàẋ29`
šéļéçţš ţĥé šàḿé çöñçéþţ îñ ƃöţĥ þļàçéš. `îñţļ` îš ţĥé ƃŕöŵšéŕ-öñļý àļţéŕñàţîṽé
ţĥàţ çàļļš ţĥé þļàţƒöŕḿ'š öŵñ `Îñţļ.Šéĝḿéñţéŕ` àñđ đöŵñļöàđš ñöţĥîñĝ. ▒

### ▒ Ţĥé çĥüñķéŕ ▒

▒ `ļļḿ` àšķš à çöñƒîĝüŕéđ ḿöđéļ ţö çĥüñķ šéḿàñţîçàļļý àñđ þŕöđüçéš ţĥé
`ļļḿ-çĥüñķ` ļàýéŕ ŕàţĥéŕ ţĥàñ ţĥé šéñţéñçé ļàýéŕ, šö à çöàŕšéŕ îñţéŕþŕéţàţîöñ
ƒöŕ ļöñĝ-ƒöŕḿ þŕöšé çàñ šîţ ƃéšîđé ţĥé šéñţéñçé šéĝḿéñţàţîöñ îñšţéàđ öƒ
ŕéþļàçîñĝ îţ. ▒

## ▒ Šéļéçţîñĝ àñ éñĝîñé ▒

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
clean sentence and the inter-sentence whitespace is left uncovered — which keeps
memory keys stable regardless of which engine ran.

Selection happens at three altitudes, each narrower than the last:

- **A flow step** names the engine in its config, which is the normal case:
  segmentation is an ordinary annotation stage placed ahead of recycling and
  translation.
- **A project** pins per-tool defaults under `defaults.tools`, so a recipe can
  fix the engine and its parameters once for every flow in the project.
- **A single invocation** overrides both — `kapi exec segmentation <file>
  --engine srx --rules-path .kapi/rules.srx`.

Separately, `defaults.segmentation` in the recipe is the **extract-side toggle**
rather than an engine selector: `source` turns the opt-in segmentation overlay
on for `kapi extract`, and `srx` optionally points at a ruleset file
([M-01](m-01-bilingual-interop.md)).

For a quick file-free tweak the tool also accepts an inline list of break and
no-break regex pairs; an inline list overrides the engine selection. Beyond a
rule or two, a real SRX file is the portable form.

## A plugin-declared engine

A plugin declares its segmenters in `capabilities.segmenters` in its manifest —
a name, a display name and description for the engine selector, an ordering, and
a path to a JSON parameter schema relative to the plugin directory. The host
walks every daemon-transport plugin it discovered, registers each declared
engine into the segment registry with the schema loaded from that file, and
routes it to a generic bridge segmenter.

**There is no per-plugin code in the host.** The bridge flattens the runs under
the shared mask options, sends the masked text and the config parameters to the
plugin's `Segment` RPC, and projects the returned interior boundaries back to
run-anchored spans through the same flattening it used on the way out — exactly
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
Text* models — XLM-RoBERTa-based ONNX models that segment any language the
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
  already provides exactly that — a pooled, long-lived subprocess with an idle
  timeout — which is why the segmenter uses it rather than a bespoke protocol.

The plugin is its own Go module, isolated so its native dependencies never enter
another module's build graph, and it builds two ways from one source tree. The
**default build** links no native libraries and is safe on any machine: the
daemon still serves, the handshake and capability probing still answer, and a
segment request reports the build limitation instead of crashing. The **ONNX
build** links the tokenizer archive and loads the ONNX Runtime shared library at
runtime; that is the configuration shipped in release archives. Because the
inference algorithm — the block/recombine windowing, the half-precision
conversion, the rune mapping — is pure Go, its tests build and run with no
native dependency.

Models are **not bundled**. The manifest pins each model's files by URL and
SHA-256, and the host's model-asset machinery downloads them on first use into
a shared cache, so a model is fetched once and verified before it is used. The
ONNX Runtime shared library *is* shipped beside the binary and resolved from
there, so an installed plugin needs no environment configuration.

The manifest also sets `capabilities.selfcheck`, which advertises the standard
self-check that `kapi plugins doctor` runs: it constructs the engine and lists
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

- [F-02: The content model](../foundations/f-02-content-model.md) — overlays, spans, and run ranges
- [F-03: Identity](../foundations/f-03-identity.md) — why a segmentation toggle does not move a block's hash
- [E-03: The tool system](../engine/e-03-tool-system.md) — the `segmentation` tool and its composed schema
- [E-05: The plugin system](../engine/e-05-plugin-system.md) — manifest capabilities, the daemon transport, signed distribution
- [E-07: Model and translation providers](../engine/e-07-model-providers.md) — the registry pattern the engine registry mirrors
- [M-01: Bilingual format interop](m-01-bilingual-interop.md) — how spans project into a bilingual file and back
- [C-09: Content memory](../context/c-09-content-memory.md) — per-segment lookup, the reason segmentation exists
- [Segmentation](/framework/segmentation) — the configuration guide, with flags and worked recipes
