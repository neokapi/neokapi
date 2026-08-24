---
sidebar_position: 2
title: Skeleton Store and Streaming HTML
description: Implementation note for E-02 — details of the SkeletonStore temp-file-backed binary store and the tokenizer-based HTML reader/writer that uses skeleton entries to faithfully reconstruct documents.
keywords: [SkeletonStore, streaming HTML, skeleton, tokenizer, HTML reader, implementation note, neokapi]
---

import { StreamDiagram } from "@neokapi/docs-shared";

# ▒ Šķéļéţöñ Šţöŕé àñđ Šţŕéàḿîñĝ ĤŢḾĻ ▒

▒ Îḿþļéḿéñţàţîöñ đéţàîļš ƒöŕ ţĥé `ŠķéļéţöñŠţöŕé` ƒŕàḿéŵöŕķ ţýþé àñđ ţĥé
ţöķéñîžéŕ-ƃàšéđ ĤŢḾĻ ŕéàđéŕ/ŵŕîţéŕ ţĥàţ üšéš îţ. Þàŕéñţ ÀĐ:
[É-02](/contribute/architecture/engine/e-02-format-system) (šķéļéţöñ šţŕàţéĝîéš). ▒

## ▒ ŠķéļéţöñŠţöŕé (`çöŕé/ƒöŕḿàţ/šķéļéţöñ.ĝö`) ▒

▒ À ţéḿþ-ƒîļé-ƃàçķéđ ƃîñàŕý šţöŕé ƒöŕ đöçüḿéñţ šķéļéţöñ đàţà. Ţĥé ŕéàđéŕ ŵŕîţéš
éñţŕîéš đüŕîñĝ éẋţŕàçţîöñ; ţĥé ŵŕîţéŕ ŕéàđš ţĥéḿ đüŕîñĝ ŕéçöñšţŕüçţîöñ. Ţĥé
þîþéļîñé (ţööļš) ñéṽéŕ šééš ţĥé šķéļéţöñ — îţ öñļý çàŕŕîéš ƃļöçķš. ▒

### ▒ Ƃîñàŕý ƒöŕḿàţ ▒

▒ Éàçĥ éñţŕý îš: ▒

```
[type:1 byte] [length:4 bytes big-endian] [data:N bytes]
```

| Type byte | Meaning | Data contents              |
| --------- | ------- | -------------------------- |
| `0`       | Text    | Non-translatable raw bytes |
| `1`       | Ref     | Block ID as UTF-8 string   |
| `2`       | Lang    | Source-locale `lang`/`xml:lang` attribute value (raw bytes between the quotes), spliced for language retargeting |

▒ Ţĥé `Ļàñĝ` éñţŕý ļéţš à ŵŕîţéŕ ŕéţàŕĝéţ ţĥé đöçüḿéñţ ļàñĝüàĝé: ŵĥéñ ţĥé šţöŕéđ
ṽàļüé ḿàţçĥéš ţĥé đöçüḿéñţ'š šöüŕçé ļöçàļé îţ éḿîţš ţĥé ţàŕĝéţ ļöçàļé,
öţĥéŕŵîšé îţ éḿîţš ţĥé šţöŕéđ ṽàļüé ṽéŕƃàţîḿ. Ŵŕîţéŕš ţĥàţ đö ñöţ üñđéŕšţàñđ
ţĥé ţýþé ḿüšţ ţŕéàţ îţ àš îñéŕţ (éḿîţţîñĝ ñöţĥîñĝ ŵöüļđ đŕöþ ţĥé àţţŕîƃüţé
ṽàļüé). Öñļý ţĥé ĤŢḾĻ ŕéàđéŕ éḿîţš `Ļàñĝ` ţöđàý; öţĥéŕ ƒöŕḿàţš ñéṽéŕ šéé îţ,
àñđ ƃéçàüšé ţĥéîŕ éñţŕý-ţýþé šŵîţçĥéš ĥàṽé ñö `đéƒàüļţ` çàšé ţĥé àđđîţîöñ îš
þüŕéļý àđđîţîṽé. ▒

▒ Ţĥé ƒöŕḿàţ îš àþþéñđ-öñļý đüŕîñĝ ŵŕîţîñĝ àñđ šéǫüéñţîàļ đüŕîñĝ ŕéàđîñĝ. Àƒţéŕ
`Ƒļüšĥ()`, ţĥé ƒîļé îš šééķéđ ţö ţĥé ƃéĝîññîñĝ àñđ éñţŕîéš àŕé ŕéàđ ŵîţĥ
`Ñéẋţ()` üñţîļ `îö.ÉÖƑ`. ▒

### ▒ ÀÞÎ ▒

```go
func NewSkeletonStore() (*SkeletonStore, error)    // creates temp file in os.TempDir()
func (s *SkeletonStore) WriteText(data []byte) error // skips empty data
func (s *SkeletonStore) WriteRef(blockID string) error
func (s *SkeletonStore) WriteLang(value string) error // language-attribute value for retargeting
func (s *SkeletonStore) Flush() error                // flushes buffered writer, seeks to 0
func (s *SkeletonStore) Next() (SkeletonEntry, error) // returns io.EOF at end
func (s *SkeletonStore) Close() error                 // removes temp file
```

▒ `ŴŕîţéŢéẋţ` šķîþš éḿþţý ƃýţé šļîçéš ţö àṽöîđ ŵŕîţîñĝ ñö-öþ éñţŕîéš. ▒

### ▒ Îñţéŕƒàçéš ▒

```go
// Implemented by readers that write skeleton data during extraction.
type SkeletonStoreEmitter interface {
    SetSkeletonStore(store *SkeletonStore)
}

// Implemented by writers that read skeleton data during reconstruction.
type SkeletonStoreConsumer interface {
    SetSkeletonStore(store *SkeletonStore)
}
```

### ▒ Ƒļöŵ éẋéçüţöŕ ŵîŕîñĝ ▒

▒ Ţĥé šķéļéţöñ šţöŕé ḿüšţ ƃé ŵîŕéđ **ƃéƒöŕé** `ŕéàđéŕ.Ŕéàđ()` îš çàļļéđ, šîñçé
ţĥé ŕéàđéŕ ŵŕîţéš šķéļéţöñ éñţŕîéš đüŕîñĝ ŕéàđîñĝ. Ţĥîš ŕéǫüîŕéš çŕéàţîñĝ ţĥé
ŵŕîţéŕ éàŕļý (ƃéƒöŕé ŕéàđîñĝ), ŵĥîçĥ îš à çĥàñĝé ƒŕöḿ ţĥé öŕîĝîñàļ ƒļöŵ ŵĥéŕé
ţĥé ŵŕîţéŕ ŵàš çŕéàţéđ àƒţéŕ ŕéàđîñĝ. ▒

▒ Ţĥéŕé àŕé ţŵö þàţĥš: ▒

1. ▒ **Ţĥé ƒļöŵ þàţĥ** — üšéđ ƃý `çļî/ƒļöŵ.ĝö` `ŕüñŠîñĝļéƑîļé()` àñđ
   `ķàþî/çḿđ/ķàþî/ḿçþ_ţööļš.ĝö` `éẋéçüţéƑļöŵ()`/`éẋéçüţéƑļöŵŴîţĥŢööļš()`. Ñéîţĥéŕ
   ŵîŕéš ţĥé šķéļéţöñ šţöŕé îñļîñé. Ƃöţĥ çöñšţŕüçţ à `ƒļöŵ.ƑîļéŔüññéŕ` ṽîà
   `ƒļöŵ.ÑéŵƑîļéŔüññéŕ(...)` àñđ çàļļ `ŕüññéŕ.ŔüñƑîļé(...)`, ŵĥîçĥ çŕéàţéš ţĥé
   ŵŕîţéŕ (îñ `ŔüñƑîļé`) ţĥéñ đéļéĝàţéš ţö
   `ƑîļéŔüññéŕ.ŔüñƑîļéŴîţĥŔéàđéŕŴŕîţéŕ()` (`çöŕé/ƒļöŵ/ƒîļéŕüññéŕ.ĝö`), ŵĥéŕé ţĥé
   šķéļéţöñ šţöŕé îš ŵîŕéđ çéñţŕàļļý ƃéƒöŕé `ŕéàđéŕ.Ŕéàđ()`. ▒
2. ▒ **Ţĥé ţööļ þàţĥ** — `çļî/ţööļŕüñ.ĝö` `þŕöçéššÖñéƑîļé()` — ŕéḿàîñš ţĥé öñļý
   îñļîñé ŵîŕîñĝ šîţé, çŕéàţîñĝ ţĥé ŵŕîţéŕ éàŕļý ţĥéñ đöîñĝ ţĥé
   éḿîţţéŕ/çöñšüḿéŕ ţýþé-àššéŕţ + `ŠéţŠķéļéţöñŠţöŕé` ƃļöçķ. ▒

▒ Ţĥé çéñţŕàļ ƒļöŵ-þàţĥ ŵîŕîñĝ (`ŔüñƑîļéŴîţĥŔéàđéŕŴŕîţéŕ`) ļööķš ļîķé: ▒

```go
// Wire skeleton store if both support it.
var skeletonStore *format.SkeletonStore
if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
    if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
        if store, storeErr := format.NewSkeletonStore(); storeErr == nil {
            skeletonStore = store
            emitter.SetSkeletonStore(store)
            consumer.SetSkeletonStore(store)
        }
    }
}

// Now read — the reader writes skeleton entries during this call.
for result := range reader.Read(ctx) { ... }
```

▒ Ţĥé šţöŕé îš ĥéļđ îñ à ļöçàļ `šķéļéţöñŠţöŕé` ṽàŕîàƃļé àñđ çļöšéđ éẋþļîçîţļý öñ
éàçĥ éŕŕöŕ þàţĥ àñđ àţ çöḿþļéţîöñ ŕàţĥéŕ ţĥàñ ṽîà à šîñĝļé `đéƒéŕ`, šîñçé ţĥé
ŵŕîţéŕ öüţļîṽéš ţĥé ƒüñçţîöñ ṽîà ţĥé ţéḿþ-ţĥéñ-ŕéñàḿé öüţþüţ. Ţĥé îñļîñé ţööļ
þàţĥ (`çļî/ţööļŕüñ.ĝö`) ƒöļļöŵš ţĥé šàḿé ţýþé-àššéŕţ šĥàþé ƃüţ üšéš à
`đéƒéŕ šţöŕé.Çļöšé()`. ▒

## ▒ Šüƃ-šķéļéţöñ: ţŕàñšļàţàƃļé šþàñš îñšîđé àñ öþàǫüé þàýļöàđ ▒

▒ Šöḿé éẋţŕàçţàƃļé çöñţéñţ îš éḿƃéđđéđ *îñšîđé* à þàýļöàđ ţĥé ŕéàđéŕ öţĥéŕŵîšé
çàþţüŕéš öþàǫüéļý àñđ ŕéþļàýš ṽéŕƃàţîḿ — ţĥé ñàţüŕàļ-ļàñĝüàĝé þŕöšé (`<ḿ:ñöŕ/>`
ŕüñš: "ŵĥéŕé", "öţĥéŕŵîšé", üñîţš) îñšîđé à Ŵöŕđ éǫüàţîöñ'š ÖḾḾĻ, ŵĥîçĥ îš
çàþţüŕéđ àš öñé öþàǫüé šéñţîñéļ ŕüñ ƒöŕ à ƃýţé-éẋàçţ ĐÖÇẊ ŕöüñđ-ţŕîþ
([Ḿ-04](/contribute/architecture/multilingual/m-04-math-and-equations)). À ƒļàţ
`Ţéẋţ`/`Ŕéƒ` šķéļéţöñ çàññöţ ŕéàçĥ îñţö ţĥàţ ƃļöƃ. ▒

▒ Ţĥé **šüƃ-šķéļéţöñ** îš ţĥé šàḿé `Ţéẋţ`/`Ŕéƒ` ḿéçĥàñîšḿ àþþļîéđ ŕéçüŕšîṽéļý öṽéŕ
ţĥé öþàǫüé ƃýţéš: îñšţéàđ öƒ éḿîţţîñĝ ţĥé ŵĥöļé þàýļöàđ àš öñé `Ţéẋţ` éñţŕý, ţĥé
ŕéàđéŕ éḿîţš ţĥé ṽéŕƃàţîḿ ƃýţé šéĝḿéñţš *ƃéţŵééñ* ţĥé ţŕàñšļàţàƃļé šþàñš àš
`Ţéẋţ` àñđ à `Ŕéƒ` ţö à ţŕàñšļàţàƃļé ƃļöçķ ƒöŕ éàçĥ šþàñ. Ñö ñéŵ éñţŕý ţýþé îš
ñééđéđ — îţ îš öŕđîñàŕý `ŴŕîţéŢéẋţ` / `ŴŕîţéŔéƒ` çàļļš ŵĥöšé `Ţéẋţ` ĥàþþéñš ţö ƃé
šļîçéš öƒ àñ öþàǫüé ƃļöƃ ŕàţĥéŕ ţĥàñ ţöþ-ļéṽéļ šţŕüçţüŕé. ▒

▒ Ƒöŕ ÖḾḾĻ (`çöŕé/ƒöŕḿàţš/öþéñẋḿļ/öḿḿļ_ḿàţĥ.ĝö`, `ŵŕîţéÖḾàţĥŠüƃŠķéļéţöñ`): ▒

1. ▒ `ẋḿàţĥ.ÑöŕŠþàñš(ŕàŵ)` (`çöŕé/ḿàţĥ/ñöŕ.ĝö`) ŕéţüŕñš éàçĥ þŕöšé šþàñ'š ţéẋţ þļüš
   îţš **ƃýţé öƒƒšéţ ŕàñĝé** îñţö ţĥé ŕàŵ ÖḾḾĻ (çàþţüŕéđ ṽîà
   `ẋḿļ.Đéçöđéŕ.ÎñþüţÖƒƒšéţ`). ▒
2. ▒ Ţĥé ŵŕîţéŕ ṽàļîđàţéš ţĥé öƒƒšéţš àŕé ḿöñöţöñîç àñđ îñ ŕàñĝé; îƒ ñöţ — öŕ îƒ
   ţĥéŕé àŕé ñö šþàñš — îţ ƒàļļš ƃàçķ ţö ŵŕîţîñĝ ţĥé þàýļöàđ ṽéŕƃàţîḿ, šö à
   þüŕé-ḿàţĥ éǫüàţîöñ îš üñàƒƒéçţéđ. ▒
3. ▒ Öţĥéŕŵîšé îţ ŵàļķš ţĥé šþàñš, éḿîţţîñĝ `šķéļŢéẋţ(ŕàŵ[çüŕšöŕ:šþàñ.Šţàŕţ])`
   (ṽéŕƃàţîḿ ÖḾḾĻ) ţĥéñ `šķéļŔéƒ(ƃļöçķÎĐ)` ƒöŕ à ţŕàñšļàţàƃļé `öḿḿļ-ñöŕ` ƃļöçķ,
   àđṽàñçîñĝ `çüŕšöŕ` þàšţ ţĥé šþàñ. ▒

▒ Öñ ŵŕîţé, ţĥé öþéñẋḿļ `ŕéñđéŕƂļöçķ` ŕéñđéŕš àñ `öḿḿļ-ñöŕ` ƃļöçķ àš **ƃàŕé
éļéḿéñţ-çöñţéñţ ţéẋţ** (`ẋḿļÉšçàþé`, ḿàţçĥîñĝ `çàþţüŕéŔàŵÉļéḿéñţ`'š ÇĥàŕĐàţà
éšçàþîñĝ) šö îţ ļàñđš đîŕéçţļý îñšîđé `<ḿ:ţ>…</ḿ:ţ>`. Àñ üñţŕàñšļàţéđ šþàñ
ţĥéŕéƒöŕé ŕéþŕöđüçéš ţĥé öŕîĝîñàļ ƃýţéš éẋàçţļý; à ţŕàñšļàţéđ öñé šþļîçéš ţĥé
ţŕàñšļàţîöñ îñ þļàçé ŵĥîļé ţĥé šüŕŕöüñđîñĝ ḿàţĥ îš ŕéþļàýéđ ƃýţé-ƒöŕ-ƃýţé. Ţĥé
çŕöšš-ƒöŕḿàţ ŵŕîţéŕš (ḿàŕķđöŵñ, đöçļàñĝ) **šķîþ** `öḿḿļ-ñöŕ` ƃļöçķš — ţĥé þŕöšé
àļŕéàđý ŕîđéš îñšîđé ţĥé ƒöŕḿüļà'š ĻàŢéẊ çàŕŕîéŕ — šö ţĥé šþàñš àŕé ţŕàñšļàţéđ ƒöŕ
ţĥé ĐÖÇẊ ŕöüñđ-ţŕîþ ŵîţĥöüţ ƃéîñĝ đüþļîçàţéđ öñ éẋþöŕţ. ▒

## ▒ ĤŢḾĻ ţöķéñîžéŕ ŕéàđéŕ (`çöŕé/ƒöŕḿàţš/ĥţḿļ/ţöķéñŕéàđéŕ.ĝö`) ▒

▒ Šîñĝļé-þàšš ŕéàđéŕ üšîñĝ Ĝö'š `ĥţḿļ.Ţöķéñîžéŕ` (ƒŕöḿ `ĝöļàñĝ.öŕĝ/ẋ/ñéţ/ĥţḿļ`).
Ñö `ĥţḿļ.Þàŕšé()`, ñö ĐÖḾ ţŕéé, ñö þŕé-šçàñ þàšš. Ŵŕîţéš šķéļéţöñ éñţŕîéš àš
îţ þŕöçéššéš ţöķéñš. ▒

### ▒ Éļéḿéñţ çļàššîƒîçàţîöñ ▒

▒ Ŵĥéñ ţĥé ţöķéñîžéŕ éñţéŕš à ƃļöçķ-ļéṽéļ éļéḿéñţ, îţ ñééđš ţö ķñöŵ ŵĥéţĥéŕ ţĥé
éļéḿéñţ îš à **çöñţàîñéŕ** (ĥàš ƃļöçķ-ļéṽéļ çĥîļđŕéñ) öŕ à **ļéàƒ ƃļöçķ**
(çöñţàîñš öñļý îñļîñé çöñţéñţ). Îñšţéàđ öƒ à þŕé-šçàñ þàšš öṽéŕ ţĥé éñţîŕé
đöçüḿéñţ, ţĥé ŕéàđéŕ **ƒöŕŵàŕđ-šçàñš** ƒŕöḿ ţĥé çüŕŕéñţ þöšîţîöñ ţĥŕöüĝĥ ţĥé
éļéḿéñţ'š ƃüƒƒéŕéđ çöñţéñţ: ▒

- ▒ Îƒ àñý đîŕéçţ çĥîļđ îš à ƃļöçķ-ļéṽéļ šţàŕţ ţàĝ → **çöñţàîñéŕ** (ḿîẋéđ
  çöñţéñţ ḿöđé — ţĥé éļéḿéñţ'š šţàŕţ/éñđ ţàĝš ĝö ţö šķéļéţöñ, çĥîļđŕéñ àŕé
  þŕöçéššéđ ŕéçüŕšîṽéļý) ▒
- ▒ Îƒ ñö ƃļöçķ çĥîļđŕéñ ƒöüñđ ƃý ţĥé éñđ ţàĝ → **ļéàƒ ƃļöçķ** (çöñţéñţ îš
  éẋţŕàçţéđ àš à ţŕàñšļàţàƃļé ƃļöçķ ŵîţĥ îñļîñé šþàñš) ▒

▒ Ţĥé ƒöŕŵàŕđ šçàñ šķîþš îñļîñé éļéḿéñţ šüƃţŕééš (ţŕàçķîñĝ đéþţĥ) àñđ öñļý
çĥéçķš đîŕéçţ çĥîļđŕéñ. Àƒţéŕ çļàššîƒîçàţîöñ, ţĥé šçàññéŕ'š ƃüƒƒéŕéđ ţöķéñš
àŕé ŕéþļàýéđ ƒöŕ þŕöçéššîñĝ. ▒

### ▒ Ţöķéñ þŕöçéššîñĝ ▒

| Token type                                                   | Action                                                                                             |
| ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------- |
| Non-translatable element start (e.g., `<script>`, `<style>`) | Write raw bytes to skeleton, consume until close tag                                               |
| Block-level element start (container)                        | Write start tag to skeleton, process children recursively                                          |
| Block-level element start (leaf)                             | Extract translatable attributes as skeleton refs, buffer content, build a `[]Run` for the block    |
| Inline element start/end                                     | Part of leaf block content → becomes a paired-code run (`PcOpen`/`PcClose`)                        |
| Text token                                                   | Part of leaf block content → appended as a `TextRun`                                               |
| Comment                                                      | Written to skeleton (non-translatable)                                                             |
| Doctype                                                      | Written to skeleton                                                                                |

### ▒ Ţŕàñšļàţàƃļé àţţŕîƃüţéš ▒

▒ Ƒöŕ éļéḿéñţš ŵîţĥ ţŕàñšļàţàƃļé àţţŕîƃüţéš (é.ĝ., `ţîţļé`, `àļţ`, `çöñţéñţ`
öñ ḿéţà ţàĝš), ţĥé ŕéàđéŕ šþļîţš ţĥé ŕàŵ ţàĝ ƃýţéš àţ àţţŕîƃüţé ṽàļüé
ƃöüñđàŕîéš ţö çŕéàţé îñţéŕļéàṽéđ šķéļéţöñ ţéẋţ àñđ ŕéƒ éñţŕîéš: ▒

<StreamDiagram
  title={'<p title="Tooltip">'}
  items={[
    { kind: "skeleton.WriteText", detail: `'<p title="'`, role: "meta" },
    { kind: "skeleton.WriteRef", detail: "tu1", role: "block", note: 'block for "Tooltip"' },
    { kind: "skeleton.WriteText", detail: `'">'`, role: "meta" },
  ]}
/>

▒ Ţĥé `ƒîñđÀţţŕṼàļüéŔàñĝé` ƒüñçţîöñ ļöçàţéš ţĥé ƃýţé ŕàñĝé öƒ àñ àţţŕîƃüţé
ṽàļüé ŵîţĥîñ ţĥé ŕàŵ ţàĝ ƃýţéš ƃý šçàññîñĝ ƒöŕ `àţţŕĶéý=` ƒöļļöŵéđ ƃý à
ǫüöţé çĥàŕàçţéŕ. ▒

▒ `ļàñĝ` / `ẋḿļ:ļàñĝ` àţţŕîƃüţé ṽàļüéš àŕé ĥàñđļéđ ţĥé šàḿé ŵàý, ƃüţ šþļîçéđ àš
à ţýþéđ `ŠķéļéţöñĻàñĝ` (ƃýţé `2`) éñţŕý ŕàţĥéŕ ţĥàñ ṽéŕƃàţîḿ ţéẋţ
(`éẋţŕàçţĻàñĝƑŕöḿŢöķéñ`), šö ţĥé ŵŕîţéŕ çàñ ŕéţàŕĝéţ ţĥé đöçüḿéñţ ļàñĝüàĝé öñ
öüţþüţ îñšţéàđ öƒ éḿîţţîñĝ ţĥé šöüŕçé-ļöçàļé ṽàļüé (ḿîŕŕöŕš Öķàþî'š ĤŢḾĻ
ƒîļţéŕ). ▒

### ▒ Ŕüñ šéǫüéñçé ƃüîļđîñĝ ▒

▒ Ƒöŕ ļéàƒ ƃļöçķ éļéḿéñţš, ţöķéñš ƃéţŵééñ šţàŕţ àñđ éñđ ţàĝ àŕé çöļļéçţéđ àñđ
ƃüîļţ îñţö à `[]ḿöđéļ.Ŕüñ` (ṽîà ţĥé ĤŢḾĻ `ŕüñƂüîļđéŕ` —
`çöŕé/ƒöŕḿàţš/ĥţḿļ/ŕüñ_ƃüîļđéŕ.ĝö`): ▒

- ▒ Ţéẋţ ţöķéñš → àþþéñđ à `ŢéẋţŔüñ` (`ÀđđŢéẋţ`, ŵĥîçĥ çöàļéšçéš àđĵàçéñţ ţéẋţ) ▒
- ▒ Îñļîñé éļéḿéñţ öþéñ/çļöšé → à þàîŕéđ `ÞçÖþéñŔüñ` / `ÞçÇļöšéŔüñ` (šĥàŕîñĝ àñ
  `ÎĐ`) ŵîţĥ `Đàţà = šţŕîñĝ(ŕàŵ)` (þŕéšéŕṽéš öŕîĝîñàļ ǫüöţé šţýļé, àţţŕîƃüţé
  öŕđéŕ, ŵĥîţéšþàçé) ▒
- ▒ Šéļƒ-çļöšîñĝ îñļîñé → à `ÞļàçéĥöļđéŕŔüñ` ▒
- ▒ Çöḿḿéñţš ŵîţĥîñ îñļîñé çöñţéñţ → à `ÞļàçéĥöļđéŕŔüñ` ▒

### ▒ Ḿéḿöŕý þŕöƒîļé ▒

| Component             | Memory                           |
| --------------------- | -------------------------------- |
| Tokenizer             | ~4KB internal buffer (streaming) |
| Forward scan          | ~1–10 tokens replay buffer       |
| Run sequence building | ~1–10KB (one leaf block)         |
| Skeleton store        | Temp file on disk                |
| Pipeline              | Blocks only (~5% of document)    |
| **Peak per document** | **~100KB**                       |

▒ Çöḿþàŕéđ ţö ţĥé ĐÖḾ-ƃàšéđ àþþŕöàçĥ: ~4–20ḾƂ þéŕ đöçüḿéñţ (ţŵö ƒüļļ ĐÖḾ ţŕééš
ƒöŕ ŕéàđéŕ + ŵŕîţéŕ). ▒

## ▒ ĤŢḾĻ ŵŕîţéŕ šķéļéţöñ ḿöđé (`çöŕé/ƒöŕḿàţš/ĥţḿļ/ŵŕîţéŕ.ĝö`) ▒

▒ Ŵĥéñ à šķéļéţöñ šţöŕé îš àṽàîļàƃļé, ţĥé ŵŕîţéŕ ŕéàđš éñţŕîéš šéǫüéñţîàļļý àñđ
ƒîļļš îñ ƃļöçķ çöñţéñţ. Ñö ţöķéñîžéŕ, ñö ĐÖḾ, ñö šţàţé ḿàçĥîñé: ▒

```go
func (w *Writer) writeFromSkeleton(
    store *format.SkeletonStore,
    blocks map[string]*model.Block,
    sourceLocale model.LocaleID,
    needsLangRewrite bool,
) error {
    for {
        entry, err := store.Next()
        if errors.Is(err, io.EOF) { break }
        if err != nil { return err }
        switch entry.Type {
        case format.SkeletonText:
            if _, err := w.Output.Write(entry.Data); err != nil {
                return err
            }
        case format.SkeletonRef:
            if block, ok := blocks[string(entry.Data)]; ok {
                text := w.getBlockText(block)
                // (block-ref substitution + HTML encoding elided)
                if _, err := io.WriteString(w.Output, text); err != nil {
                    return err
                }
            }
        case format.SkeletonLang:
            // Retarget the document language: when the stored source-locale
            // lang matches, emit the writer's target locale; else verbatim.
            lang := string(entry.Data)
            if needsLangRewrite && sameLanguage(lang, sourceLocale.String()) {
                lang = w.Locale.String()
            }
            if _, err := io.WriteString(w.Output, lang); err != nil {
                return err
            }
        }
    }
    return nil
}
```

### ▒ Ŵŕîţéŕ ƒàļļƃàçķ çĥàîñ ▒

▒ Ţĥé ŵŕîţéŕ ţŕîéš ţĥŕéé ḿöđéš îñ öŕđéŕ: ▒

1. ▒ **Šķéļéţöñ šţöŕé** (ƃýţé-éẋàçţ, ~4ĶƂ ḿéḿöŕý) — àṽàîļàƃļé ŵĥéñ
   `ŠķéļéţöñŠţöŕéÇöñšüḿéŕ.ŠéţŠķéļéţöñŠţöŕé()` ŵàš çàļļéđ ▒
2. ▒ **Ŕé-þàŕšé öŕîĝîñàļ çöñţéñţ** — ŕé-þàŕšéš ţĥé öŕîĝîñàļ ĤŢḾĻ ŵîţĥ à ĐÖḾ
   ŵàļķéŕ, þàţçĥéš ţŕàñšļàţîöñš îñţö ţĥé ţŕéé, ŕéñđéŕš ƃàçķ. Ŕéǫüîŕéš
   `ÖŕîĝîñàļÇöñţéñţŠéţţéŕ.ŠéţÖŕîĝîñàļÇöñţéñţ()` öŕ
   `ŠöüŕçéÞàţĥŠéţţéŕ.ŠéţŠöüŕçéÞàţĥ()` ▒
3. ▒ **Ƃļöçķ-öñļý ƒàļļƃàçķ** — öüţþüţš öñļý ƃļöçķ ţéẋţ çöñţéñţ, ñö ĤŢḾĻ
   šţŕüçţüŕé. Ļàšţ ŕéšöŕţ ŵĥéñ ñö öŕîĝîñàļ çöñţéñţ îš àṽàîļàƃļé. ▒

## ▒ Ƒîļéš ▒

| File                                  | Role                                                    |
| ------------------------------------- | ------------------------------------------------------- |
| `core/format/skeleton.go`             | SkeletonStore type, binary format, interfaces           |
| `core/format/skeleton_test.go`        | Unit tests (roundtrip, empty skip, large data)          |
| `core/formats/html/tokenreader.go`    | Single-pass tokenizer reader                            |
| `core/formats/html/reader.go`         | Dispatch: skeleton store → tokenizer, else → DOM        |
| `core/formats/html/writer.go`         | Skeleton mode + re-parse fallback + block-only fallback |
| `core/formats/html/roundtrip_test.go` | Byte-exact, translation, and attribute roundtrip tests  |
| `core/flow/filerunner.go`             | Central skeleton store wiring in `RunFileWithReaderWriter()` (emitter/consumer check), shared by all flow file runs |
| `cli/flow.go`                         | `runSingleFile()` builds a `FileRunner`; skeleton wiring delegated to `FileRunner` |
| `cli/toolrun.go`                      | Skeleton store wiring in `processOneFile()`             |
| `kapi/cmd/kapi/mcp_tools.go`          | `executeFlow()`/`executeFlowWithTools()` build a `FileRunner`; skeleton wiring delegated to `FileRunner` |
