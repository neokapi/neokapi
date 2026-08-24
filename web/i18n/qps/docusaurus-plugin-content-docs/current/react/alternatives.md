---
sidebar_position: 12
title: neokapi-i18n vs. Alternatives
description: A comparison of neokapi-i18n with react-i18next, FormatJS (react-intl), LinguiJS, fbtee, and Paraglide — covering source identifiers, JSX wrapping, extraction, format, and runtime tradeoffs.
keywords: [react-i18next, FormatJS, react-intl, LinguiJS, fbtee, Paraglide, alternatives, i18n comparison, neokapi-i18n]
---

# ▒ Àļţéŕñàţîṽéš ▒

▒ À ǫüîçķ ŕéƒéŕéñçé ƒöŕ ţéàḿš àļŕéàđý üšîñĝ — öŕ éṽàļüàţîñĝ — àñöţĥéŕ Ŕéàçţ î18ñ ļîƃŕàŕý. Àļļ öƒ ţĥéšé àŕé šöļîđ þŕöĵéçţš; ţĥé đîƒƒéŕéñçéš ƃéļöŵ àŕé àƃöüţ ƒîţ, ñöţ ǫüàļîţý. ▒

## ▒ ŕéàçţ-î18ñéẋţ ▒

▒ Ţĥé îñçüḿƃéñţ. Üšéš đéṽéļöþéŕ-àüţĥöŕéđ ķéýš àñđ à `ţ(ķéý)` / `<Ţŕàñš>` ŕüñţîḿé. ▒

|                   | react-i18next                         | neokapi-i18n                                                  |
| ----------------- | ------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Developer-invented key (natural-language keys also supported) | Source text + own element tag                               |
| JSX wrapping      | `t("key")` or `<Trans i18nKey="...">` | Plain JSX                                                   |
| Extraction        | `i18next-cli` / `i18next-parser`, or manual | Plugin during normal build                            |
| Format            | JSON (nested or flat); XLIFF via external conversion | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Ships the i18next runtime (interpolation, plural resolution, resource store); dict loaded at runtime | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

▒ Ḿîĝŕàţîñĝ ƒŕöḿ ŕéàçţ-î18ñéẋţ ţýþîçàļļý ḿéàñš đŕöþþîñĝ ţĥé `ţ()` / `<Ţŕàñš>` ŵŕàþþéŕš àñđ ŕé-ŕüññîñĝ ţĥé éẋţŕàçţ àĝàîñšţ ţĥé ƃàŕé ĴŠẊ. Éẋîšţîñĝ ţŕàñšļàţîöñš çàñ ƃé ļöàđéđ àš-îš îƒ ýöü ķéý ţĥéḿ ƃý ţĥé šàḿé šöüŕçé ţéẋţ; öţĥéŕŵîšé îţ'š à öñé-ţîḿé ŕé-ţŕàñšļàţîöñ þàšš ţĥŕöüĝĥ ýöüŕ çöñţéñţ ḿéḿöŕý. ▒

## ▒ ƑöŕḿàţĴŠ (ŕéàçţ-îñţļ) ▒

▒ Đéṽéļöþéŕ-àüţĥöŕéđ ḿéššàĝé đéšçŕîþţöŕš ŵîţĥ ÎÇÜ ƒöŕḿàţţîñĝ ƃàķéđ îñ. ▒

|                   | FormatJS                                               | neokapi-i18n                                   |
| ----------------- | ------------------------------------------------------ | -------------------------------------------- |
| Source identifier | Developer-invented id (or auto-hash of the descriptor) | Source text + own element tag                |
| JSX wrapping      | `<FormattedMessage>` or `useIntl().formatMessage()`    | Plain JSX                                    |
| Plurals / select  | Raw ICU message strings                                | `<Plural>` / `<Select>` authoring components |
| Extraction        | `@formatjs/cli`                                        | Plugin during normal build                   |
| Runtime cost      | Ships `intl-messageformat` (ICU parser/formatter); can precompile to AST to drop the parser | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

▒ ƑöŕḿàţĴŠ'š ÎÇÜ-îñ-šöüŕçé àþþŕöàçĥ ĥàñđļéš çöḿþļéẋ ḿéššàĝé çöḿþöšîţîöñ ŵéļļ, ƃüţ ƒöŕçéš ţŕàñšļàţöŕš (àñđ đéṽéļöþéŕš) ţö ŵöŕķ îñ ÎÇÜ đîŕéçţļý. ñéöķàþî-î18ñ ķééþš ţĥé šöüŕçé ļööķîñĝ ļîķé Ŕéàçţ, ţĥéñ éḿîţš ţĥé çàñöñîçàļ ÎÇÜ ţéḿþļàţé ƒöŕ ţŕàñšļàţöŕš' ÇÀŢ ţööļš đöŵñšţŕéàḿ. ▒

## ▒ Ļîñĝüî ▒

▒ Ţĥé çļöšéšţ îñ þĥîļöšöþĥý — Ļîñĝüî üšéš ḿàçŕöš (`<Ţŕàñš>`, `ţ` ţàĝĝéđ ţéḿþļàţéš) ţö ŕéŵŕîţé šöüŕçé ţéẋţ îñţö ĥàšĥéđ-ķéý ŕüñţîḿé ļööķüþš àţ ƃüîļđ ţîḿé. ▒

|                   | Lingui                                   | neokapi-i18n                                                  |
| ----------------- | ---------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Source text (Babel macro; experimental SWC plugin) | Source text + own element tag (via SWC plugin)              |
| JSX wrapping      | `<Trans>Hello</Trans>`, `t\`...\`` macro | Plain JSX                                                   |
| Extraction        | `lingui extract`                         | Plugin during normal build                                  |
| Format            | PO (default), JSON, CSV                   | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Small `@lingui/core` runtime + compiled catalogs; dict lookup | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

▒ Ļîñĝüî àñđ ñéöķàþî-î18ñ àĝŕéé öñ "šöüŕçé ţéẋţ àš ķéý". Ţĥé çöŕé đîƒƒéŕéñçé: Ļîñĝüî àšķš ýöü ţö öþţ éṽéŕý šţŕîñĝ îñţö ţĥé ḿàçŕö (`<Ţŕàñš>`, `` ţ`...` ``); ñéöķàþî-î18ñ öþţš îñ ƃý đéƒàüļţ. `ţ()` îñ ñéöķàþî-î18ñ îš à šḿàļļ éšçàþé ĥàţçĥ ƒöŕ ñöñ-ĴŠẊ šţŕîñĝš, ñöţ ţĥé ñöŕḿàļ àüţĥöŕîñĝ þàţţéŕñ. ▒

## ▒ ƒƃţéé ▒

▒ Ţĥé ḿöđéŕñ çöñţîñüàţîöñ öƒ Ḿéţà'š `ƒƃţ` (Ḿéţà àŕçĥîṽéđ `ƒƃţ` îñ ļàţé 2024). ƒƃţéé ŕéƃüîļđš îţ ƒöŕ ŢýþéŠçŕîþţ, Ŕéàçţ 19, ÉŠḾ, àñđ Ṽîţé / Ñéẋţ.ĵš ŵîţĥ ƃöţĥ Ƃàƃéļ àñđ ŠŴÇ ţŕàñšƒöŕḿš, ŵĥîļé ķééþîñĝ ƒƃţ'š àüţĥöŕîñĝ ḿöđéļ: éṽéŕý ţŕàñšļàţàƃļé šţŕîñĝ îš ŵŕàþþéđ îñ àñ éẋþļîçîţ `<ƒƃţ>` ḿàŕķéŕ, àñđ ţĥé šöüŕçé ţéẋţ îš ţĥé ķéý. ▒

|                   | fbtee                                                  | neokapi-i18n                                               |
| ----------------- | ------------------------------------------------------ | -------------------------------------------------------- |
| Source identifier | Source text + required `desc`                          | Source text + own element tag                           |
| JSX wrapping      | `<fbt desc="...">`, `fbt()` / `fbs()`                  | Plain JSX                                                |
| Plurals / gender  | `<fbt:plural>`, `<fbt:pronoun>`, `<fbt:enum>`          | `<Plural>` / `<Select>` authoring components             |
| Extraction        | `fbtee collect` → `prepare-translations` → `translate` | Plugin during normal build                               |
| Format            | JSON (`source_strings.json` + per-locale files)        | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Ships the fbt runtime to resolve params/plural/pronoun at render; translations loaded | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

▒ ƒƃţéé šĥàŕéš ñéöķàþî-î18ñ'š "šöüŕçé ţéẋţ àš ķéý" þĥîļöšöþĥý, ƃüţ ţàķéš ţĥé öþþöšîţé šţàñçé öñ ŵŕàþþîñĝ: îţ đéļîƃéŕàţéļý ŕéǫüîŕéš àñ `<ƒƃţ>` ḿàŕķéŕ (ŵîţĥ à `đéšç`) àŕöüñđ éṽéŕý ţŕàñšļàţàƃļé šţŕîñĝ šö ţĥé Ƃàƃéļ / ŠŴÇ çöḿþîļéŕ àñđ ÉŠĻîñţ þļüĝîñ çàñ šţàţîçàļļý àñàļýšé, ţýþé-çĥéçķ, àñđ éẋţŕàçţ îţ. Ţĥàţ ƃüýš çöḿþîļé-ţîḿé ĝüàŕàñţééš àñđ đéçļàŕàţîṽé îñļîñé þļüŕàļ / ĝéñđéŕ ĥàñđļîñĝ, àţ ţĥé çöšţ öƒ ŵŕàþþîñĝ çéŕéḿöñý öñ éṽéŕý šţŕîñĝ — ţĥé šàḿé ŵŕàþþîñĝ ţàẋ ñéöķàþî-î18ñ ŕéḿöṽéš ƃý éẋţŕàçţîñĝ þļàîñ ĴŠẊ àüţöḿàţîçàļļý. ▒

## ▒ Þàŕàĝļîđé (Îñļàñĝ) ▒

▒ Ţýþéđ, þéŕ-ḿéššàĝé ƒüñçţîöñš ĝéñéŕàţéđ àţ ƃüîļđ ţîḿé. À ḿéššàĝé `ŵéļçöḿé` ƃéçöḿéš `ḿ.ŵéļçöḿé()`. ▒

|                   | Paraglide                                            | neokapi-i18n                       |
| ----------------- | ---------------------------------------------------- | -------------------------------- |
| Source identifier | Developer-invented message id                        | Source text + own element tag |
| JSX wrapping      | Generated function call (`m.welcome()`)              | Plain JSX                        |
| Tree-shakeability | Every message is a function — excellent tree-shaking | Dict lookup — dict is one object |
| Runtime cost      | Minimal runtime; tree-shaken per-message functions, so unused messages cost ~0 | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

▒ Þàŕàĝļîđé'š ţýþéđ-ƒüñçţîöñ ḿöđéļ ĝîṽéš šţŕöñĝ ŕéƒàçţöŕîñĝ šüþþöŕţ ƃüţ ŕéǫüîŕéš ţĥé îđš-àš-ƒüñçţîöñ-ñàḿéš ḿöđéļ. ñéöķàþî-î18ñ îš šöüŕçé-ţéẋţ-àš-ķéý; ţĥé ţŵö çàñ çöéẋîšţ îñ à çöđéƃàšé îƒ ñééđéđ, ƃüţ üšüàļļý ýöü þîçķ öñé. ▒

## ▒ Ŵĥéŕé ñéöķàþî-î18ñ îš üñüšüàļ ▒

▒ Ţŵö þŕöþéŕţîéš àŕé ŵöŕţĥ çàļļîñĝ öüţ ƃéçàüšé ţĥéý ƒöļļöŵ ƒŕöḿ çĥöîçéš ţĥé ţàƃļé àƃöṽé đöéšñ'ţ çàþţüŕé: ▒

- ▒ **Ķéýš šüŕṽîṽé ŕéƒàçţöŕîñĝ.** Ţĥé ķéý îš đéŕîṽéđ ƒŕöḿ ţĥé šöüŕçé ţéẋţ *àñđ ţĥé éļéḿéñţ'š öŵñ ţàĝ*, àñđ đéļîƃéŕàţéļý ñöţ ƒŕöḿ îţš àñçéšţöŕš. Ŵŕàþþîñĝ à šéçţîöñ îñ à ñéŵ `<đîṽ>`, ḿöṽîñĝ à þàŕàĝŕàþĥ îñţö à `<Çàŕđ>`, ŕéšţŕüçţüŕîñĝ ţĥé þàĝé àŕöüñđ îţ — ñöñé öƒ ţĥéšé çĥàñĝé à ķéý, šö ñöñé öƒ ţĥéḿ öŕþĥàñ à ţŕàñšļàţîöñ. Ţĥé éļéḿéñţ îš šţîļļ éñöüĝĥ ţö ķééþ à ƃüţţöñ'š "Öþéñ" đîšţîñçţ ƒŕöḿ à ḿéñü îţéḿ'š; ŵĥéŕé îţ îšñ'ţ, ýöü đîšàḿƃîĝüàţé éẋþļîçîţļý ŵîţĥ à ñöţé. ▒
- ▒ **[Ŕéṽîéŵ ĥàþþéñš öñ ţĥé ŕüññîñĝ àþþ](./in-context-review).** ÀĻŢ+çļîçķ à šţŕîñĝ ţö šéé îţš šöüŕçé àñđ éđîţ îţš ţàŕĝéţ, ŵîţĥ ţéŕḿîñöļöĝý àñđ ǪÀ ƒîñđîñĝš þàîñţéđ öñţö ţĥé ļîṽé ţéẋţ, àñđ ţĥé éđîţ ŵŕîţţéñ šţŕàîĝĥţ ƃàçķ ţö ţĥé `.ķƃƒ.ĵšöñ` àš à ĝîţ đîƒƒ. Ŕéṽîéŵ ñééđš ñö àççöüñţ àñđ ñö ñéţŵöŕķ — ţĥé šţŕîñĝš àŕé ƒîļéš îñ ýöüŕ ŕéþöšîţöŕý, šö à ŕéṽîéŵéŕ ŵîţĥ à çĥéçķöüţ îš à ŕéṽîéŵéŕ. ▒

## ▒ Ŵĥîçĥ ţö þîçķ ▒

- ▒ **Ýöü ŵàñţ žéŕö-ŵŕàþþéŕ éŕĝöñöḿîçš àñđ ýöüŕ šţŕîñĝš ḿöšţļý ļîṽé îñ ĴŠẊ** → ñéöķàþî-î18ñ. ▒
- ▒ **Ýöü ŵàñţ ţýþéđ ḿéššàĝé ƒüñçţîöñš ţĥàţ ţŕéé-šĥàķé ţö ţĥé ḿéššàĝéš ýöü üšé** → Þàŕàĝļîđé. ▒
- ▒ **Ýöü'ŕé đééþļý îñṽéšţéđ îñ ÎÇÜ-àš-šöüŕçé** → ƑöŕḿàţĴŠ. ▒
- ▒ **Ýöü ŵàñţ éẋþļîçîţ, çöḿþîļé-ţîḿé-çĥéçķéđ îñļîñé ḿàŕķéŕš ŵîţĥ đéçļàŕàţîṽé þļüŕàļ/ĝéñđéŕ** → ƒƃţéé. ▒
- ▒ **Ýöü ĥàṽé à ļàŕĝé éẋîšţîñĝ ŕéàçţ-î18ñéẋţ çöđéƃàšé** → šţàý ŵîţĥ ŕéàçţ-î18ñéẋţ üñļéšš ýöü'ŕé đöîñĝ à ŕéŵŕîţé àñýŵàý. ▒

## ▒ Ŵĥéñ ñéöķàþî-î18ñ îšñ'ţ ţĥé ŕîĝĥţ ƒîţ ▒

▒ Šéé ţĥé šàḿé šéçţîöñ îñ ţĥé [Îñţŕöđüçţîöñ](./introduction#when-neokapi-i18n-isnt-the-right-fit). ▒
