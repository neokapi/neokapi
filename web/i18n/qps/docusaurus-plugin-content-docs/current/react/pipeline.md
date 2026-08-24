---
sidebar_position: 7
title: Extract, Translate, Compile Pipeline
description: The three-phase neokapi-i18n pipeline — extract JSX to a KBF archive, translate it with kapi (AI, MT, or content memory), compile locales back into runtime JSON. Includes an optional split phase for code-split apps.
keywords: [extract, translate, compile, KBF, neokapi-i18n pipeline, code splitting, translation pipeline]
---

import { PhaseFlow } from "@neokapi/docs-shared";

# ▒ Ţĥé éẋţŕàçţ → ţŕàñšļàţé → çöḿþîļé þîþéļîñé ▒

▒ Ţĥŕéé þĥàšéš, öñé çöñţŕàçţ: ţĥé ĶƂƑ đîŕéçţöŕý àŕçĥîṽé. À ƒöüŕţĥ öþţîöñàļ þĥàšé — **šþļîţ** — šļîçéš ţĥé çöḿþîļéđ öüţþüţ àļöñĝ ƃüñđļéŕ çĥüñķ ļîñéš šö çöđé-šþļîţ àþþš çàñ ļàžý-ļöàđ ţŕàñšļàţîöñš þéŕ ŕöüţé. ▒

<PhaseFlow
  nodes={[
    { label: "Your source code" },
    {
      label: "i18n/",
      sub: "KBF archive",
      role: "io",
      edge: "neokapi-i18n extract",
      loop: ["kapi translate / pseudo-translate / qa / review", "accumulate target locales in place"],
    },
    {
      label: "public/translations/{locale}.json",
      sub: "loaded at runtime by your app",
      edge: "neokapi-i18n compile",
    },
    {
      label: "dist/translations/{locale}/{chunk}.json",
      sub: "lazy-loaded per route",
      edge: "neokapi-i18n split (optional)",
    },
  ]}
/>

▒ Ţĥé šàḿé `î18ñ/` îš ţĥé šöüŕçé-öƒ-ţŕüţĥ àŕţîƒàçţ ţĥŕöüĝĥ ţĥé ŵĥöļé ŕöüñđ-ţŕîþ. Ţŕàñšļàţîöñ ţööļš ŕéàđ îţ, àþþéñđ ţĥé ţàŕĝéţ ļöçàļé ţĥéý'ŕé þŕöđüçîñĝ, àñđ ŵŕîţé ƃàçķ ţö ţĥé šàḿé ƒîļé — šö ýöü àççüḿüļàţé ļöçàļéš ŕàţĥéŕ ţĥàñ ĵüĝĝļîñĝ þéŕ-ŕüñ öüţþüţ ƒîļéš. Öñé ƒîļé îñ ţĥé ŕéþö, öñé ƒîļé ţö šĥîþ ţö ţŕàñšļàţöŕš, öñé ƒîļé ţö çöḿþîļé. ▒

▒ Éàçĥ þĥàšé ĥàš à šîñĝļé ţööļ; ñöñé öƒ ţĥéḿ àŕé çöüþļéđ ţö ţĥé öţĥéŕš. Ýöü çàñ šŵàþ öüţ ţĥé ţŕàñšļàţöŕ šţéþ ƒöŕ àñý þŕöçéšš ţĥàţ þŕéšéŕṽéš ţĥé ĶƂƑ çöñţŕàçţ — ĥüḿàñ ţŕàñšļàţöŕš ŵöŕķîñĝ îñ à ÇÀŢ ţööļ, ÀÎ ţŕàñšļàţîöñ, þŕé-éẋîšţîñĝ ŢḾŠ. ▒

## ▒ Þĥàšé 0: éẋþļàîñ (öþţîöñàļ) ▒

▒ Ƃéƒöŕé ýöü éẋţŕàçţ àñýţĥîñĝ, ýöü çàñ àšķ ţĥé éẋţŕàçţöŕ ŵĥàţ îţ *ŵöüļđ* đö àñđ ŵĥý: ▒

```bash
vp neokapi-i18n explain src/components/Settings.tsx
```

```text
L3    <div>          [container] skipped — has block-level children (they extract separately)
L4    <h1>           [translatable] extracted  hash=cYEMc2v3JVx
L6    <code>         [non-translatable] skipped — classified non-translatable
L7    <input>        [container] skipped — no translator-editable text
        ↳ placeholder [attribute] extracted  hash=i42kuGUFbb4
```

▒ Éàçĥ ļîñé îš ţĥé éļéḿéñţ'š Ŵ3Ç ÎŢŠ çļàššîƒîçàţîöñ, ţĥé ĝàţé ţĥàţ đéçîđéđ îţš ƒàţé, àñđ ţĥé ĥàšĥ îţ ŕéçéîṽéđ. "Žéŕö-çöñƒîĝ éẋţŕàçţîöñ" îš öñļý ţŕüšţŵöŕţĥý îƒ ýöü çàñ àüđîţ îţ; ţĥîš îš ţĥé àüđîţ. Àđđ `--éẋţŕàçţéđ` ţö ļîšţ öñļý ŵĥàţ ḿàđé ţĥé çàţàļöĝ. ▒

## ▒ Þĥàšé 1: éẋţŕàçţ ▒

▒ Ţĥé éẋţŕàçţöŕ ŵàļķš éṽéŕý `.ĵšẋ` / `.ţšẋ` ƒîļé îñ ýöüŕ þŕöĵéçţ àñđ þŕöđüçéš ţŕàñšļàţàƃļé ƃļöçķš. Ţŵö öüţþüţ ḿöđéš: ▒

- ▒ **Đéƒàüļţ** — þéŕ-ƒîļé `.ķƃƒ.ĵšöñ` üñđéŕ `--öüţ` (đéƒàüļţ `î18ñ/`). Ĥüḿàñ-ŕéàđàƃļé, ĝîţ-đîƒƒàƃļé. ▒
- ▒ **`--šţŕéàḿ`** — ÑĐĴŠÖÑ ƃļöçķ ŕéçöŕđš öñ šţđöüţ. Ƒîļé đîšçöṽéŕý ĥàþþéñš ṽîà `--šŕç` ĝļöƃ ŵĥéñ šţđîñ îš à ţéŕḿîñàļ; ķàþî'š éẋéç ƒöŕḿàţ çàñ þîþé ÑÜĻ-šéþàŕàţéđ þàţĥš ţö šţđîñ ƒöŕ ƃàţçĥ-çöñţŕöļļéđ éẋţŕàçţîöñ. ▒

```bash
# Default: write .kbf.json files for inspection / commit.
vp neokapi-i18n extract \
  --src "src/**/*.{tsx,jsx}" \
  --out i18n \
  --source-locale en \
  --target-locale fr \
  --target-locale de \
  --target-locale ja

# Or stream NDJSON blocks to stdout for piping:
vp neokapi-i18n extract --stream > i18n/blocks.ndjson
```

▒ Ƒļàĝš: ▒

| Flag              | Default              | Purpose                                                     |
| ----------------- | -------------------- | ----------------------------------------------------------- |
| `--src`           | `src/**/*.{tsx,jsx}` | Glob of source files to scan.                               |
| `--out`           | `i18n`               | Output directory for `.kbf.json` files.                          |
| `--stream`        | off                  | Emit NDJSON blocks on stdout instead of writing `.kbf.json`.     |
| `--ignore`        | —                    | Glob to exclude (repeatable) — fixtures, stories, tests.    |
| `--strict`        | off                  | Exit non-zero if any warning was recorded (CI enforcement). |
| `--config`        | —                    | Path to a JSON config file (componentMap, rules).           |
| `--project`       | `app`                | Project id stamped into the file's `project` field.         |
| `--source-locale` | `en`                 | Source locale in file metadata.                             |
| `--target-locale` | —                    | Declared target locale (repeatable).                        |

▒ Ţĥé éẋţŕàçţöŕ àļšö **þŕîñţš ŵàŕñîñĝš** ƒöŕ üñḿàþþéđ Ŕéàçţ çöḿþöñéñţš, šö ýöü ķñöŵ ŵĥîçĥ öñéš ţö àđđ ţö `çöḿþöñéñţḾàþ` ƒöŕ ĥàšĥ šţàƃîļîţý: ▒

```text
Scanning 186 files...
[neokapi] src/components/Settings.tsx:19: <TabsTrigger> is an unmapped component with translatable text — extracted. Add a componentMap entry: { TabsTrigger: 'button' }.
Extracted 1007 blocks from 186 files → i18n/
```

▒ Ŵîŕé îţ îñţö ýöüŕ þàçķàĝé šçŕîþţš àñđ ÇÎ: ▒

```json title="package.json"
{
  "scripts": {
    "extract": "vp neokapi-i18n extract",
    "extract:ci": "vp neokapi-i18n extract --strict",
    "pack": "vp neokapi-i18n extract --stream > i18n/blocks.ndjson"
  }
}
```

▒ Ƒöŕ ƒüļļ àüţĥöŕîñĝ-ţîḿé çöṽéŕàĝé, þàîŕ ţĥîš ŵîţĥ [`@ñéöķàþî/î18ñ-ŕéàçţ-ļîñţ`](./linting) — éđîţöŕ šǫüîĝĝļîéš ƒöŕ `ţ(ṽàŕîàƃļé)`, `<îḿĝ àļţ={'Logo ' + x} />`, àñđ ţĥé öţĥéŕ þàţţéŕñš ţĥé ƃüîļđ-ţîḿé ţŕàñšƒöŕḿ çàñ'ţ çàţçĥ. ▒

### ▒ Ŵĥàţ'š îñ ţĥé ĶƂƑ đîŕéçţöŕý ▒

▒ À đîŕéçţöŕý öƒ þéŕ-ƒîļé `.ķƃƒ.ĵšöñ` đöçüḿéñţš, ḿîŕŕöŕîñĝ ýöüŕ šöüŕçé ţŕéé
(é.ĝ. `šŕç/Àþþ.ţšẋ` → `î18ñ/šŕç/Àþþ.ķƃƒ.ĵšöñ`). Éàçĥ öñé îš à šéļƒ-çöñţàîñéđ ĶƂƑ
`Ƒîļé` çàŕŕýîñĝ: ▒

- ▒ `þŕöĵéçţ` — îđ, šöüŕçé ļöçàļé, đéçļàŕéđ ţàŕĝéţ ļöçàļéš. ▒
- ▒ `đöçüḿéñţš` — öñé đöçüḿéñţ ƒöŕ ţĥé šöüŕçé ƒîļé, ĥöļđîñĝ îţš `Ƃļöçķ`š. ▒
- ▒ Öþţîöñàļ ţàŕĝéţš / šķéļéţöñ / àññöţàţîöñ öṽéŕļàýš (àđđéđ ƃý ţŕàñšļàţöŕš). ▒

▒ Šéé [Ç-01](/contribute/architecture/context/c-01-project-model) ƒöŕ ţĥé ƒüļļ šçĥéḿà. ▒

### ▒ Öñé ƃļöçķ þéŕ ▒

- ▒ Ţŕàñšļàţàƃļé ĴŠẊ éļéḿéñţ (`<ĥ1>`, `<þ>`, `<ƃüţţöñ>`, àüţö-þŕöḿöţéđ `<đîṽ>`, üñḿàþþéđ çöḿþöñéñţš). ▒
- ▒ Ţŕàñšļàţàƃļé àţţŕîƃüţé (`ţîţļé`, `þļàçéĥöļđéŕ`, `àļţ`, `àŕîà-ļàƃéļ`, …). ▒
- ▒ Üšéŕ-ƒàçîñĝ `ţ(...)` çàļļ. ▒
- ▒ `<Þļüŕàļ>` / `<Šéļéçţ>` çöñšţŕüçţ. ▒

▒ Éàçĥ ƃļöçķ çàŕŕîéš: ▒

- ▒ `ĥàšĥ` — šţàƃļé îđ çöḿþüţéđ ƒŕöḿ ţĥé šöüŕçé ţéẋţ + ţĥé éļéḿéñţ'š öŵñ ţàĝ. ▒
- ▒ `šöüŕçé` — ţýþéđ ŕüñš (ţéẋţ, þļàçéĥöļđéŕš, îñļîñé éļéḿéñţ ţöķéñš, þļüŕàļ/šéļéçţ ŵŕàþþéŕš). ▒
- ▒ `þļàçéĥöļđéŕš` — ḿéţàđàţà àƃöüţ éàçĥ `{name}` / `{=mN}` îñ ţĥé šöüŕçé. ▒
- ▒ `þŕöþéŕţîéš` — ƒîļé + ļîñé + çöḿþöñéñţ ñàḿé + `éļéḿéñţ` (ţĥé ŕéšöļṽéđ ţàĝ) + öþţîöñàļ ţŕàñšļàţöŕ ñöţé. ▒

## ▒ Þĥàšé 2: ţŕàñšļàţé ▒

▒ Ţĥé `.ķƃƒ.ĵšöñ` îš ţĥé ţŕàñšļàţöŕ'š đéļîṽéŕàƃļé. Ţĥŕéé çöḿḿöñ þàţĥš: ▒

### ▒ Þàţĥ À: ÀÎ ţŕàñšļàţîöñ ▒

▒ Ŕüñ à ƒüļļ ţŕàñšļàţîöñ þàšš ŵîţĥ `ķàþî ţŕàñšļàţé`: ▒

```bash
kapi translate i18n/ --target-lang fr
kapi translate i18n/ --target-lang de
kapi translate i18n/ --target-lang ja
```

▒ Éàçĥ ŕüñ **àççüḿüļàţéš** à ţàŕĝéţ ļöçàļé îñţö ţĥé šàḿé `.ķƃƒ.ĵšöñ`. Ţĥé ŵŕîţéŕ îš ļöçàļé-àđđîţîṽé ƃý đéšîĝñ — éẋîšţîñĝ ţàŕĝéţš šţàý þüţ, ţĥé ŕéǫüéšţéđ ļöçàļé îš àđđéđ öŕ üþđàţéđ îñ þļàçé. Ñö `-ö` ñééđéđ üñļéšš ýöü ŵàñţ ţö ŕéđîŕéçţ öüţþüţ. ▒

▒ `ķàþî` šüþþöŕţš Àñţĥŕöþîç, ÖþéñÀÎ, Àžüŕé ÖþéñÀÎ, Ĝööĝļé Ĝéḿîñî, àñđ Öļļàḿà. Îţ þŕéšéŕṽéš þļàçéĥöļđéŕš, îñļîñé éļéḿéñţ ţöķéñš, àñđ þļüŕàļ/šéļéçţ šţŕüçţüŕé — ÀÎ þŕöṽîđéŕš ţĥàţ ḿàñĝļé ţĥéḿ àŕé àüţöḿàţîçàļļý ŵŕàþþéđ ŵîţĥ ŕéçöṽéŕý ļöĝîç. ▒

### ▒ Þàţĥ Ƃ: Þšéüđö-ţŕàñšļàţé ▒

▒ Ƒöŕ ÜÎ-ļàýöüţ ǪÀ, þšéüđö-ţŕàñšļàţîöñ ĝéñéŕàţéš ṽîšîƃļý-àļţéŕéđ šţŕîñĝš ŵîţĥöüţ àñý ŕéàļ ţŕàñšļàţîöñ: ▒

```bash
kapi pseudo-translate i18n/
```

▒ `Ŵéļçöḿé` ƃéçöḿéš `[Ŵéḷçőḿé]`, þàđđéđ àñđ àççéñţéđ. Ḿîššîñĝ ţŕàñšļàţîöñš šţàñđ öüţ îñšţàñţļý, àñđ šţŕîñĝš ţĥàţ ŵŕàþ ţöö àĝĝŕéššîṽéļý (öŕ ţöö ñàŕŕöŵļý) šĥöŵ üþ îñ ļàýöüţ ţéšţîñĝ. ▒

### ▒ Þàţĥ Ç: ÇÀŢ ţööļš / ŢḾŠ / ĥüḿàñ ţŕàñšļàţöŕš ▒

▒ Ţĥé `.ķƃƒ.ĵšöñ` îš ţĥé éẋçĥàñĝé ƒöŕḿàţ. À ţŕàñšļàţöŕ'š ŵöŕķƒļöŵ ḿîĝĥţ ƃé: ▒

1. ▒ Öþéñ ţĥé `î18ñ/` àŕçĥîṽé (öŕ ţĥé îñđîṽîđüàļ `.ķƃƒ.ĵšöñ` ƒîļéš) îñ ţĥéîŕ ÇÀŢ ţööļ. ▒
2. ▒ Ţŕàñšļàţé éṽéŕý ƃļöçķ, ļéṽéŕàĝîñĝ ţĥéîŕ éẋîšţîñĝ çöñţéñţ ḿéḿöŕý. ▒
3. ▒ Šàṽé ƃàçķ ţö ţĥé šàḿé `î18ñ/`. ▒

▒ Ţĥé çöñţéẋţ à ƃļöçķ çàŕŕîéš (îţš ƒîļé àñđ ļîñé, îţš éļéḿéñţ, ţĥé ţŕàñšļàţöŕ ñöţé, ţĥé îñļîñé éļéḿéñţ ţöķéñš) ŕéñđéŕš àš ŕîçĥ çöñţéẋţ îñ ḿöđéŕñ ÇÀŢ ţööļš. ▒

### ▒ Îñ-þļàçé đéƒàüļţ ṽš. éẋþļîçîţ ŕéđîŕéçţ ▒

▒ `ķàþî` ţööļ çöḿḿàñđš đéƒàüļţ ţö îñ-þļàçé ƒöŕ ĶƂƑ îñþüţš — `ķàþî þšéüđö-ţŕàñšļàţé î18ñ/` ŕéàđš àñđ ŵŕîţéš ţĥé šàḿé ƒîļéš, šîñçé ţĥé ĶƂƑ ŵŕîţéŕ îš ļöçàļé-àđđîţîṽé (îţ àđđš öŕ üþđàţéš ţĥé ŕéǫüéšţéđ ļöçàļé, ļéàṽîñĝ ţĥé öţĥéŕš îñţàçţ). Þàšš `-ö öţĥéŕ-đîŕ/` ţö ŕéđîŕéçţ ŵîţĥöüţ ţöüçĥîñĝ ţĥé öŕîĝîñàļš. ▒

▒ Ñöñ-ĶƂƑ ƒöŕḿàţš (ĴŠÖÑ, ẊĻÎƑƑ, …) àŕéñ'ţ ļöçàļé-àđđîţîṽé, šö ţĥéý ŵŕîţé à ñéŵ ƒîļé îñ à ļöçàļé-àŵàŕé ļöçàţîöñ: îƒ ţĥé îñþüţ þàţĥ çàŕŕîéš ţĥé šöüŕçé ļöçàļé îţ îš šŵàþþéđ ƒöŕ ţĥé ţàŕĝéţ (`šŕç/ļöçàļéš/éñ/àþþ.ĵšöñ → šŕç/ļöçàļéš/ƒŕ/àþþ.ĵšöñ`), öţĥéŕŵîšé ţĥé öüţþüţ ļàñđš üñđéŕ à `{lang}/` đîŕéçţöŕý ƃéšîđé ţĥé îñþüţ (`ḿéššàĝéš.ĵšöñ → ƒŕ/ḿéššàĝéš.ĵšöñ`). Üšé `-ö` ƒöŕ àñ éẋþļîçîţ þàţĥ öŕ ţéḿþļàţé, öŕ `--öüţþüţ-đîŕ ĐÎŔ` ţö ŕööţ öüţþüţš àţ `ĐÎŔ/{lang}/`. ▒

### ▒ Ļàýöüţ: öñé ţŕéé, öŕ à šüƃđîŕ þéŕ ļöçàļé ▒

▒ Ţŵö ļàýöüţš, ƃöţĥ çļéàñ: ▒

- ▒ **Ļöçàļé-àđđîţîṽé** — öñé `î18ñ/` ţŕéé ŵĥéŕé éàçĥ ƃļöçķ çàŕŕîéš éṽéŕý ţàŕĝéţ ļöçàļé, ƒîļļéđ îñ þļàçé (ţĥé đéƒàüļţ ƒöŕ `ķàþî ţŕàñšļàţé î18ñ/ --ţàŕĝéţ-ļàñĝ …`). Šîḿþļéšţ ţö ṽéŕšîöñ; àļļ ţŕàñšļàţîöñš šţàý ţöĝéţĥéŕ. ▒
- ▒ **Ŕéçîþé-đŕîṽéñ þéŕ-ļöçàļé ƒîļéš** — ţĥé šöüŕçé çàţàļöĝš ļîṽé üñđéŕ `î18ñ/šŕç/` àñđ ķàþî ŵŕîţéš à šéþàŕàţé ƒîļé þéŕ ļöçàļé üñđéŕ `î18ñ/{lang}/`, ḿàþþéđ ƃý à `ķàþî.ýàḿļ` çöñţéñţ éñţŕý (`þàţĥ: î18ñ/šŕç/**/*.ķƃƒ.ĵšöñ` → `ţàŕĝéţ: î18ñ/{lang}/{path}.ķƃƒ.ĵšöñ`). Ţĥîš îš ŵĥàţ `ķàþî îñîţ --ƒŕàḿéŵöŕķ ñéöķàþî-î18ñ` šçàƒƒöļđš. ▒

▒ Ƃöţĥ ķééþ éṽéŕýţĥîñĝ üñđéŕ öñé `î18ñ/` đîŕéçţöŕý. Ƃéçàüšé ţĥé šöüŕçé ļîṽéš üñđéŕ `î18ñ/šŕç/`, ţĥé šöüŕçé ĝļöƃ ñéṽéŕ ḿàţçĥéš ţĥé ĝéñéŕàţéđ `î18ñ/{lang}/` ţàŕĝéţš — šö ţĥéŕé îš ñö ñééđ ƒöŕ šîƃļîñĝ `î18ñ-<ļàñĝ>/` ţŕééš. Šéé [Ç-01](/contribute/architecture/context/c-01-project-model) ƒöŕ ţĥé þŕöĵéçţ ḿöđéļ àñđ [Đŕîṽé îţ ƒŕöḿ à þŕöĵéçţ](./translating-with-kapi#drive-it-from-a-project) ƒöŕ ţĥé ŕéçîþé. ▒

### ▒ Þŕöĵéçţ-đŕîṽéñ ƒļöŵ ŵîţĥ `ķàþî.ýàḿļ` ▒

▒ Îƒ ýöü àļŕéàđý üšé à [`ķàþî.ýàḿļ` þŕöĵéçţ ƒîļé](/contribute/architecture/context/c-01-project-model) ţö đéƒîñé ýöüŕ ŵöŕķƒļöŵ, đéçļàŕé éàçĥ àŕçĥîṽé-ƃàçķéđ çöļļéçţîöñ ŵîţĥ àñ `éẋéç` ƒöŕḿàţ þöîñţîñĝ àţ ñéöķàþî-î18ñ (öŕ àñý öţĥéŕ éẋţŕàçţöŕ): ▒

```yaml title="kapi.yaml"
version: v1
name: MyApp
defaults:
  source_language: en
  target_languages: [fr, de, ja]
collections:
  - name: ui
    # Block state lives in the project cache (gitignored, regenerable).
    content:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "vp neokapi-i18n extract --stream"
```

```bash
# 1. Extract — kapi runs the declared command for each collection,
#    streams NDJSON blocks into the collection's block store.
kapi extract -p kapi.yaml

# 2. Translate — run a composed flow over the project for each target language.
kapi run translate-qa -p kapi.yaml
```

▒ Ţĥé `çöḿḿàñđ` šţŕîñĝ þîçķš ţĥé þàçķàĝé ḿàñàĝéŕ — `ṽþ`, `þñþḿ`, `ñþḿ`, `ýàŕñ`, öŕ à đîŕéçţ ƃîñàŕý þàţĥ — šö ţĥé þŕöĵéçţ đéçļàŕéš îţš þŕéƒéŕéñçéš éẋþļîçîţļý ŵîţĥöüţ ķàþî ḿàķîñĝ àššüḿþţîöñš. `ķàþî ŕüñ` ţĥéñ éẋéçüţéš ţĥé ñàḿéđ [ƒļöŵ](/framework/flows) àĝàîñšţ ţĥé þŕöĵéçţ'š éẋţŕàçţéđ ƃļöçķš ƒöŕ éàçĥ ţàŕĝéţ ļàñĝüàĝé. ▒

### ▒ Šţàñđàļöñé þîþé (ñö `ķàþî.ýàḿļ`) ▒

▒ Ƒöŕ àđ-ĥöç þŕöĵéçţš, šķîþ `ķàþî.ýàḿļ` éñţîŕéļý àñđ çöḿþöšé ŵîţĥ Üñîẋ þîþéš: ▒

```bash
vp neokapi-i18n extract --stream > i18n/blocks.ndjson
kapi pseudo-translate i18n/
vp neokapi-i18n compile i18n/ --out public/translations
```

▒ Šàḿé üñđéŕļýîñĝ ŵîŕé ƒöŕḿàţ (ÑĐĴŠÖÑ öñ ţĥé éẋţŕàçţ šţàĝé, ĶƂƑ ƒŕöḿ ţĥéŕé öñ) — ţĥé đéçļàŕàţîṽé `ķàþî.ýàḿļ` ƒöŕḿ ĵüšţ ƒàçţöŕš ţĥé þîþé îñţö ţĥé þŕöĵéçţ ƒîļé. ▒

## ▒ Þĥàšé 3: çöḿþîļé ▒

▒ `ñéöķàþî-î18ñ çöḿþîļé` ŕéàđš ţĥé ţŕàñšļàţéđ `.ķƃƒ.ĵšöñ` àñđ éḿîţš öñé ĴŠÖÑ đîçţ þéŕ ļöçàļé: ▒

```bash
neokapi-i18n compile i18n/ \
  --out public/translations
```

▒ Öüţþüţ: ▒

```
Compiled 1007 entries → public/translations/fr.json
Compiled 1007 entries → public/translations/de.json
Compiled 1007 entries → public/translations/ja.json
```

▒ Éàçĥ ĴŠÖÑ ƒîļé îš à ƒļàţ `{hash: renderedText}` ḿàþ. Ţĥé ŕüñţîḿé `__ţ(ĥàšĥ, ƒàļļƃàçķ, þàŕàḿš)` ļööķš üþ ţĥé ĥàšĥ; ţĥé ŕéñđéŕéŕ þîçķš ţĥé þļüŕàļ / šéļéçţ ƒöŕḿ. ▒

## ▒ Þĥàšé 4: šþļîţ (öþţîöñàļ) ▒

▒ Ƒöŕ çöđé-šþļîţ àþþš, ţĥé çöḿþîļéđ `{locale}.ĵšöñ` îš öñé ƒîļé þéŕ ļöçàļé — ţĥé üšéŕ đöŵñļöàđš éṽéŕý šţŕîñĝ éṽéñ ƒöŕ ŕöüţéš ţĥéý ñéṽéŕ ṽîšîţ. Ţĥé þļüĝîñ + `ñéöķàþî-î18ñ šþļîţ` đîṽîđé ţĥàţ çàţàļöĝ àļöñĝ ƃüñđļéŕ çĥüñķ ƃöüñđàŕîéš šö éàçĥ çĥüñķ ļàñđš îţš öŵñ ţŕàñšļàţîöñ šüƃšéţ àļöñĝšîđé îţš ĴŠ. ▒

▒ Ţŵö îñþüţš: ▒

- ▒ **`ţŕàñšļàţîöñš-ḿàñîƒéšţ.ĵšöñ`** — éḿîţţéđ ŵĥéñ `ḿöđé: "ŕüñţîḿé"` ƃý Ṽîţé, Ŕöļļüþ, ŵéƃþàçķ, Ŕšþàçķ, àñđ éšƃüîļđ (éšƃüîļđ ñééđš `ḿéţàƒîļé: ţŕüé`). Ḿàþš éàçĥ öüţþüţ çĥüñķ ţö ţĥé šéţ öƒ ĥàšĥéš îţš ḿöđüļéš ŕéƒéŕéñçé. ▒
- ▒ **`þüƃļîç/ţŕàñšļàţîöñš/{locale}.ĵšöñ`** — ţĥé çöḿþîļéđ ḿàšţéŕ đîçţ ƒŕöḿ Þĥàšé 3. ▒

```bash
vite build                                       # emits dist/translations-manifest.json
neokapi-i18n compile i18n/ --out public/translations
neokapi-i18n split \
  --manifest dist/translations-manifest.json \
  --locales  public/translations \
  --out      dist/translations
```

▒ Öüţþüţ: ▒

```
dist/translations/
├── manifest.json                   ← copy of the chunk → hashes map
└── {locale}/
    ├── index.json                  ← hashes used by the main chunk
    ├── SettingsPage.json
    └── FlowEditor.json
```

▒ Ĥàšĥéš šĥàŕéđ àçŕöšš çĥüñķš àŕé đüþļîçàţéđ îñţö éàçĥ šüƃšéţ šö éṽéŕý çĥüñķ ƒîļé îš îñđéþéñđéñţļý ļöàđàƃļé. Ŕüñţîḿé ŵîŕîñĝ îš à öñé-ļîñé àđđîţîöñ ţö éàçĥ ļàžý ŕöüţé: ▒

```tsx
import { loadTranslationChunk } from "@neokapi/i18n-react/runtime";

const routes = [
  {
    path: "/settings",
    lazy: async () => {
      const [mod] = await Promise.all([
        import("./SettingsPage"),
        loadTranslationChunk(locale, `/translations/${locale}/SettingsPage.json`),
      ]);
      return { Component: mod.default };
    },
  },
];
```

▒ `ļöàđŢŕàñšļàţîöñÇĥüñķ` ḿéŕĝéš ţĥé šüƃšéţ îñţö ţĥé àçţîṽé đîçţ; çöñçüŕŕéñţ çàļļš ƒöŕ ţĥé šàḿé `(ļöçàļé, üŕļ)` šĥàŕé à šîñĝļé ƒéţçĥ. Ḿîššîñĝ ĥàšĥéš ƒàļļ ƃàçķ ţö ţĥé šöüŕçé ţéẋţ ƃàķéđ îñţö éàçĥ `__ţ` / `__ţẋ` çàļļ àţ ƃüîļđ ţîḿé — à ļàţé-àŕŕîṽîñĝ çĥüñķ ñéṽéŕ ƃŕéàķš ŕéñđéŕ. Šéé [Ŕüñţîḿé ḿöđé → Ļàžý ļöàđîñĝ þéŕ ŕöüţé](./modes#lazy-loading-per-route-code-splitting) ƒöŕ ţĥé ƒüļļ ŕüñţîḿé çöñţŕàçţ. ▒

▒ Àþþš ţĥàţ šĥîþ à šîñĝļé ƃüñđļé đöñ'ţ ñééđ ţĥîš þĥàšé àţ àļļ — ķééþ üšîñĝ `ļöàđŢŕàñšļàţîöñš(ļöçàļé, üŕļ)` àĝàîñšţ ţĥé çöḿþîļéđ ḿàšţéŕ đîçţ. ▒

## ▒ Ŕöüñđ-ţŕîþ îñ öñé đîàĝŕàḿ ▒

<PhaseFlow
  nodes={[
    { label: "src/App.tsx", sub: "<h1>Welcome</h1>" },
    {
      label: "i18n/ Block",
      sub: 'hash "aB3" · source + targets',
      edge: "neokapi-i18n extract (source only)",
      role: "io",
      loop: ["kapi translate --target-lang fr", "then de … (additive, in place)"],
    },
    {
      label: "public/translations/{locale}.json",
      sub: '{ "aB3": "Bienvenue" }',
      edge: "neokapi-i18n compile",
      role: "io",
      loop: ["loadTranslations(locale, url)", "single bundle → app renders"],
    },
    {
      label: "dist/translations/{locale}/",
      sub: "index.json + lazy chunks",
      edge: "neokapi-i18n split (optional)",
      role: "io",
    },
    {
      label: 'Your app renders "Bienvenue"',
      edge: "loadTranslationChunk per route",
      role: "io",
    },
  ]}
/>

## ▒ ÇÎ: ŕé-éẋţŕàçţ éṽéŕý ƃüîļđ, ƒàîļ öñ đŕîƒţ ▒

▒ Ţĥé éẋţŕàçţ îš đéţéŕḿîñîšţîç, šö ÇÎ çàñ üšé ţĥé àŕçĥîṽé ĥàšĥ àš à çöñţŕàçţ: ▒

```yaml title=".github/workflows/ci.yml"
- name: Extract translatable content
  run: npm run extract

- name: Fail if translators need to re-open the file
  run: |
    git diff --exit-code i18n/ || {
      echo "::error::i18n/ drifted. Re-extract locally and commit."
      exit 1
    }
```

▒ Ƒöŕ àþþš ŵîţĥ à ţŕàñšļàţîöñ ƃàçķéñđ, ýöü'đ îñšţéàđ þüšĥ ţĥé àŕçĥîṽé ţö ţĥàţ ƃàçķéñđ àñđ ŵàîţ ƒöŕ ţŕàñšļàţéđ đéļîṽéŕàƃļéš — ƃüţ ţĥé þŕîñçîþļé îš ţĥé šàḿé: éẋţŕàçţ öñ éṽéŕý çĥàñĝé, đöñ'ţ ļéţ ţŕàñšļàţîöñš đŕîƒţ ƒŕöḿ šöüŕçé. ▒

## ▒ Îñçŕéḿéñţàļ éẋţŕàçţš ▒

▒ Ţĥé éẋţŕàçţöŕ îš šţàţéļéšš — îţ àļŵàýš þŕöđüçéš ţĥé šàḿé `.ķƃƒ.ĵšöñ` ƒöŕ ţĥé šàḿé šöüŕçé + çöñƒîĝ. Ƒöŕ àñ îñçŕéḿéñţàļ þîþéļîñé (öñļý ţŕàñšļàţé ŵĥàţ çĥàñĝéđ), đîƒƒ ţŵö àŕçĥîṽéš öñ ţĥé ţŕàñšļàţîöñ šîđé. Éàçĥ ƃļöçķ'š ĥàšĥ ţéļļš ýöü ŵĥéţĥéŕ îţš šöüŕçé šĥîƒţéđ. ▒

## ▒ Ñéẋţ ▒

- ▒ [Ŕüñţîḿé ṽš. îñļîñé ḿöđéš](./modes) — šĥîþþîñĝ öñé ƃüñđļé ŵîţĥ ÖŢÀ đîçţš ṽš. öñé ƃüñđļé þéŕ ļöçàļé. ▒
- ▒ [Ţŕàñšļàţîñĝ ŵîţĥ ķàþî](./translating-with-kapi) — þšéüđö-ţŕàñšļàţîöñ, ÀÎ ţŕàñšļàţîöñ, ǪÀ. ▒
- ▒ [Çöñƒîĝüŕàţîöñ](./configuration) — çöḿþöñéñţḾàþ, ŕüļéš, Šţöŕýƃööķ, çüšţöḿ ŵàŕñîñĝ ĥàñđļéŕš. ▒
