---
title: Ship-Aware Language Picker
description: "A locale ships established (a person established it) or translated (translated with its checks green). Emit a ship.json manifest with kapi status --ship and drive a language picker that hides locales that do not ship and flags the translated ones as AI."
keywords: [ship gate, established gate, ship.json, language picker, ship state, AI badge, kapi status, neokapi-i18n]
---

# Ship-aware language picker

A project decides which translated versions to offer its users. neokapi models
that decision with **two gates**, both declared in `kapi.yaml`, both evaluated
the same way against the target status ladder
(`draft → translated → established`):

- **The ship gate**: the bar to go live. A locale that clears it ships
  `translated`: translated with its checks green, and safe to offer. It is the
  `ship_gate` / `ship_gates` configuration.
- **The established gate**: the bar to ship `established`, meaning a person
  established the content. It is the `established_gate` / `established_gates`
  configuration.

A locale that ships `translated` is AI work, and the picker marks it. A project
that delivers only established content says so in its ship gate
(`ship_gate: { established: 100 }`).

## Declaring the established gate

The established gate uses the same three additive forms and the same
precedence as the ship gate, and resolves a `gate:` name against the shared
`gates:` registry:

```yaml
# kapi.yaml
ship_gate: { translated: 100 } # go live once fully translated
established_gate: { established: 100 } # ship established once a person established every unit
```

A rule list narrows the bar per collection or locale, most-specific rule wins:

```yaml
established_gates:
  - when: { locales: [ja] }
    gate: { established: 100 } # Japanese needs a person on every unit
  - gate: { established: 80 }
```

:::note With no established gate, nothing ships established

A project with **no** established gate has no locale in the `established`
state: every locale that ships reads `translated`, marked AI.

:::

:::note A locale no ship gate matches is not gated

A locale that no `ship_gate` or `ship_gates` rule matches has no bar to clear,
so kapi reports it as **not gated** rather than shippable. Nothing withholds it
on coverage, so the picker offers it by default. Stale wording, a translation a
reviewer turned down, a failing check, or terms that were not checked withhold a
locale whether or not a gate matches it.

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

The file is keyed by locale, each entry carrying whether the locale ships and
its ship state:

```json
{
  "fr": { "shippable": true, "state": "established" },
  "de": { "shippable": true, "state": "translated" },
  "nl": { "shippable": true, "state": "translated", "not_governed": ["terms"] },
  "sv": { "shippable": true, "state": "not_gated" },
  "ja": { "shippable": false, "state": "withheld" }
}
```

Here French ships established (no badge), German ships translated (flagged
AI), Dutch ships the same way in a language no terms govern, Swedish has no
gate and is offered as not gated, and Japanese is withheld.

`state` takes one of four values:

| `state` | Meaning | `shippable` |
| --- | --- | --- |
| `established` | An established gate matches the locale and the locale clears it: governed content. | `true` |
| `translated` | A ship gate matches the locale and the locale clears it, and no established gate is met: AI-shippable content. | `true` |
| `withheld` | The locale does not ship: it is short of its gate, or stale, rejected or failing content, or unchecked terms, hold it back. | `false` |
| `not_gated` | No gate matches the locale, and nothing withholds it. | `true` |

A locale spread over several collections takes the weakest state among them, in
the order withheld, not_gated, translated, established. `shippable` is `true`
for every state but `withheld`, so a picker that reads only `shippable` offers a
not-gated locale; `state` is what tells a cleared gate from no gate. An entry carries `not_governed` when a dimension governs
nothing in that language: `terms` means that none of the terms bound where the
language's content sits, under the project defaults or on a profile, has a term
for it. The picker does not read it. It is there so a build step or a
reader does not take the language for a governed one. The richer `kapi status --json`
report carries `shippable` and the state, as `shipState`, per collection and
locale, with the full coverage percentages, for dashboards.

## A hosted feed instead of a built file

The manifest does not have to be a file you build into your assets. Because the
loader takes a URL, it can read the same shape from a **hosted feed**, a
server that serves the per-locale manifest live at a public URL. The build step
disappears, and the picker reads the current standing on each load rather than
whatever was true at build time.

The contract is exactly the file's: an object keyed by locale, each value
`{ shippable, state }`, with `not_governed` where it applies. A hosted feed is read-only and needs no auth (a
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
// → [{ locale: "fr", label: "Français", shippable: true, badge: 'ai' | null, state: "translated" }, …]
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
`{ labels?: Record<string, string>; includeNotGated?: boolean }`.

The first letter of a derived endonym is capitalized for a menu-style label
(`français` → `Français`), so lowercase endonyms read consistently alongside the
ones that are already capitalized (`Deutsch`); scripts without case (`日本語`) are
left unchanged. If `Intl.DisplayNames` is unavailable or has no name for a code,
the label falls back to the override map and then to the raw code; it never
throws.

`languagePickerModel` returns the locales whose entry is shippable, which
includes a not-gated locale. Pass `includeNotGated: false` to offer only the
locales that clear a gate. Each entry carries the manifest's `state` and a
`badge`: `'ai'` when the locale ships but is not `established`, and `null` when
it is. **`'ai'` is the only badge this layer emits; an established locale has
no badge.** A React binding wraps the same two functions and takes the same options
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

The established gate is additive. A recipe that declares none has every
shipping locale read `translated`. The picker helper degrades safely when
`ship.json` is absent, and reads an entry with no `state` by its `shippable`
flag alone, so a project can adopt the manifest and the picker independently.
A recipe that still carries `verified_gate` fails to load and names
`established_gate`.
