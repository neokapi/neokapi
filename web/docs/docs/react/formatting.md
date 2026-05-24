---
sidebar_position: 6
title: Formatting Dates, Numbers, and Currency
description: How to combine kapi-react's locale with the platform Intl API and third-party formatters — date-fns, Luxon, Dinero — for locale-aware date, number, and currency display.
keywords: [formatting, dates, numbers, currency, Intl API, date-fns, Luxon, locale, kapi-react]
---

# Formatting dates, numbers, currency

kapi-react is strictly a translation layer — it gives you translated strings. **Formatting** (turning a `Date` into a localized string, a `number` into a price, a `Duration` into "3 hours ago") is a separate concern. You bring your own formatter; kapi-react gives you the locale.

## The integration surface

Every locale-aware library on the platform takes a BCP-47 locale string — the same shape kapi-react tracks internally. Pull it out reactively via `useNeokapi()`:

```tsx
import { useNeokapi } from "@neokapi/react/runtime";

function Price({ amount, currency }: { amount: number; currency: string }) {
  const { locale } = useNeokapi();
  return (
    <span>{new Intl.NumberFormat(locale, { style: "currency", currency }).format(amount)}</span>
  );
}
```

When `loadTranslations()` swaps the dict, `useNeokapi()` fires a re-render with the new locale string. Formatters pick it up on the next render. You don't need to plumb locale through props or context — the hook is the boundary.

## Start with native `Intl`

`Intl.*` ships in every modern runtime (browser + Node ≥ 18) with full CLDR data. Zero bundle cost, excellent TypeScript types, fast. Covers the common cases:

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

Three Intl APIs that matter for other subsystems:

- **`Intl.PluralRules`** — already used internally by kapi-react's [`<Plural>`](./plurals-and-select) component. You don't need a third-party pluralizer.
- **`Intl.Collator`** — locale-correct string comparison. Use for sorting lists of translated names (`items.sort((a, b) => new Intl.Collator(locale).compare(a.name, b.name))`).
- **`Intl.Segmenter`** — word / sentence / grapheme boundaries (useful when you want to cut a label mid-word correctly in CJK).

## Reusable formatter hooks

Re-creating formatters on every render is fine (they're cheap), but memoizing is cleaner — and lets you share configuration across components. A tiny wrapper:

```tsx
import { useMemo } from "react";
import { useNeokapi } from "@neokapi/react/runtime";

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

Usage:

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

## Third-party libraries

When Intl doesn't cover what you need, the idiom is the same: `const { locale } = useNeokapi()` → map to the library's locale type → pass in.

### date-fns

date-fns locales are explicit imports. Keep a small map:

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

For timezone-aware formatting, pair with `date-fns-tz`.

### Luxon

Luxon speaks BCP-47 natively (it's an Intl wrapper underneath) and supports timezones first-class.

```tsx
import { DateTime } from "luxon";

function LocalTime({ iso, zone }: { iso: string; zone: string }) {
  const { locale } = useNeokapi();
  const dt = DateTime.fromISO(iso, { zone }).setLocale(locale);
  return <time>{dt.toLocaleString(DateTime.DATETIME_MED)}</time>;
}
```

### dayjs

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

Side-effect imports register locale data; one per locale you ship.

### FormatJS / react-intl

FormatJS is a full-featured ICU MessageFormat stack. If you're already on it, kapi-react and FormatJS can coexist — use FormatJS for formatting and kapi-react for extraction + translation. But you'll have two systems tracking locale: wire `currentLocale` into FormatJS's `IntlProvider`:

```tsx
import { IntlProvider } from "react-intl";
import { useNeokapi } from "@neokapi/react/runtime";

function I18nRoot({ children }) {
  const { locale } = useNeokapi();
  return (
    <IntlProvider locale={locale} defaultLocale="en" messages={{}}>
      {children}
    </IntlProvider>
  );
}
```

For greenfield apps: stick with Intl. FormatJS adds ~40 kB for features kapi-react already handles (plurals, select, message interpolation) plus a bunch it doesn't (but which Intl often covers).

## Library picker

| Need                                                                      | Pick                                                                   | Notes                                                                             |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Currency, percent, date, time, relative time, list, unit, compact numbers | `Intl.*`                                                               | Already in the runtime. No imports.                                               |
| Pluralization (count-aware copy)                                          | kapi-react's [`<Plural>`](./plurals-and-select)                        | Uses `Intl.PluralRules`. No extra library.                                        |
| Sorting translated names                                                  | `Intl.Collator`                                                        | `list.sort((a,b) => col.compare(a,b))`                                            |
| Timezone-aware dates, heavy date math                                     | Luxon or date-fns(-tz)                                                 | Luxon is Intl-based; date-fns is older but lighter.                               |
| Duration formatting ("3h 12m")                                            | Luxon `Duration.toHuman()` or `@formatjs/intl-durationformat` polyfill | `Intl.DurationFormat` exists in newer runtimes but isn't universally shipped yet. |
| Legacy moment.js codebase                                                 | migrate incrementally                                                  | moment.js is maintenance-mode; Luxon is its successor from the same author.       |
| ICU MessageFormat outside kapi-react's plural/select                      | `@formatjs/intl-messageformat` standalone                              | Just the formatter, not the whole react-intl stack.                               |

## Initial render and SSR

All of the above read the locale at render time. On first paint — before `loadTranslations()` resolves — `useNeokapi()` returns the default locale (`""` unless you pre-called `setTranslations`). That usually maps to English fallback formatting, which matches the English source text the app renders before translations arrive. If the flicker matters, seed the locale on the server side:

```tsx
// On the server, before hydration
setTranslations(cookieLocale, {}); // empty dict; locale alone is enough
```

Now the first client render happens with the right locale, Intl formatters match, and the dict swap only changes _strings_ — not formatting.

See also [Configuration → HTML `lang` and `dir` attributes](./configuration#html-lang-and-dir-attributes) for keeping the document locale in sync on first paint.

## What kapi-react deliberately doesn't do

- **Number input parsing.** Parsing `"1.234,56 €"` back into `1234.56` is locale-dependent and non-trivial. Use a form library with a locale-aware input (`react-number-format` has locale support) or write a small parser per input shape.
- **Unit conversion.** Intl formats "1 km"; converting 1 km to miles is your app's responsibility.
- **Address / phone / postal code formatting.** Use a specialized library (`libphonenumber-js`, `libpostal`).

These aren't i18n concerns so much as data normalization — they need domain logic kapi-react has no business in.

## Next

- [Plurals and select](./plurals-and-select) — the one formatting case kapi-react _does_ own, because it's intertwined with the translated string itself.
- [`t()` escape hatch](./t-escape-hatch) — feeding formatted values into translated copy via placeholders: `t("Price: {price}", { price: currencyFormatter.format(amount) })`.
- [Configuration](./configuration) — runtime options, including the `<html lang>` / `dir` sync.
