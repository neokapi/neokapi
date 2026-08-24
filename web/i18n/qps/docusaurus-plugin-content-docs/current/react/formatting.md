---
sidebar_position: 6
title: Formatting Dates, Numbers, and Currency
description: ICU number, date, and time formatting inside translated strings, and how to combine neokapi-i18n's locale with the platform Intl API and third-party formatters for everything else.
keywords: [formatting, dates, numbers, currency, ICU, Intl API, date-fns, Luxon, locale, neokapi-i18n]
---

# ▒ Ƒöŕḿàţţîñĝ đàţéš, ñüḿƃéŕš, çüŕŕéñçý ▒

▒ Ţĥéŕé àŕé ţŵö þļàçéš ƒöŕḿàţţîñĝ ĥàþþéñš, àñđ ţĥé šþļîţ ḿàţţéŕš: ▒

- ▒ **Îñšîđé à ţŕàñšļàţéđ šţŕîñĝ** — `Ýöü ĥàṽé {n, number} üñŕéàđ ḿéššàĝéš`. Ţĥé ŕüñţîḿé ƒöŕḿàţš ţĥé ṽàļüé ţĥŕöüĝĥ `Îñţļ`, îñ ţĥé àçţîṽé ļöçàļé, àš þàŕţ öƒ ŕéšöļṽîñĝ ţĥé šţŕîñĝ. Ţĥé ţŕàñšļàţöŕ çöñţŕöļš îţ; ñö çöđé çĥàñĝé. ▒
- ▒ **Öüţšîđé à ţŕàñšļàţéđ šţŕîñĝ** — à þŕîçé îñ à ţàƃļé çéļļ, à ţîḿéšţàḿþ îñ à ļöĝ ṽîéŵ. Ţĥàţ'š ýöüŕ çöđé'š ĵöƃ. ñéöķàþî-î18ñ ĝîṽéš ýöü ţĥé ļöçàļé; ýöü ƃŕîñĝ ţĥé ƒöŕḿàţţéŕ. ▒

## ▒ Ƒöŕḿàţţîñĝ îñšîđé ţŕàñšļàţéđ šţŕîñĝš ▒

▒ À þļàçéĥöļđéŕ çàñ çàŕŕý àñ ÎÇÜ ƒöŕḿàţ, àñđ ţĥé ŕüñţîḿé ŕéšöļṽéš îţ àĝàîñšţ ţĥé àçţîṽé ļöçàļé: ▒

```tsx
<p>You have {count} unread messages.</p>
```

▒ Ţĥé éẋţŕàçţéđ ţéḿþļàţé îš `Ýöü ĥàṽé {count} üñŕéàđ ḿéššàĝéš.` — à ƃàŕé þļàçéĥöļđéŕ, îñţéŕþöļàţéđ ṽéŕƃàţîḿ. Ƃüţ à ţŕàñšļàţöŕ çàñ *üþĝŕàđé* îţ îñ ţĥé ţàŕĝéţ, ƃéçàüšé ţĥé ƒöŕḿàţ ļîṽéš îñ ţĥé šţŕîñĝ, ñöţ ţĥé šöüŕçé: ▒

```
Du hast {count, number} ungelesene Nachrichten.
```

▒ Ñöŵ `1234` ŕéñđéŕš àš `1.234` îñ Ĝéŕḿàñ àñđ `1,234` îñ Éñĝļîšĥ, àñđ ñöƃöđý ţöüçĥéđ ţĥé ĴŠẊ. Ţĥé šüþþöŕţéđ ƒöŕḿàţš: ▒

| Format                    | Example input | `en`         | `de`         |
| ------------------------- | ------------- | ------------ | ------------ |
| `{n, number}`             | `1234.5`      | `1,234.5`    | `1.234,5`    |
| `{n, number, integer}`    | `1234.5`      | `1,235`      | `1.235`      |
| `{n, number, percent}`    | `0.182`       | `18%`        | `18 %`       |
| `{n, number, currency/EUR}` | `1234.5`    | `€1,234.50`  | `1.234,50 €` |
| `{d, date}`               | a `Date`      | `Jul 11, 2026` | `11.07.2026` |
| `{d, date, short\|medium\|long\|full}` | a `Date` | `7/11/26` … | `11.07.26` … |
| `{t, time}`               | a `Date`      | `2:30:00 PM` | `14:30:00`   |
| `{t, time, short\|medium\|long\|full}` | a `Date` | `2:30 PM` … | `14:30` …    |

▒ Îñšîđé à `<Þļüŕàļ>` ƃŕàñçĥ, `#` îš ţĥé çöüñţ, ƒöŕḿàţţéđ ţĥé šàḿé ŵàý — `#` îñ Ĝéŕḿàñ ĝîṽéš `1.234`, ñöţ `1234`. ▒

▒ Þàšš `Đàţé` öƃĵéçţš àñđ ñüḿƃéŕš ţĥŕöüĝĥ àš-îš; ţĥé ŕüñţîḿé đöéš ţĥé çöñṽéŕšîöñ: ▒

```tsx
<p>Last synced {when} — {count} items.</p>   // when: Date, count: number
```

▒ Ţĥîš çöšţš ñö éẋţŕà ƃüñđļé: îţ'š ţĥé šàḿé `Îñţļ` ţĥé þļüŕàļ ŕéšöļṽéŕ àļŕéàđý üšéš. ▒

## ▒ Ƒöŕḿàţţîñĝ öüţšîđé ţŕàñšļàţéđ šţŕîñĝš — ţĥé îñţéĝŕàţîöñ šüŕƒàçé ▒

▒ Éṽéŕý ļöçàļé-àŵàŕé ļîƃŕàŕý öñ ţĥé þļàţƒöŕḿ ţàķéš à ƂÇÞ-47 ļöçàļé šţŕîñĝ — ţĥé šàḿé šĥàþé ñéöķàþî-î18ñ ţŕàçķš îñţéŕñàļļý. Þüļļ îţ öüţ ŕéàçţîṽéļý ṽîà `üšéÑéöķàþî()`: ▒

```tsx
import { useNeokapi } from "@neokapi/i18n-react/runtime";

function Price({ amount, currency }: { amount: number; currency: string }) {
  const { locale } = useNeokapi();
  return (
    <span>{new Intl.NumberFormat(locale, { style: "currency", currency }).format(amount)}</span>
  );
}
```

▒ Ŵĥéñ `ļöàđŢŕàñšļàţîöñš()` šŵàþš ţĥé đîçţ, `üšéÑéöķàþî()` ƒîŕéš à ŕé-ŕéñđéŕ ŵîţĥ ţĥé ñéŵ ļöçàļé šţŕîñĝ. Ƒöŕḿàţţéŕš þîçķ îţ üþ öñ ţĥé ñéẋţ ŕéñđéŕ. Ýöü đöñ'ţ ñééđ ţö þļüḿƃ ļöçàļé ţĥŕöüĝĥ þŕöþš öŕ çöñţéẋţ — ţĥé ĥööķ îš ţĥé ƃöüñđàŕý. ▒

## ▒ Šţàŕţ ŵîţĥ ñàţîṽé `Îñţļ` ▒

▒ `Îñţļ.*` šĥîþš îñ éṽéŕý ḿöđéŕñ ŕüñţîḿé (ƃŕöŵšéŕ + Ñöđé ≥ 18) ŵîţĥ ƒüļļ ÇĻĐŔ đàţà. Žéŕö ƃüñđļé çöšţ, éẋçéļļéñţ ŢýþéŠçŕîþţ ţýþéš, ƒàšţ. Çöṽéŕš ţĥé çöḿḿöñ çàšéš: ▒

```tsx
// Currency
new Intl.NumberFormat(locale, { style: "currency", currency: "USD" }).format(1234.56);
// "$1,234.56" / "1.234,56 $"

// Percent
new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 }).format(0.1824);
// "18.2%" / "18,2 %"

// Date
new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(new Date());
// "Apr 22, 2026" / "22. Apr. 2026"

// Date + time
new Intl.DateTimeFormat(locale, { dateStyle: "short", timeStyle: "short" }).format(new Date());
// "4/22/26, 6:30 PM" / "22.04.26, 18:30"

// Relative time
new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(-2, "day");
// "2 days ago" / "vor 2 Tagen"

// List
new Intl.ListFormat(locale, { style: "long", type: "conjunction" }).format([
  "apples",
  "oranges",
  "bananas",
]);
// "apples, oranges, and bananas"

// Unit
new Intl.NumberFormat(locale, { style: "unit", unit: "kilometer-per-hour" }).format(80);
// "80 km/h"

// Compact
new Intl.NumberFormat(locale, { notation: "compact", compactDisplay: "short" }).format(1_250_000);
// "1.3M" / "1,3 Mio."
```

▒ Ţĥŕéé Îñţļ ÀÞÎš ţĥàţ ḿàţţéŕ ƒöŕ öţĥéŕ šüƃšýšţéḿš: ▒

- ▒ **`Îñţļ.ÞļüŕàļŔüļéš`** — àļŕéàđý üšéđ îñţéŕñàļļý ƃý ñéöķàþî-î18ñ'š [`<Þļüŕàļ>`](./plurals-and-select) çöḿþöñéñţ. Ýöü đöñ'ţ ñééđ à ţĥîŕđ-þàŕţý þļüŕàļîžéŕ. ▒
- ▒ **`Îñţļ.Çöļļàţöŕ`** — ļöçàļé-çöŕŕéçţ šţŕîñĝ çöḿþàŕîšöñ. Üšé ƒöŕ šöŕţîñĝ ļîšţš öƒ ţŕàñšļàţéđ ñàḿéš (`îţéḿš.šöŕţ((à, ƃ) => ñéŵ Îñţļ.Çöļļàţöŕ(ļöçàļé).çöḿþàŕé(à.ñàḿé, ƃ.ñàḿé))`). ▒
- ▒ **`Îñţļ.Šéĝḿéñţéŕ`** — ŵöŕđ / šéñţéñçé / ĝŕàþĥéḿé ƃöüñđàŕîéš (üšéƒüļ ŵĥéñ ýöü ŵàñţ ţö çüţ à ļàƃéļ ḿîđ-ŵöŕđ çöŕŕéçţļý îñ ÇĴĶ). ▒

## ▒ Ŕéüšàƃļé ƒöŕḿàţţéŕ ĥööķš ▒

▒ Ŕé-çŕéàţîñĝ ƒöŕḿàţţéŕš öñ éṽéŕý ŕéñđéŕ îš ƒîñé (ţĥéý'ŕé çĥéàþ), ƃüţ ḿéḿöîžîñĝ îš çļéàñéŕ — àñđ ļéţš ýöü šĥàŕé çöñƒîĝüŕàţîöñ àçŕöšš çöḿþöñéñţš. À ţîñý ŵŕàþþéŕ: ▒

```tsx
import { useMemo } from "react";
import { useNeokapi } from "@neokapi/i18n-react/runtime";

export function useCurrency(currency: string) {
  const { locale } = useNeokapi();
  return useMemo(
    () => new Intl.NumberFormat(locale, { style: "currency", currency }),
    [locale, currency],
  );
}

export function useDateFormat(options: Intl.DateTimeFormatOptions = { dateStyle: "medium" }) {
  const { locale } = useNeokapi();
  // Stringify options once so useMemo deps stay stable for callers
  // passing a fresh object literal each render.
  const key = JSON.stringify(options);
  return useMemo(() => new Intl.DateTimeFormat(locale, options), [locale, key]);
}
```

▒ Üšàĝé: ▒

```tsx
function Cart() {
  const currency = useCurrency("EUR");
  const date = useDateFormat({ dateStyle: "short" });
  return (
    <footer>
      Subtotal: {currency.format(subtotal)}
      Delivered: {date.format(deliveryDate)}
    </footer>
  );
}
```

## ▒ Ţĥîŕđ-þàŕţý ļîƃŕàŕîéš ▒

▒ Ŵĥéñ Îñţļ đöéšñ'ţ çöṽéŕ ŵĥàţ ýöü ñééđ, ţĥé îđîöḿ îš ţĥé šàḿé: `çöñšţ { locale } = üšéÑéöķàþî()` → ḿàþ ţö ţĥé ļîƃŕàŕý'š ļöçàļé ţýþé → þàšš îñ. ▒

### ▒ đàţé-ƒñš ▒

▒ đàţé-ƒñš ļöçàļéš àŕé éẋþļîçîţ îḿþöŕţš. Ķééþ à šḿàļļ ḿàþ: ▒

```tsx
import { formatDistance, format } from "date-fns";
import { enUS, de, fr, es, ja } from "date-fns/locale";
import type { Locale } from "date-fns";

const DATE_FNS_LOCALES: Record<string, Locale> = {
  en: enUS,
  de,
  fr,
  es,
  ja,
};

function useDateFnsLocale(): Locale {
  const { locale } = useNeokapi();
  const primary = locale.split("-")[0];
  return DATE_FNS_LOCALES[primary] ?? enUS;
}

function Ago({ date }: { date: Date }) {
  const dfl = useDateFnsLocale();
  return <time>{formatDistance(date, new Date(), { addSuffix: true, locale: dfl })}</time>;
}
```

▒ Ƒöŕ ţîḿéžöñé-àŵàŕé ƒöŕḿàţţîñĝ, þàîŕ ŵîţĥ `đàţé-ƒñš-ţž`. ▒

### ▒ Ļüẋöñ ▒

▒ Ļüẋöñ šþéàķš ƂÇÞ-47 ñàţîṽéļý (îţ'š àñ Îñţļ ŵŕàþþéŕ üñđéŕñéàţĥ) àñđ šüþþöŕţš ţîḿéžöñéš ƒîŕšţ-çļàšš. ▒

```tsx
import { DateTime } from "luxon";

function LocalTime({ iso, zone }: { iso: string; zone: string }) {
  const { locale } = useNeokapi();
  const dt = DateTime.fromISO(iso, { zone }).setLocale(locale);
  return <time>{dt.toLocaleString(DateTime.DATETIME_MED)}</time>;
}
```

### ▒ đàýĵš ▒

```tsx
import dayjs from "dayjs";
import "dayjs/locale/de";
import "dayjs/locale/fr";
import relativeTime from "dayjs/plugin/relativeTime";
dayjs.extend(relativeTime);

function Ago({ iso }: { iso: string }) {
  const { locale } = useNeokapi();
  return <time>{dayjs(iso).locale(locale).fromNow()}</time>;
}
```

▒ Šîđé-éƒƒéçţ îḿþöŕţš ŕéĝîšţéŕ ļöçàļé đàţà; öñé þéŕ ļöçàļé ýöü šĥîþ. ▒

### ▒ ƑöŕḿàţĴŠ / ŕéàçţ-îñţļ ▒

▒ ƑöŕḿàţĴŠ îš à ƒüļļ-ƒéàţüŕéđ ÎÇÜ ḾéššàĝéƑöŕḿàţ šţàçķ. Îƒ ýöü'ŕé àļŕéàđý öñ îţ, ñéöķàþî-î18ñ àñđ ƑöŕḿàţĴŠ çàñ çöéẋîšţ — üšé ƑöŕḿàţĴŠ ƒöŕ ƒöŕḿàţţîñĝ àñđ ñéöķàþî-î18ñ ƒöŕ éẋţŕàçţîöñ + ţŕàñšļàţîöñ. Ƃüţ ýöü'ļļ ĥàṽé ţŵö šýšţéḿš ţŕàçķîñĝ ļöçàļé: ŵîŕé `çüŕŕéñţĻöçàļé` îñţö ƑöŕḿàţĴŠ'š `ÎñţļÞŕöṽîđéŕ`: ▒

```tsx
import { IntlProvider } from "react-intl";
import { useNeokapi } from "@neokapi/i18n-react/runtime";

function I18nRoot({ children }) {
  const { locale } = useNeokapi();
  return (
    <IntlProvider locale={locale} defaultLocale="en" messages={{}}>
      {children}
    </IntlProvider>
  );
}
```

▒ Ƒöŕ ĝŕééñƒîéļđ àþþš: šţîçķ ŵîţĥ Îñţļ. ƑöŕḿàţĴŠ àđđš ~40 ķƂ ƒöŕ ƒéàţüŕéš ñéöķàþî-î18ñ àļŕéàđý ĥàñđļéš (þļüŕàļš, šéļéçţ, ḿéššàĝé îñţéŕþöļàţîöñ, àñđ ñüḿƃéŕ/đàţé/ţîḿé ƒöŕḿàţţîñĝ îñšîđé šţŕîñĝš) þļüš à ƃüñçĥ îţ đöéšñ'ţ (ƃüţ ŵĥîçĥ Îñţļ öƒţéñ çöṽéŕš). ▒

## ▒ Ļîƃŕàŕý þîçķéŕ ▒

| Need                                                                      | Pick                                                                   | Notes                                                                             |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Currency, percent, date, time, relative time, list, unit, compact numbers | `Intl.*`                                                               | Already in the runtime. No imports.                                               |
| Pluralization (count-aware copy)                                          | neokapi-i18n's [`<Plural>`](./plurals-and-select)                        | Uses `Intl.PluralRules`. No extra library.                                        |
| Sorting translated names                                                  | `Intl.Collator`                                                        | `list.sort((a,b) => col.compare(a,b))`                                            |
| Timezone-aware dates, heavy date math                                     | Luxon or date-fns(-tz)                                                 | Luxon is Intl-based; date-fns is older but lighter.                               |
| Duration formatting ("3h 12m")                                            | Luxon `Duration.toHuman()` or `@formatjs/intl-durationformat` polyfill | `Intl.DurationFormat` exists in newer runtimes but isn't universally shipped yet. |
| Legacy moment.js codebase                                                 | migrate incrementally                                                  | moment.js is maintenance-mode; Luxon is its successor from the same author.       |
| Number/date/time **inside** a translated string                            | ICU in the string — `{n, number}`, `{d, date, long}`                   | Built in; see the top of this page. No library.                                    |
| ICU MessageFormat beyond that subset (ordinals, unit skeletons, …)         | `@formatjs/intl-messageformat` standalone                              | Just the formatter, not the whole react-intl stack.                               |

## ▒ Îñîţîàļ ŕéñđéŕ àñđ ŠŠŔ ▒

▒ Àļļ öƒ ţĥé àƃöṽé ŕéàđ ţĥé ļöçàļé àţ ŕéñđéŕ ţîḿé. Öñ ƒîŕšţ þàîñţ — ƃéƒöŕé `ļöàđŢŕàñšļàţîöñš()` ŕéšöļṽéš — `üšéÑéöķàþî()` ŕéţüŕñš ţĥé đéƒàüļţ ļöçàļé (`""` üñļéšš ýöü þŕé-çàļļéđ `šéţŢŕàñšļàţîöñš`). Ţĥàţ üšüàļļý ḿàþš ţö Éñĝļîšĥ ƒàļļƃàçķ ƒöŕḿàţţîñĝ, ŵĥîçĥ ḿàţçĥéš ţĥé Éñĝļîšĥ šöüŕçé ţéẋţ ţĥé àþþ ŕéñđéŕš ƃéƒöŕé ţŕàñšļàţîöñš àŕŕîṽé. Îƒ ţĥé ƒļîçķéŕ ḿàţţéŕš, šééđ ţĥé ļöçàļé öñ ţĥé šéŕṽéŕ šîđé: ▒

```tsx
// On the server, before hydration
setTranslations(cookieLocale, {}); // empty dict; locale alone is enough
```

▒ Ñöŵ ţĥé ƒîŕšţ çļîéñţ ŕéñđéŕ ĥàþþéñš ŵîţĥ ţĥé ŕîĝĥţ ļöçàļé, Îñţļ ƒöŕḿàţţéŕš ḿàţçĥ, àñđ ţĥé đîçţ šŵàþ öñļý çĥàñĝéš _šţŕîñĝš_ — ñöţ ƒöŕḿàţţîñĝ. ▒

▒ Šéé àļšö [Çöñƒîĝüŕàţîöñ → ĤŢḾĻ `ļàñĝ` àñđ `đîŕ` àţţŕîƃüţéš](./configuration#html-lang-and-dir-attributes) ƒöŕ ķééþîñĝ ţĥé đöçüḿéñţ ļöçàļé îñ šýñç öñ ƒîŕšţ þàîñţ. ▒

## ▒ Ŵĥàţ ñéöķàþî-î18ñ đéļîƃéŕàţéļý đöéšñ'ţ đö ▒

- ▒ **Ñüḿƃéŕ îñþüţ þàŕšîñĝ.** Þàŕšîñĝ `"1.234,56 €"` ƃàçķ îñţö `1234.56` îš ļöçàļé-đéþéñđéñţ àñđ ñöñ-ţŕîṽîàļ. Üšé à ƒöŕḿ ļîƃŕàŕý ŵîţĥ à ļöçàļé-àŵàŕé îñþüţ (`ŕéàçţ-ñüḿƃéŕ-ƒöŕḿàţ` ĥàš ļöçàļé šüþþöŕţ) öŕ ŵŕîţé à šḿàļļ þàŕšéŕ þéŕ îñþüţ šĥàþé. ▒
- ▒ **Üñîţ çöñṽéŕšîöñ.** Îñţļ ƒöŕḿàţš "1 ķḿ"; çöñṽéŕţîñĝ 1 ķḿ ţö ḿîļéš îš ýöüŕ àþþ'š ŕéšþöñšîƃîļîţý. ▒
- ▒ **Àđđŕéšš / þĥöñé / þöšţàļ çöđé ƒöŕḿàţţîñĝ.** Üšé à šþéçîàļîžéđ ļîƃŕàŕý (`ļîƃþĥöñéñüḿƃéŕ-ĵš`, `ļîƃþöšţàļ`). ▒

▒ Ţĥéšé àŕéñ'ţ î18ñ çöñçéŕñš šö ḿüçĥ àš đàţà ñöŕḿàļîžàţîöñ — ţĥéý ñééđ đöḿàîñ ļöĝîç ñéöķàþî-î18ñ ĥàš ñö ƃüšîñéšš îñ. ▒

## ▒ Ñéẋţ ▒

- ▒ [Þļüŕàļš àñđ šéļéçţ](./plurals-and-select) — ţĥé öñé ƒöŕḿàţţîñĝ çàšé ñéöķàþî-î18ñ _đöéš_ öŵñ, ƃéçàüšé îţ'š îñţéŕţŵîñéđ ŵîţĥ ţĥé ţŕàñšļàţéđ šţŕîñĝ îţšéļƒ. ▒
- ▒ [`ţ()` éšçàþé ĥàţçĥ](./t-escape-hatch) — ƒééđîñĝ ƒöŕḿàţţéđ ṽàļüéš îñţö ţŕàñšļàţéđ çöþý ṽîà þļàçéĥöļđéŕš: `ţ("Þŕîçé: {price}", { price: currencyFormatter.format(amount) })`. ▒
- ▒ [Çöñƒîĝüŕàţîöñ](./configuration) — ŕüñţîḿé öþţîöñš, îñçļüđîñĝ ţĥé `<ĥţḿļ ļàñĝ>` / `đîŕ` šýñç. ▒
