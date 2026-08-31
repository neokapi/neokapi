---
title: Ship-Aware Language Picker
description: "The two-gate ship model: a ship gate decides which locales go live, a verified gate decides which were human-reviewed. Emit a ship.json manifest with kapi status --ship and drive a language picker that hides un-shippable locales and flags AI-only ones."
keywords: [ship gate, verified gate, ship.json, language picker, verified, AI badge, kapi status, neokapi-i18n]
---

# Ship-aware language picker

A project decides which translated versions to offer its users. neokapi models
that decision with **two gates**, both declared in `kapi.yaml`, both evaluated
the same way against the target status ladder
(`draft → translated → reviewed → signed-off`):

- **The ship gate**: the bar to go live. A locale that clears it is safe to
  offer. This is the existing `ship_gate` / `ship_gates` configuration; its
  behaviour is unchanged.
- **The verified gate**: the bar to count as human-verified: a person reviewed
  or signed off the content. A locale that ships but is not verified is AI-only
  work.

The two gates are independent. Being verified is not a prerequisite for
shipping: a project can go live with machine translation and mark those locales
as unverified until a reviewer catches up.

## Declaring the verified gate

The verified gate uses the same three additive forms and the same precedence as
the ship gate, and resolves a `gate:` name against the shared `gates:` registry:

```yaml
# kapi.yaml
ship_gate: { translated: 100 } # go live once fully translated
verified_gate: { reviewed: 100 } # count as verified once fully reviewed
```

A rule list narrows the bar per collection or locale, most-specific rule wins:

```yaml
verified_gates:
  - when: { locales: [ja] }
    gate: { signed-off: 100 } # Japanese needs sign-off
  - gate: { reviewed: 100 } # everything else: reviewed
```

The recipe keys are **`verified_gate`** (a single catch-all gate) and
**`verified_gates`** (a when/gate rule list).

:::note The default is "nothing is verified"

A project with **no** verified gate has no verified locales: every shippable
locale reads as AI-only. Declaring `verified_gate` / `verified_gates` is how a
project opts in to the stronger claim. This keeps the label honest: "verified"
never appears unless a bar was declared and cleared.

:::

## Emitting the manifest

`kapi status --ship` projects the per-locale standing to a minimal manifest a
language picker can consume. Write it into your app's static assets as part of
the build:

```bash
kapi status --ship --emit public/ship.json
```

Without `--emit`, the manifest goes to stdout, so a build step can redirect it:

```bash
kapi status --ship > public/ship.json
```

The file is keyed by locale, each entry carrying the two gates' outcomes:

```json
{
  "fr": { "shippable": true, "verified": true },
  "de": { "shippable": true, "verified": false },
  "ja": { "shippable": false, "verified": false }
}
```

Here French ships and is verified (no badge), German ships but is AI-only
(flagged), and Japanese is not yet offered. The richer `kapi status --json`
report still carries the same `shippable` and `verified` fields per locale (plus
the full coverage percentages) for dashboards.

## A hosted feed instead of a built file

The manifest does not have to be a file you build into your assets. Because the
loader takes a URL, it can read the same shape from a **hosted feed**, a
server that serves the per-locale manifest live at a public URL. The build step
disappears, and the picker reads the current standing on each load rather than
whatever was true at build time.

The contract is exactly the file's: an object keyed by locale, each value
`{ shippable, verified }`. A hosted feed is read-only and needs no auth (a
public picker fetches it directly), and should send an `ETag` and a short
`Cache-Control: public, max-age=…` so a picker or a CDN can revalidate cheaply
with a `304`.

Point the loader at the URL; nothing else changes:

```ts
const status = await loadShipStatus("https://example.com/ship.json");
const model = languagePickerModel(status, ["en", "fr", "de", "ja"]);
```

The same `languagePickerModel` transform and the same `useShipStatus` hook (whose
second argument is the manifest URL) drive the picker whether the manifest came
from a built file or a hosted feed: one code path, one shape.

## Driving the picker

`@neokapi/i18n-react/ship` provides a dependency-free loader and a headless
transform. The loader tolerates a missing or malformed manifest: it resolves to
an empty object, so the picker falls back to showing every locale unbadged
rather than breaking the page.

Pass locale **codes**; the display label for each is derived automatically as the
locale's endonym (the language named in its own language) via
`Intl.DisplayNames`, so there is no per-locale label table to maintain:

```ts
import { loadShipStatus, languagePickerModel } from "@neokapi/i18n-react/ship";

const status = await loadShipStatus(); // defaults to /ship.json
const model = languagePickerModel(status, ["en", "fr", "de", "ja"]);
// → [{ locale: "fr", label: "Français", shippable: true, badge: 'ai' | null }, …]
```

The label is resolved in this order: an explicit `label` on a `LocaleInput`, then
an entry in the `labels` override map, then the `Intl.DisplayNames` endonym, then
the raw code. So an explicit label overrides the derived one where you want a
different form, and the override map names locales `Intl` cannot: a pseudo-locale
such as `qps` has no standard name, so give it one:

```ts
const model = languagePickerModel(status, ["fr", "de", "qps"], {
  labels: { qps: "Pseudo English" },
});
// fr → "Français" (derived), de → "Deutsch" (derived), qps → "Pseudo English" (override)
```

The signature is
`languagePickerModel(status, locales, options?)`, where `options` is
`{ labels?: Record<string, string> }`.

The first letter of a derived endonym is capitalized for a menu-style label
(`français` → `Français`), so lowercase endonyms read consistently alongside the
ones that are already capitalized (`Deutsch`); scripts without case (`日本語`) are
left unchanged. If `Intl.DisplayNames` is unavailable or has no name for a code,
the label falls back to the override map and then to the raw code; it never
throws.

`languagePickerModel` returns only the shippable locales. Each entry carries a
`badge`: `'ai'` when the locale ships but is not verified, and `null` when it is
verified. **`'ai'` is the only badge this layer emits; a verified locale has no
badge.** A React binding wraps the same two functions and takes the same options
as a third argument:

```tsx
import { useShipStatus } from "@neokapi/i18n-react/ship/react";

function LanguagePicker({ locales }) {
  // loads /ship.json, derives labels, returns the model
  const options = useShipStatus(locales, undefined, { labels: { qps: "Pseudo English" } });
  return (
    <ul>
      {options.map(({ locale, label, badge }) => (
        <li key={locale}>
          {label}
          {badge === "ai" && <span className="badge-ai">AI</span>}
        </li>
      ))}
    </ul>
  );
}
```

Rendering is left to the application: style the entries and the `AI` badge to
match your design. Development-only locales (pseudo-translation such as `qps`)
are not part of the ship manifest; name them through the `labels` override map
if you list them in the picker, or surface them separately in your own dev
tooling.

## Compatibility

The verified gate is purely additive. Recipes that declare no `verified_gate`
are unaffected: their ship-gate behaviour is unchanged and every locale reads as
unverified. The picker helper degrades safely when `ship.json` is absent, so a
project can adopt the manifest and the picker independently.
