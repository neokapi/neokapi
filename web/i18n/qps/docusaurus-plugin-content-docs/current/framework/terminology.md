---
sidebar_position: 11
title: Terminology
description: neokapi's concept-oriented terminology system groups multi-locale terms under language-neutral concepts with lifecycle status and grammatical metadata — backing the kapi terms commands and the term-check pipeline tool.
keywords: [terminology, terms, TBX, concepts, term enforcement, quality checks]
---

import { PipelineDiagram, StreamDiagram } from "@neokapi/docs-shared";

# ▒ Ţéŕḿîñöļöĝý ▒

▒ ñéöķàþî ḿàñàĝéš ţéŕḿîñöļöĝý ŵîţĥ à çöñçéþţ-öŕîéñţéđ ḿöđéļ îñšþîŕéđ ƃý ţĥé ŢƂẊ
(ŢéŕḿƂàšé éẊçĥàñĝé) šţàñđàŕđ: ļàñĝüàĝé-ñéüţŕàļ çöñçéþţš ĝŕöüþ ḿüļţî-ļöçàļé
ţéŕḿš, éàçĥ çàŕŕýîñĝ à ļîƒéçýçļé šţàţüš àñđ öþţîöñàļ ĝŕàḿḿàţîçàļ ḿéţàđàţà. Ţĥé
šàḿé ḿöđéļ ƃàçķš ţĥé `ķàþî ţéŕḿš` çöḿḿàñđš, ţĥé `ţéŕḿ-ļööķüþ` àñđ
`ţéŕḿ-éñƒöŕçé` þîþéļîñé ţööļš, àñđ ţĥé `ţéŕḿš/` Ĝö ļîƃŕàŕý. ▒

## ▒ Çöñçéþţ-öŕîéñţéđ ḿöđéļ ▒

▒ À **çöñçéþţ** îš à ļàñĝüàĝé-ñéüţŕàļ ķñöŵļéđĝé üñîţ. Îţ çàŕŕîéš à đöḿàîñ àñđ à
đéƒîñîţîöñ, àñđ ĝŕöüþš **ţéŕḿš** àçŕöšš ļöçàļéš. Éàçĥ ţéŕḿ ĥàš à ļîƒéçýçļé
šţàţüš, àñđ à ļöçàļé ḿàý ĥöļđ šéṽéŕàļ ţéŕḿš (à þŕéƒéŕŕéđ ƒöŕḿ þļüš àđḿîţţéđ
ṽàŕîàñţš). ▒

<StreamDiagram
  title='Concept — "cloud storage"'
  ariaLabel='Concept "cloud storage": its domain, its definition, and its terms in English, French, German and Japanese'
  items={[
    { kind: "Domain", detail: '"infrastructure"', role: "meta" },
    { kind: "Definition", detail: '"Remote file storage accessed via internet"', role: "meta" },
    { kind: "Term · en", detail: '"cloud storage"', role: "block", note: "preferred" },
    { kind: "Term · fr", detail: '"stockage cloud"', role: "block", note: "preferred" },
    { kind: "Term · fr", detail: '"stockage en nuage"', role: "block", note: "admitted" },
    { kind: "Term · de", detail: '"Cloud-Speicher"', role: "block", note: "preferred" },
    { kind: "Term · ja", detail: '"クラウドストレージ"', role: "block", note: "preferred" },
  ]}
/>

▒ Ţĥîš đîƒƒéŕš ƒŕöḿ à ƒļàţ ļîšţ öƒ šöüŕçé→ţàŕĝéţ þàîŕš àñđ îš ŵĥàţ éñàƃļéš
ḿüļţîþļé ţéŕḿš þéŕ ļöçàļé, šţàţüš-đŕîṽéñ éñƒöŕçéḿéñţ, àñđ ŕîçĥ ḿéţàđàţà
àţţàçĥéđ ţö à šîñĝļé ļàñĝüàĝé-ñéüţŕàļ çöñçéþţ. ▒

### ▒ Ţéŕḿ ļîƒéçýçļé šţàţüšéš ▒

| Status       | Meaning                       | Usage                           |
| ------------ | ----------------------------- | ------------------------------- |
| `preferred`  | The recommended term          | Always suggest to translators   |
| `approved`   | Accepted for use              | Valid alternative               |
| `admitted`   | Allowed but not recommended   | Show with lower priority        |
| `deprecated` | Being phased out              | Warn when found in translations |
| `proposed`   | Under review, not yet approved | Show as suggestion with caveat |
| `forbidden`  | Must not be used              | Flag as error in QA             |

## ▒ Çöñçéþţ ŕéļàţîöñš ▒

▒ Çöñçéþţš àŕé ñöţ îšļàñđš. À ţéŕḿš šţöŕé þéŕšîšţš ţýþéđ, đîŕéçţéđ **ŕéļàţîöñš**
ƃéţŵééñ çöñçéþţš, šö à ŕéñàḿéđ þŕöđüçţ þöîñţš àţ îţš ŕéþļàçéḿéñţ àñđ à
đéþŕéçàţéđ ţéŕḿ þöîñţš àţ ţĥé öñé ţö üšé îñšţéàđ. Ţĥé ŕéļàţîöñ ṽöçàƃüļàŕý îš
àļîĝñéđ ŵîţĥ [ŠĶÖŠ](https://www.w3.org/2004/02/skos/): ▒

| Category        | Labels                          | Meaning                              |
| --------------- | ------------------------------- | ------------------------------------ |
| Hierarchy       | `broader`, `narrower`           | A parent/child concept relationship  |
| Composition     | `part-of`, `has-part`           | A whole/component relationship       |
| Association     | `related`                       | A non-hierarchical association       |
| Succession      | `replaced-by`                   | A concept superseded by another      |
| Guidance        | `use-instead`                   | A discouraged term points at a preferred one |
| Cross-scheme    | `exact-match`, `close-match`    | Equivalence across schemes           |
| Stance          | `competitor`                    | A competitor's term                  |

▒ Éàçĥ éđĝé îš à çöñçéþţ þöîñţîñĝ àţ àñöţĥéŕ üñđéŕ öñé öƒ ţĥöšé ļàƃéļš: ▒

<PipelineDiagram
  channelLabel="use-instead"
  stages={[
    { label: "utilize", sub: "forbidden", role: "qa" },
    { label: "use", sub: "preferred", role: "io" },
  ]}
/>

<PipelineDiagram
  channelLabel="replaced-by"
  stages={[
    { label: "web store", sub: "deprecated", role: "qa" },
    { label: "marketplace", sub: "preferred", role: "io" },
  ]}
/>

<PipelineDiagram
  channelLabel="broader"
  stages={[{ label: "cloud storage" }, { label: "infrastructure" }]}
/>

▒ À ŕéļàţîöñ îš à ƒîŕšţ-çļàšš ŕéçöŕđ ŵîţĥ àñ ÎĐ, à šöüŕçé àñđ ţàŕĝéţ çöñçéþţ, à
ţýþé ƒŕöḿ ţĥé ṽöçàƃüļàŕý àƃöṽé, àñ öþţîöñàļ ñöţé, àñđ àñ öþţîöñàļ ṽàļîđîţý
(ƃéļöŵ). Ţĥé ţéŕḿš šţöŕé ṽàļîđàţéš ţĥàţ ţĥé ţýþé îš ķñöŵñ àñđ ţĥàţ ƃöţĥ çöñçéþţš
éẋîšţ ƃéƒöŕé þéŕšîšţîñĝ àñ éđĝé. ▒

### ▒ Ŕéļàţîöñ àñđ ţéŕḿ ṽàļîđîţý ▒

▒ À ŕéļàţîöñ, àñđ àñ îñđîṽîđüàļ ţéŕḿ, ḿàý çàŕŕý à **ṽàļîđîţý**: à ĥàļƒ-öþéñ ţîḿé
îñţéŕṽàļ `[ṽàļîđ-ƒŕöḿ, ṽàļîđ-ţö)` þļüš à šéţ öƒ ƒŕéé-ƒöŕḿ ţàĝš. À ǫüéŕý šüþþļîéš
à **šçöþé** — à þöîñţ îñ ţîḿé àñđ à šéţ öƒ ţàĝš — àñđ öñļý éđĝéš àñđ ţéŕḿš ŵĥöšé
ṽàļîđîţý ḿàţçĥéš ţĥé šçöþé àŕé ŕéţüŕñéđ. À ñîļ ṽàļîđîţý îš üñƃöüñđéđ (îţ ḿàţçĥéš
éṽéŕý šçöþé); à ñîļ šçöþé àþþļîéš ñö ƒîļţéŕîñĝ. ▒

▒ Ţĥîš ḿàķéš ţĥé šàḿé ţéŕḿš šţöŕé àñšŵéŕ šçöþé-đéþéñđéñţ ǫüéšţîöñš: ŵĥîçĥ ţéŕḿš ŵéŕé
þŕéƒéŕŕéđ *àš öƒ* ļàšţ ǫüàŕţéŕ, öŕ ŵĥîçĥ ŕéļàţîöñš ĥöļđ *ŵîţĥîñ* à ĝîṽéñ ḿàŕķéţ.
Ţàĝš àŕé öþéñ-éñđéđ (ţĥé ƒŕàḿéŵöŕķ àššîĝñš ţĥéḿ ñö ḿéàñîñĝ); à çàļļéŕ çĥööšéš à
ţàĝ ṽöçàƃüļàŕý — ƒöŕ éẋàḿþļé à `ḿàŕķéţ` ķéý — àñđ üšéš îţ çöñšîšţéñţļý. À ñîļ
ṽàļîđîţý ḿàţçĥéš éṽéŕý šçöþé; à ñîļ šçöþé ƒîļţéŕš ñöţĥîñĝ. ▒

### ▒ Šţàţüš ţŕàñšîţîöñš ▒

▒ À ţéŕḿ'š šţàţüš çĥàñĝéš öṽéŕ îţš ļîƒéţîḿé. `ṼàļîđàţéŢŕàñšîţîöñ(ƒŕöḿ, ţö)`
àççéþţš àñý ţŕàñšîţîöñ ƃéţŵééñ ķñöŵñ šţàţüšéš — ĥîšţöŕý îš ţĥé ĝüàŕđ, ñöţ à
ţŕàþ — ŵĥîļé `ÎšĜöṽéŕñéđŢŕàñšîţîöñ(ƒŕöḿ, ţö)` ŕéþöŕţš ŵĥéţĥéŕ à çĥàñĝé îš
çöñšéǫüéñţîàļ éñöüĝĥ ţö đéšéŕṽé ŕéṽîéŵ: àñý ţŕàñšîţîöñ **ţö** `ƒöŕƃîđđéñ` öŕ
`þŕéƒéŕŕéđ`, öŕ àñý ţŕàñšîţîöñ **ƒŕöḿ** `ƒöŕƃîđđéñ`. Ţĥé ƒŕàḿéŵöŕķ öñļý
çļàššîƒîéš; à þļàţƒöŕḿ ƃüîļţ öñ îţ đéçîđéš ŵĥàţ ĝöṽéŕñàñçé à ĝöṽéŕñéđ ţŕàñšîţîöñ
ŕéǫüîŕéš. ▒

## ▒ Šţöŕàĝé ƃàçķéñđš ▒

▒ Ţŵö ƃàçķéñđš šĥîþ îñ ţĥé `ţéŕḿš/` þàçķàĝé, ƃöţĥ ţĥŕéàđ-šàƒé
(ŔŴḾüţéẋ-þŕöţéçţéđ) àñđ îḿþļéḿéñţîñĝ ţĥé ƒüļļ `Ţéŕḿîñöļöĝý` îñţéŕƒàçé: ▒

1. ▒ **Îñ-ḿéḿöŕý** (`ţéŕḿš.ÑéŵÎñḾéḿöŕýŠţöŕé`) — ƒàšţ àñđ éþĥéḿéŕàļ, üšéđ
   ƒöŕ šéššîöñ-šçöþéđ ƃàţçĥ þŕöçéššîñĝ. ▒
2. ▒ **ŠǪĻîţé** (`ţéŕḿš.ÑéŵŠǪĻîţéŠţöŕé`) — þéŕšîšţéñţ ƒîļé-ƃàšéđ šţöŕàĝé
   ƒöŕ ÇĻÎ ŵöŕķƒļöŵš, ŵîţĥ ƒüžžý ḿàţçĥîñĝ ṽîà ŠǪĻ-ƃàšéđ Ļéṽéñšĥţéîñ đîšţàñçé. ▒

▒ Ţĥé `Ţéŕḿîñöļöĝý` îñţéŕƒàçé àļšö àççöḿḿöđàţéš šéŕṽéŕ-šîđé ƃàçķéñđš ƒöŕ ḿüļţî-üšéŕ
đéþļöýḿéñţš ŵîţĥ þŕöĵéçţ šçöþîñĝ, ţéŕḿîñöļöĝý šţŕéàḿš, àñđ ŵöŕķšþàçé îšöļàţîöñ. ▒

## ▒ ÇĻÎ üšàĝé ▒

### ▒ Ŕéšöüŕçé ļöçàţîöñ ▒

▒ Àļļ `ķàþî ţéŕḿš` çöḿḿàñđš (éẋçéþţ `ļîšţ`) àççéþţ ţĥéšé ḿüţüàļļý éẋçļüšîṽé ƒļàĝš: ▒

| Flag            | Resolves to                         | Example                      |
| --------------- | ----------------------------------- | ---------------------------- |
| `--name <n>`    | `~/.config/kapi/terms/<n>.db`   | `--name project-terms`       |
| `--local`       | `./terms.db` (current directory) | `--local`                    |
| `--file <path>` | Explicit file path                  | `--file /shared/terms.db`    |
| _(no flag)_     | Same as `--local`                   |                              |

▒ Đàţàƃàšéš àŕé çŕéàţéđ öñ đéḿàñđ îƒ ţĥéý đöñ'ţ éẋîšţ. `ţéŕḿš/` àñđ `ţéŕḿš.đƃ`
àŕé ţĥé öñ-đîšķ ñàḿéš ƒöŕ à ñàḿéđ öŕ ļööšé ţéŕḿš šţöŕé. `~/.çöñƒîĝ/ķàþî` îš ţĥé
üšéŕ çöñƒîĝ đîŕéçţöŕý öñ Ļîñüẋ, àñđ ŕéšöļṽéš ţö
`~/Ļîƃŕàŕý/Àþþļîçàţîöñ Šüþþöŕţ/ķàþî` öñ ḿàçÖŠ. `ķàþî çöñƒîĝ þàţĥ` þŕîñţš ţĥé
ŕéšöļṽéđ ļöçàţîöñ. ▒

▒ Ŵîţĥ ñö ƒļàĝ îñšîđé à þŕöĵéçţ, ţĥé ţéŕḿš šţöŕé îš îñšţéàđ à šéţ öƒ ţàƃļéš îñ ţĥé
þŕöĵéçţ'š `.ķàþî/ŵöŕķ/šţöŕé.đƃ`, çöḿþîļéđ ƒŕöḿ ţĥé çöḿḿîţţéđ ƃüñđļé ţĥé ŕéçîþé ƃîñđš
ŵîţĥ `đéƒàüļţš.ţéŕḿš_šöüŕçé` — šéé
[Ḿéḿöŕý & ţéŕḿš šţöŕàĝé](/kapi/recipes/memory-and-terms-storage). ▒

```bash
# Import terms
kapi terms import terms.json --name project-terms

# Export terms
kapi terms export --name project-terms --format bundle -o terms.json

# Look up a term (exact, or --fuzzy)
kapi terms lookup "encryption" --name project-terms -s en -t fr
kapi terms lookup "authenticating users" -s en -t fr --fuzzy

# Search concepts, view statistics, list named terms stores
kapi terms search "auth" -s en --limit 50
kapi terms stats --name project-terms
kapi terms list
```

▒ Ţĥé `ķàþî ţéŕḿš` çöḿḿàñđš çöṽéŕ îḿþöŕţ, éẋþöŕţ, ļööķüþ, šéàŕçĥ,
šţàţîšţîçš, àñđ ļîšţîñĝ. Çöñçéþţ **ŕéļàţîöñš** àŕé ñöţ éđîţéđ ƒŕöḿ ţĥé
çöḿḿàñđ ļîñé: ţĥéý àŕé àüţĥöŕéđ ṽîšüàļļý. Ķàþî Đéšķţöþ öþéñš à þéŕ-çöñçéþţ
đàšĥƃöàŕđ — ţĥé `@ñéöķàþî/çöñçéþţ-üî` çöḿþöñéñţ, ŵĥîçĥ šĥöŵš à çöñçéþţ'š
ţéŕḿš, ĝéöĝŕàþĥý, çöñšţŕàîñţš, à ļöçàļ ŕéļàţîöñš ŵîđĝéţ, àñđ à ţîḿéļîñé —
öṽéŕ à ļöçàļ ţéŕḿš šţöŕé, ŵĥéŕé àñ éđîţöŕ àđđš, ŕéţýþéš, šçöþéš, àñđ ŕéḿöṽéš
éđĝéš đîŕéçţļý. Ţĥé ŕéļàţîöñ đàţà ţĥîš þŕöđüçéš îš ţĥé šàḿé
`ÇöñçéþţŔéļàţîöñ` ŕéçöŕđš þéŕšîšţéđ ƃý ţĥé ţéŕḿš šţöŕé àñđ ŕéàđ ţĥŕöüĝĥ ţĥé [Ĝö
ÀÞÎ](#go-library) ƃéļöŵ. ▒

## ▒ Þîþéļîñé îñţéĝŕàţîöñ ▒

▒ Ţŵö þîþéļîñé ţööļš ƃŕîñĝ ţéŕḿîñöļöĝý îñţö ţĥé ţŕàñšļàţîöñ ƒļöŵ: ▒

- ▒ **`ţéŕḿ-ļööķüþ`** šçàñš éàçĥ Ƃļöçķ'š šöüŕçé ţéẋţ àñđ àţţàçĥéš ḿàţçĥéđ
  ţéŕḿîñöļöĝý àš `ŢéŕḿÀññöţàţîöñ` éñţŕîéš (šöüŕçé ţéŕḿ, ţàŕĝéţ šüĝĝéšţîöñš,
  þöšîţîöñš, šţàţüš). Îţ çàñ àļšö þöŵéŕ þéŕ-ƃļöçķ šüĝĝéšţîöñš îñ àñ éđîţöŕ. ▒
- ▒ **`ţéŕḿ-éñƒöŕçé`** çĥéçķš ţĥàţ ţŕàñšļàţéđ ƃļöçķš üšé ţĥé éẋþéçţéđ
  ţéŕḿîñöļöĝý. Ṽîöļàţîöñš àŕé ŕéþöŕţéđ àš ƃļöçķ þŕöþéŕţîéš
  (`ţéŕḿ-éñƒöŕçé-éŕŕöŕš`, `ţéŕḿ-éñƒöŕçé-ṽîöļàţîöñš`) àñđ àš àññöţàţîöñš ŵîţĥ
  éẋþéçţéđ-ṽš-àçţüàļ đéţàîļ. ▒

## ▒ Ĝö ļîƃŕàŕý ▒

### ▒ Îñţéŕƒàçé ▒

```go
type Terminology interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (Concept, bool)
    DeleteConcept(id string) error
    Lookup(sourceText string, opts LookupOptions) []TermMatch
    LookupAll(sourceText string, opts LookupOptions) []TermMatch
    Search(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int)

    // Relations between concepts, optionally validity-scoped.
    AddRelation(rel ConceptRelation) error
    DeleteRelation(id string) error
    RelationsOf(conceptID string, scope *graph.Scope) []ConceptRelation // both directions
    ListRelations(scope *graph.Scope) []ConceptRelation

    Count() int
    Concepts() []Concept
    Close() error
}
```

▒ (Ḿéţĥöđš, àñđ ţĥé îḿþöŕţ/éẋþöŕţ ĥéļþéŕš ƃéļöŵ, ţàķé à `çöñţéẋţ.Çöñţéẋţ` àš ţĥéîŕ
ƒîŕšţ àŕĝüḿéñţ îñ ţĥé ŕéàļ ÀÞÎ; îţ îš éļîđéđ ĥéŕé ƒöŕ ŕéàđàƃîļîţý.) ▒

▒ `Ļööķüþ` ƒîñđš ţĥé ƃéšţ ḿàţçĥ ƒöŕ à šîñĝļé ţéŕḿ. `ĻööķüþÀļļ` šçàñš ŕüññîñĝ ţéẋţ
àñđ ŕéţüŕñš éṽéŕý ţéŕḿ öççüŕŕéñçé ŵîţĥ þöšîţîöñš — ţĥîš îš ŵĥàţ þöŵéŕš ţĥé
`ţéŕḿ-ļööķüþ` ţööļ àñđ éđîţöŕ šüĝĝéšţîöñš. Ƃý đéƒàüļţ `ĻööķüþÀļļ` ḿàţçĥéš
çàšé-îñšéñšîţîṽéļý (ţéŕḿîñöļöĝý šĥöüļđ ƃé ŕéçöĝñîžéđ ŕéĝàŕđļéšš öƒ
çàþîţàļîžàţîöñ); šéţ `ÇàšéŠéñšîţîṽé` ţö öṽéŕŕîđé. ▒

### ▒ Ķéý ţýþéš ▒

```go
type Concept struct {
    ID         string
    Domain     string            // subject area (security, ui, marketing)
    Definition string            // language-neutral description
    Terms      []Term
    Properties map[string]string // extensible metadata
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type Term struct {
    Text         string
    Locale       model.LocaleID
    Status       model.TermStatus // preferred, approved, admitted, deprecated, proposed, forbidden
    PartOfSpeech string
    Gender       string
    Note         string
    Validity     *graph.Validity // optional time + tag scope (nil = unbounded)
}

type ConceptRelation struct {
    ID           string
    SourceID     string
    TargetID     string
    RelationType string          // a SKOS-aligned label: broader, use-instead, replaced-by, …
    Note         string
    Validity     *graph.Validity // optional time + tag scope (nil = unbounded)
    CreatedAt    time.Time
}

type TermMatch struct {
    Concept   Concept
    Term      Term                // the matched source term
    Score     float64             // 0.0-1.0
    MatchType model.MatchStrategy // exact, normalized, fuzzy
    Position  model.TextRange     // position in source text
}

type LookupOptions struct {
    SourceLocale  model.LocaleID
    TargetLocale  model.LocaleID
    CaseSensitive bool
    MinScore      float64             // minimum fuzzy score (default 0.8)
    MatchModes    []model.MatchStrategy
    Domains       []string            // restrict to specific domains
    StatusFilter  []model.TermStatus  // only return terms with these statuses
}
```

▒ `Çöñçéþţ` ĥéļþéŕš: `ŠöüŕçéŢéŕḿ(ļöçàļé)`, `ŢàŕĝéţŢéŕḿš(ļöçàļé)`,
`ÞŕéƒéŕŕéđŢéŕḿ(ļöçàļé)`. ▒

### ▒ Éẋàḿþļé ▒

```go
package main

import (
    "fmt"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/terms"
)

func main() {
    tb := terms.NewInMemoryStore()
    defer tb.Close()

    tb.AddConcept(terms.Concept{
        ID:         "c1",
        Domain:     "security",
        Definition: "Process of encoding information",
        Terms: []terms.Term{
            {Text: "encryption", Locale: "en", Status: model.TermPreferred},
            {Text: "chiffrement", Locale: "fr", Status: model.TermPreferred},
        },
    })

    matches := tb.LookupAll(
        "The encryption module handles end-to-end encryption",
        terms.LookupOptions{SourceLocale: "en", TargetLocale: "fr"},
    )
    for _, m := range matches {
        fmt.Printf("Found %q at [%d:%d] → %s (%s)\n",
            m.Term.Text, m.Position.Start, m.Position.End,
            m.Concept.TargetTerms("fr")[0].Text, m.Term.Status)
    }
}
```

### ▒ Îḿþöŕţ / éẋþöŕţ ▒

```go
// JSON preserves the full concept-oriented structure
count, err := terms.ImportJSON(tb, reader)
err = terms.ExportJSON(tb, writer, "Project Terms")

// CSV is a flat source/target form with optional metadata
opts := terms.CSVImportOptions{
    SourceLocale: "en", TargetLocale: "fr", Domain: "general", HasHeader: true,
}
count, err = terms.ImportCSV(tb, reader, opts)
err = terms.ExportCSV(tb, writer, "en", "fr", true)
```

▒ ÇŠṼ çöļüḿñš àŕé `šöüŕçé,ţàŕĝéţ,đöḿàîñ` (đöḿàîñ öþţîöñàļ). ĴŠÖÑ çàŕŕîéš ţĥé ƒüļļ
çöñçéþţ šţŕüçţüŕé: ▒

```json
{
  "name": "Project Terms",
  "version": "1.0",
  "concepts": [
    {
      "id": "c1",
      "domain": "security",
      "definition": "Encryption where only endpoints can decrypt",
      "terms": [
        { "text": "end-to-end encryption", "locale": "en", "status": "preferred" },
        { "text": "chiffrement de bout en bout", "locale": "fr", "status": "preferred" }
      ]
    }
  ]
}
```

## ▒ Ţéŕḿîñöļöĝý àñđ çöñţéñţ ḿéḿöŕý ▒

▒ Ţéŕḿîñöļöĝý àñđ [çöñţéñţ ḿéḿöŕý](/framework/content-memory) àŕé
đéļîƃéŕàţéļý šéþàŕàţé šýšţéḿš ƃéçàüšé ţĥéý àñšŵéŕ đîƒƒéŕéñţ ǫüéšţîöñš: ▒

- ▒ **Çöñţéñţ ḿéḿöŕý** — "Ĥöŵ ŵàš ţĥîš šéñţéñçé ŕéñđéŕéđ ƃéƒöŕé?" (šéĝḿéñţ þàîŕš). ▒
- ▒ **Ţéŕḿîñöļöĝý** — "Ŵĥàţ îš ţĥé çöŕŕéçţ ţéŕḿ ƒöŕ ţĥîš çöñçéþţ?" (ḿüļţî-ļöçàļé
  ķñöŵļéđĝé üñîţš). ▒

▒ Ţĥéý šĥàŕé ţĥé `Ƃļöçķ` àññöţàţîöñ šýšţéḿ àš ţĥéîŕ îñţéĝŕàţîöñ þöîñţ, šö ƃöţĥ
ḿéḿöŕý ḿàţçĥéš àñđ ţéŕḿ ḿàţçĥéš àŕé àṽàîļàƃļé ţö àñý đöŵñšţŕéàḿ ţööļ öŕ éđîţöŕ. ▒

▒ Ţéŕḿîñöļöĝý àñđ [šéĝḿéñţàţîöñ](/framework/segmentation) àŕé ŕüñ-àñçĥöŕéđ öṽéŕļàýš
þŕöđüçéđ îñ ţĥé [çöñţéñţ-þŕéþàŕàţîöñ](/framework/content-preparation) þàšš ţĥàţ
ŕéàđîéš à šöüŕçé ƃéƒöŕé ţŕàñšļàţîöñ. ▒
