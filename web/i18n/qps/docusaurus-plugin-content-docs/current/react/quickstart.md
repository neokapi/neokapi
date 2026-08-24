---
sidebar_position: 2
title: Quick start
description: Add neokapi-i18n to a Vite + React project in about five minutes — install the plugin, write plain JSX, run kapi pseudo-translate, and flip between locales from a toolbar.
keywords: [neokapi-i18n, quick start, Vite, React, install, pseudo-translate, i18n setup]
---

# ▒ Ǫüîçķ šţàŕţ ▒

▒ Àđđ ñéöķàþî-î18ñ ţö à Ṽîţé + Ŕéàçţ þŕöĵéçţ. ~5 ḿîñüţéš; ýöü'ļļ ƒîñîšĥ ŵîţĥ à ŕüññîñĝ àþþ ţĥàţ ƒļîþš ƃéţŵééñ Éñĝļîšĥ àñđ þšéüđö-Éñĝļîšĥ ƒŕöḿ à ţööļƃàŕ. ▒

## ▒ 1. Îñšţàļļ ▒

```bash
npm install -D @neokapi/i18n-react
```

▒ Ţĥé þàçķàĝé šĥîþš à ƃüîļđ þļüĝîñ (Ṽîţé, Ŕöļļüþ, ŵéƃþàçķ, Ŕšþàçķ, éšƃüîļđ), ţĥé `éẋţŕàçţ` / `çöḿþîļé` / `šþļîţ` / `éẋþļàîñ` ÇĻÎ šüƃçöḿḿàñđš, àñđ ţĥé ţîñý ŕüñţîḿé (~2 ķƂ). Ñö þééŕ đéþéñđéñçîéš ƃéýöñđ Ŕéàçţ 18+. ▒

▒ Ţĥé [`ķàþî` ÇĻÎ](/kapi/cli) îš ţĥé ţŕàñšļàţîöñ þîþéļîñé ţĥàţ þŕöđüçéš þšéüđö-ţŕàñšļàţîöñš ƒŕöḿ ţĥé ĶƂƑ đîŕéçţöŕý ñéöķàþî-î18ñ éẋţŕàçţš. Îñšţàļļ îţ ţöö: ▒

```bash
# macOS / Linux
brew install neokapi/tap/kapi-cli-beta

# Windows
winget install Neokapi.KapiCli
```

## ▒ 2. Àđđ ţĥé þļüĝîñ ţö `ṽîţé.çöñƒîĝ.ţš` ▒

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import neokapi from "@neokapi/i18n-react/vite";

export default defineConfig({
  plugins: [
    neokapi({ mode: "runtime" }), // ← add this
    react(),
  ],
});
```

▒ Ţŵö ḿöđéš àŕé àṽàîļàƃļé — þîçķ `ŕüñţîḿé` ƒöŕ ñöŵ: ▒

- ▒ `ŕüñţîḿé` — šĥîþ öñé ƃüñđļé; ļöàđ à ţŕàñšļàţîöñ đîçţ àţ ŕüñţîḿé ṽîà `ƒéţçĥ`. Ĝööđ ƒöŕ àþþš ţĥàţ šĥîþ ḿàñý ļöçàļéš ƒŕöḿ à ÇĐÑ. ▒
- ▒ `îñļîñé` — þŕöđüçé öñé ƃüñđļé þéŕ ļöçàļé ŵîţĥ ţŕàñšļàţîöñš þŕé-îñļîñéđ. Žéŕö ŕüñţîḿé ļööķüþ, ƒàšţéšţ ƒîŕšţ þàîñţ. ▒

## ▒ 3. Ŵŕîţé ĴŠẊ àš ýöü ñöŕḿàļļý ŵöüļđ ▒

```tsx title="src/App.tsx"
export default function App() {
  return (
    <main>
      <h1>Welcome to Acme</h1>
      <p>Ship your product in every language your users speak.</p>
      <button>Get started</button>
    </main>
  );
}
```

▒ Ñö `ţ(...)` çàļļš, ñö ķéýš. Ţĥé þļüĝîñ ŵàļķš ţĥé ĴŠẊ àţ ƃüîļđ ţîḿé àñđ ŕéŵŕîţéš éàçĥ ţŕàñšļàţàƃļé šîţé ţö à ĥàšĥ-ƃàšéđ ļööķüþ. ▒

## ▒ 4. Éẋţŕàçţ ţö à ĶƂƑ đîŕéçţöŕý ▒

▒ Ŵîŕé ţĥé éẋţŕàçţöŕ + þàçķ îñţö ýöüŕ þàçķàĝé šçŕîþţš: ▒

```json title="package.json"
{
  "scripts": {
    "extract": "vp neokapi-i18n extract",
    "compile": "vp neokapi-i18n compile i18n/ --out public/translations"
  }
}
```

▒ Ŕüñ éẋţŕàçţ: ▒

```bash
npm run extract
```

▒ Öüţþüţ: ▒

```
Scanning 1 files...
Extracted 3 blocks from 1 files → i18n/
```

▒ `î18ñ/` îš à đîŕéçţöŕý çàŕŕýîñĝ öñé `.ķƃƒ.ĵšöñ` đöçüḿéñţ þéŕ šöüŕçé ƒîļé, ḿîŕŕöŕîñĝ
ýöüŕ šöüŕçé ţŕéé (é.ĝ. `î18ñ/šŕç/Àþþ.ķƃƒ.ĵšöñ`). Ţĥé ţĥŕéé ƃļöçķš àŕé
"Ŵéļçöḿé ţö Àçḿé", ţĥé þàŕàĝŕàþĥ, àñđ "Ĝéţ šţàŕţéđ". Éàçĥ öñé îš þļàîñ ĴŠÖÑ, àñđ
ţĥé šüƒƒîẋ šàýš šö — ýöüŕ éđîţöŕ, `ĵǫ`, àñđ ĜîţĤüƃ ŕéàđ îţ ŵîţĥöüţ àñý šéţüþ, šö
îţ šţàýš ĥüḿàñ-ŕéàđàƃļé àñđ ĝîţ-đîƒƒàƃļé. ▒

## ▒ 5. Þšéüđö-ţŕàñšļàţé ŵîţĥ `ķàþî` ▒

▒ Þšéüđö-ţŕàñšļàţîöñ ĝéñéŕàţéš `[Ŵëļçöḿé ţö Âçḿé]`-šţýļé àççéñţéđ šţŕîñĝš ţĥàţ ḿàķé îţ öƃṽîöüš ŵĥàţ'š ƃééñ þîçķéđ üþ ƒöŕ ţŕàñšļàţîöñ — àñđ ŵĥîçĥ šţŕîñĝš àŕé šţîļļ Éñĝļîšĥ. Þéŕƒéçţ ƒîŕšţ þàšš. ▒

```bash
kapi pseudo-translate i18n/
```

## ▒ 6. Çöḿþîļé ţö à ŕüñţîḿé đîçţ ▒

▒ `ñéöķàþî-î18ñ çöḿþîļé` ţüŕñš ţĥé ţŕàñšļàţéđ ĶƂƑ îñţö à `{locale}.ĵšöñ` ƒîļé þéŕ ļöçàļé: ▒

```bash
npm run compile
```

▒ Öüţþüţ: ▒

```
Compiled 3 entries → public/translations/qps.json
```

▒ Ţĥé ĴŠÖÑ îš `{ "<hash>": "<flattened target text>" }`. ▒

## ▒ 7. Ļöàđ ţĥé ţŕàñšļàţîöñ àţ ŕüñţîḿé ▒

▒ Ţŵö ļîñéš îñ ýöüŕ àþþ ƃööţšţŕàþ: ▒

```tsx title="src/main.tsx"
import { loadTranslations } from "@neokapi/i18n-react/runtime";
import ReactDOM from "react-dom/client";
import App from "./App";

async function bootstrap() {
  await loadTranslations("qps", "/translations/qps.json").catch(() => {});
  ReactDOM.createRoot(document.getElementById("root")!).render(<App />);
}

void bootstrap();
```

▒ `ļöàđŢŕàñšļàţîöñš(ļöçàļé, üŕļ)` ƒéţçĥéš ţĥé đîçţ àñđ àçţîṽàţéš îţ. Àƒţéŕ îţ ŕéšöļṽéš, éṽéŕý ŕéñđéŕéđ `<ĥ1>Ŵéļçöḿé ţö Àçḿé</ĥ1>` ŕéñđéŕš àš `[Ŵëļçöḿé ţö Âçḿé]`. (Þàšš àñ àŕŕàý öƒ ÜŔĻš îñšţéàđ öƒ öñé ţö ƃüîļđ à [ƒàļļƃàçķ çĥàîñ](./modes#fallback-chain) — `þţ-ƂŔ` öṽéŕ `þţ`, šàý.) ▒

## ▒ 8. Àđđ à ļàñĝüàĝé šŵîţçĥéŕ (öþţîöñàļ) ▒

▒ À 10-ļîñé ļàñĝüàĝé þîçķéŕ ŵîŕéđ ţö `šéţŢŕàñšļàţîöñš` / `ļöàđŢŕàñšļàţîöñš`: ▒

```tsx
import { loadTranslations, setTranslations, useNeokapi } from "@neokapi/i18n-react/runtime";

export function LocaleSwitcher() {
  useNeokapi(); // subscribe so the component re-renders on locale change

  return (
    <select
      onChange={async (e) => {
        const value = e.target.value;
        if (value === "en") setTranslations("en", {});
        else await loadTranslations(value, `/translations/${value}.json`);
      }}
    >
      <option value="en">English</option>
      <option value="qps">Pseudo-English</option>
    </select>
  );
}
```

▒ `üšéÑéöķàþî()` ŵîŕéš ţĥé ŕööţ öƒ ýöüŕ ţŕéé îñţö ñéöķàþî-î18ñ'š ţŕàñšļàţîöñ šţöŕé šö à ļöçàļé çĥàñĝé ŕé-ŕéñđéŕš ţĥé ŵĥöļé šüƃšçŕîƃéđ šüƃţŕéé — ñö ñàṽîĝàţîöñ ŕéǫüîŕéđ. ▒

## ▒ Ŵĥàţ ĵüšţ ĥàþþéñéđ ▒

- ▒ **Žéŕö ŵŕàþþéŕš** — ýöü ŵŕöţé ñöŕḿàļ ĴŠẊ. ▒
- ▒ **Þļüĝîñ éẋţŕàçţéđ** éṽéŕý ţŕàñšļàţàƃļé éļéḿéñţ àţ ƃüîļđ ţîḿé, çöḿþüţéđ šţàƃļé ĥàšĥéš, àñđ ŕéŵŕöţé ţĥé ĴŠẊ ţö ļööķ ţĥéḿ üþ àţ ŕéñđéŕ ţîḿé. ▒
- ▒ **ķàþî þšéüđö-ţŕàñšļàţéđ** ţĥé ĶƂƑ → àñöţĥéŕ ĶƂƑ ŵîţĥ `ǫþš` ţàŕĝéţš þöþüļàţéđ. ▒
- ▒ **ñéöķàþî-î18ñ çöḿþîļéđ** ţĥàţ ĶƂƑ ţö à ĴŠÖÑ đîçţ ýöüŕ àþþ ļöàđš. ▒
- ▒ **Ţĥé ŕüñţîḿé** ŕéšöļṽéđ éàçĥ ĥàšĥ öñ ŕéñđéŕ; üñķñöŵñ ĥàšĥéš ƒàļļ ƃàçķ ţö ţĥé ĴŠẊ šöüŕçé ţéẋţ, šö ţĥé àþþ ñéṽéŕ šĥöŵš ŕàŵ îđéñţîƒîéŕš. ▒

## ▒ Ñéẋţ šţéþš ▒

- ▒ [Ŵŕîţîñĝ ţŕàñšļàţàƃļé çöḿþöñéñţš](./writing-components) — ŵĥàţ ñéöķàþî-î18ñ þîçķš üþ àüţöḿàţîçàļļý, ŵĥàţ îţ ŵàŕñš àƃöüţ, àñđ ĥöŵ ţö öþţ öüţ. ▒
- ▒ [Þļüŕàļš àñđ šéļéçţ](./plurals-and-select) — ÇĻĐŔ-àŵàŕé þļüŕàļ àüţĥöŕîñĝ ŵîţĥöüţ ÎÇÜ šţŕîñĝš îñ ýöüŕ šöüŕçé. ▒
- ▒ [Éẋţŕàçţ → ţŕàñšļàţé → çöḿþîļé þîþéļîñé](./pipeline) — ÀÎ ţŕàñšļàţîöñ, îñçŕéḿéñţàļ éẋţŕàçţš, ÇÎ îñţéĝŕàţîöñ. ▒
- ▒ [`ţ()` éšçàþé ĥàţçĥ](./t-escape-hatch) — ƒöŕ ţĥé šţŕîñĝš ţĥàţ ĝéñüîñéļý ƃéļöñĝ öüţšîđé ĴŠẊ. ▒
