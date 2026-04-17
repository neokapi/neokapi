# @neokapi/kapi-react

Zero-config i18n for React. Write vanilla JSX — translations happen at build time or runtime, with no source code changes.

```jsx
// You write this:
<h1>Welcome back, {user.name}!</h1>
<button>Save changes</button>
<input placeholder="Search..." />

// That's it. No imports. No wrappers. No translation keys.
```

## How it works

The plugin applies [W3C HTML5 translatability rules](https://www.w3.org/TR/its20/) to determine what needs translation — headings, paragraphs, buttons, labels, form placeholders, and more — automatically. It extracts translatable strings, and at build time either inlines translated text directly into the JSX or emits lightweight runtime calls for dynamic (OTA) loading.

## Install

```bash
npm install @neokapi/kapi-react
```

## Quick Start

### 1. Add the plugin to your build tool

<details open>
<summary><strong>Vite</strong></summary>

```ts
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';      // or plugin-react-swc
import neokapi from '@neokapi/kapi-react/vite';

export default defineConfig({
  plugins: [
    neokapi({ locale: process.env.LOCALE }),
    react(),
  ],
});
```

</details>

<details>
<summary><strong>Webpack</strong></summary>

```js
// webpack.config.js
const neokapi = require('@neokapi/kapi-react/webpack');

module.exports = {
  plugins: [
    neokapi({ locale: process.env.LOCALE }),
  ],
};
```

</details>

<details>
<summary><strong>Next.js</strong></summary>

```js
// next.config.js
const neokapi = require('@neokapi/kapi-react/webpack');

module.exports = {
  webpack: (config) => {
    config.plugins.push(
      neokapi({
        locale: process.env.LOCALE,
        translationsDir: './translations',
      })
    );
    return config;
  },
};
```

</details>

<details>
<summary><strong>Rollup</strong></summary>

```js
// rollup.config.js
import neokapi from '@neokapi/kapi-react/rollup';

export default {
  plugins: [
    neokapi({ locale: process.env.LOCALE }),
  ],
};
```

</details>

<details>
<summary><strong>esbuild</strong></summary>

```ts
import { build } from 'esbuild';
import neokapi from '@neokapi/kapi-react/esbuild';

await build({
  entryPoints: ['src/index.tsx'],
  plugins: [
    neokapi({ locale: process.env.LOCALE }),
  ],
});
```

</details>

<details>
<summary><strong>Rspack</strong></summary>

```js
// rspack.config.js
const neokapi = require('@neokapi/kapi-react/webpack');  // Rspack uses webpack API

module.exports = {
  plugins: [
    neokapi({ locale: process.env.LOCALE }),
  ],
};
```

</details>

### 2. Extract translatable content

```bash
vpx kapi-react extract --out i18n/extracted.klz
```

This scans your `src/` directory and produces a `.klz` archive —
the AD-045 exchange format. Each translatable JSX element becomes
a `Block` with structured `Run[]` that preserves inline markup,
variable tokens, and conditional placeholders:

```
documents/
  src-App.tsx.klf         # one .klf per source file, Block[] with typed Runs
  src-Sidebar.tsx.klf
manifest.json             # SHA-256 + per-part metadata
```

`.klz` is a JSON-inside-ZIP archive — you can `unzip -p app.klz
documents/src-App.tsx.klf | jq .` to inspect any block.

### 3. Translate (or pseudo-translate for testing)

```bash
# Pseudo-translate for visual QA:
kapi pseudo-translate i18n/extracted.klz --target-lang qps -o i18n/translated.klz

# Or hand off to your TMS / translators → get back a translated .klz
```

The translated `.klz` has per-locale target runs populated on every
`Block`; pseudo-translate leaves placeholders intact and only
rewrites text runs. Inline codes stay protected.

### 4. Compile to the runtime dictionary

```bash
vpx kapi-react compile i18n/translated.klz --out public/translations
```

Produces one `<locale>.json` file per target locale with the
`{hash: flattened-text}` shape the runtime loader fetches via
`loadTranslations()`:

```json
// translations/de.json
{
  "3kF": "Willkommen, {user.name}!",
  "7xQ": "Anderungen speichern",
  "xY2": "Suchen..."
}
```

### 4. Build with translations

```bash
LOCALE=de npm run build
```

Output — pure translated JSX, zero runtime:

```jsx
<h1>Willkommen, {user.name}!</h1>
<button>Anderungen speichern</button>
<input placeholder="Suchen..." />
```

## Plurals and Select

When a locale needs different text per count or per category, author
it with the `<Plural>` / `<Select>` components from
`@neokapi/kapi-react/runtime`. Each form is a child component
(`<Zero>`, `<One>`, `<Two>`, `<Few>`, `<Many>`, `<Other>` for plural,
`<Case when="…">` / `<Other>` for select) and each form's body is
fully typed JSX — inline elements, variables, and conditional
expressions inside a form stay structured, not stringified.

```tsx
import { Plural, Zero, One, Other } from '@neokapi/kapi-react/runtime';

<p>
  <Plural count={items.length}>
    <Zero>Your cart is empty</Zero>
    <One>1 item in your cart</One>
    <Other><strong>{items.length}</strong> items in your cart</Other>
  </Plural>
</p>
```

```tsx
import { Select, Case, Other } from '@neokapi/kapi-react/runtime';

<p>
  <Select value={user.role}>
    <Case when="admin">Welcome, admin</Case>
    <Case when="guest">You're browsing as a guest</Case>
    <Other>Welcome, {user.name}!</Other>
  </Select>
</p>
```

At build time the plugin rewrites these into an ICU template in the
runtime call:

```js
__tx(
  "3mUQVu",
  "{items.length, plural, zero {Your cart is empty} one {1 item in your cart} other {{=m0} items in your cart}}",
  { "=m0": <strong>{items.length}</strong> },
  { "items.length": items.length },
)
```

The compiled `translations/<locale>.json` keeps the same ICU shape —
translators get per-form text with inline markup preserved. The
runtime's `Intl.PluralRules` picks the right form at render time, and
`<strong>` etc. splice back in via `{=mN}` tokens so the final HTML
is identical to whatever the untranslated source would render.

Pivot variables (`count`, `value`) are marked in the extracted Block
as `kind: 'icu-pivot'` so validators know they must not be dropped
from any target locale.

## Three Modes

### Dev mode (default)

When no `locale` or `mode` is set, the plugin does nothing. Source text renders as-is. No overhead, instant HMR.

### Inline mode (build-time translation)

Set `locale` to inline translations at build time. Output is pure translated JSX — **zero runtime shipped to the browser**.

```ts
neokapi({ locale: 'de', translationsDir: './translations' })
```

Ideal for SSR/SSG (Next.js, Remix, Astro) where the locale is known at build or request time.

### Runtime/OTA mode (dynamic loading)

Set `mode: 'runtime'` for apps that switch languages without rebuilding. The plugin emits lightweight `t()` and `tx()` calls (~2KB runtime).

```ts
neokapi({ mode: 'runtime' })
```

```tsx
// The only code change needed — a language switcher:
import { loadTranslations } from '@neokapi/kapi-react/runtime';

function LanguageSwitcher() {
  return (
    <select onChange={e => loadTranslations(e.target.value, `/translations/${e.target.value}.json`)}>
      <option value="en">English</option>
      <option value="de">Deutsch</option>
      <option value="ja">Japanese</option>
    </select>
  );
}
```

All other components remain vanilla JSX — no i18n imports, no wrappers.

The runtime provides:

```ts
import { t, tx, useNeokapi, setTranslations, loadTranslations } from '@neokapi/kapi-react/runtime';

t(hash, fallback, params?)            // String translation with ICU support
tx(hash, fallback, elements, params?) // Rich JSX translation (inline elements preserved)
useNeokapi()                          // React hook — re-renders on translation change
setTranslations(locale, dict)         // Set translations synchronously
loadTranslations(locale, url)         // Fetch and activate translations from URL
```

### Inline elements in runtime mode

Text with `<a>`, `<strong>`, or other inline elements uses `tx()` instead of `t()`. The plugin detects this automatically — no developer action needed.

```jsx
// Developer writes:
<p>Click <a href="/settings">here</a> to manage your account.</p>

// Plugin emits (runtime mode):
<p>{tx("9qR", "Click {=m0} to continue.", { "=m0": <a href="/settings">here</a> })}</p>

// tx() resolves translation, preserving the <a> element:
// German: "Klicken Sie {=m0}, um Ihr Konto zu verwalten." → <a> inserted at {=m0}
```

The translator can reorder `{=m0}` tokens freely — the original JSX elements are spliced in at the right positions.

## How `locale` Works

The `locale` option in the plugin config is a **build-time target locale** — it tells the plugin which translation file to load from disk. It is **not** automatic browser locale detection.

```ts
locale: 'de'        // → reads translations/de.json → inlines German text
locale: 'qps'       // → reads translations/qps.json → inlines pseudo-translated text
locale: undefined   // → no-op (dev mode, source text shown)
```

How the end user's locale reaches the plugin depends on your deployment model:

### Static builds (one bundle per locale)

Build once for each locale. A CDN, router, or deploy script serves the right bundle.

```bash
LOCALE=en npm run build    # → dist-en/
LOCALE=de npm run build    # → dist-de/
LOCALE=ja npm run build    # → dist-ja/
```

### SSR / SSG (Next.js, Remix, Astro)

The framework determines the locale from the URL, cookie, or `Accept-Language` header and passes it to the build:

<details>
<summary><strong>Next.js with i18n routing</strong></summary>

```js
// next.config.js
const neokapi = require('@neokapi/kapi-react/webpack');

module.exports = {
  i18n: {
    locales: ['en', 'de', 'ja'],
    defaultLocale: 'en',
  },
  webpack: (config, { nextRuntime }) => {
    // Next.js builds each locale separately.
    // Use LOCALE env var or fall back to default.
    config.plugins.push(
      neokapi({
        locale: process.env.LOCALE || 'en',
        translationsDir: './translations',
      })
    );
    return config;
  },
};
```

```bash
# Build all locales:
for locale in en de ja; do
  LOCALE=$locale next build
done
```

</details>

<details>
<summary><strong>Remix / Astro</strong></summary>

These frameworks typically resolve locale from the URL path (`/de/about`). Use the `LOCALE` env var per-build, or use runtime mode for dynamic switching.

```bash
LOCALE=de npm run build
```

</details>

### SPA with dynamic locale switching (OTA/runtime mode)

In runtime mode, the plugin doesn't use `locale` at all — translations load dynamically in the browser. Your app determines the user's locale and fetches the matching translations:

```tsx
import { loadTranslations } from '@neokapi/kapi-react/runtime';

// On app startup — detect locale and load translations
const userLocale = detectLocale();
if (userLocale !== 'en') {
  loadTranslations(userLocale, `/translations/${userLocale}.json`);
}

function detectLocale(): string {
  // 1. Check user preference (saved in localStorage or cookie)
  const saved = localStorage.getItem('locale');
  if (saved) return saved;

  // 2. Check URL (e.g., /de/about → "de")
  const fromUrl = window.location.pathname.split('/')[1];
  if (['de', 'ja', 'fr'].includes(fromUrl)) return fromUrl;

  // 3. Check browser language
  const browserLang = navigator.language.split('-')[0];
  if (['de', 'ja', 'fr'].includes(browserLang)) return browserLang;

  // 4. Default
  return 'en';
}
```

### Summary

| Deployment | Who detects locale | How locale reaches the plugin |
|---|---|---|
| Static build | Deploy script / CI | `LOCALE=de npm run build` |
| SSR (Next.js) | Framework from URL/header | `process.env.LOCALE` in `next.config.js` |
| SSG | Build script | One `npm run build` per locale |
| SPA (runtime mode) | Your app at runtime | `loadTranslations(locale, url)` — plugin config uses `mode: 'runtime'` |

The plugin intentionally doesn't detect locale automatically — locale detection varies by framework, deployment, and business logic. The plugin's job is to translate; your app's job is to decide which language.

## Fallback Locale Chain

When a translation is missing in the primary locale, fall back through a chain of related locales before showing source text:

```ts
neokapi({
  locale: 'de-AT',
  fallbackLocales: ['de', 'en'],
  // Merges: en.json < de.json < de-AT.json (most specific wins)
})
```

This is useful for regional variants — Austrian German (`de-AT`) inherits from standard German (`de`), which inherits from English (`en`). Only strings that differ need to be in `de-AT.json`.

```
translations/
  en.json        ← 500 strings (full coverage)
  de.json        ← 500 strings (full German)
  de-AT.json     ← 12 strings  (only Austrian-specific overrides)
```

## Missing Translation Detection

Catch untranslated strings at build time instead of shipping half-translated pages:

```ts
neokapi({
  locale: 'de',
  strict: 'warn',     // Log warning, fall back to source text (default)
  // strict: 'error', // Fail the build on missing translations
  // strict: false,   // Silent fallback
})
```

In `strict: 'warn'` mode (default when locale is set), the build output shows:

```
[neokapi] Missing translation for "Save changes" (hash: 7xQ, locale: de)
[neokapi] Missing translation for "Search..." (hash: xY2, locale: de)
```

In `strict: 'error'` mode, the build fails on the first missing translation — useful in CI to enforce complete translations before deploy.

## Plurals and Gender

Plurals and gender are **translator-driven**. The developer writes plain English. The translator adds ICU MessageFormat in the translation file when the target language needs it.

### Developer writes:

```jsx
<p>{count} messages from {name}</p>
```

### German translator writes ICU plural:

```json
{
  "3kF": "{count, plural, one {{count} Nachricht von {name}} other {{count} Nachrichten von {name}}}"
}
```

### Gender via ICU select:

```json
{
  "7xQ": "{gender, select, male {{name} hat sein Profil aktualisiert} female {{name} hat ihr Profil aktualisiert} other {{name} hat das Profil aktualisiert}}"
}
```

The runtime resolves ICU using `Intl.PluralRules` (built into all browsers, zero polyfill). In inline mode, ICU is resolved at build time.

## Translatability Rules

The plugin automatically determines what to translate using W3C HTML5 defaults:

| Translatable | Not translatable | Container (children traversed) |
|---|---|---|
| `h1`-`h6`, `p`, `li`, `td`, `th` | `code`, `pre`, `kbd`, `var` | `div`, `section`, `form`, `nav` |
| `button`, `label`, `legend`, `option` | `script`, `style`, `textarea` | `header`, `footer`, `article` |
| `span`, `strong`, `em`, `a`, `b`, `i` | | `table`, `ul`, `ol`, `dl` |

**Translatable attributes:** `alt`, `title`, `placeholder`, `aria-label`, `aria-description`

### Opt out with standard HTML

```jsx
<p translate="no">API_KEY_PREFIX_12345</p>
```

### Add translator context

```jsx
<button data-i18n-note="verb: to close a dialog, not 'nearby'">Close</button>
```

### Override rules

```ts
neokapi({
  rules: [
    { selector: '.code-block', translate: false },
    { selector: '.hero-text', translate: true },
    { selector: '[data-testid]', translate: false },
  ],
});
```

### Custom components

The plugin auto-detects what HTML element a component renders:

```tsx
// Auto-detected: Button renders <button>
function Button({ children }) {
  return <button className="btn">{children}</button>;
}

// Also auto-detected from library .d.ts types:
// ForwardRefExoticComponent<Props & RefAttributes<HTMLButtonElement>> → button
```

For components that can't be auto-detected, use `componentMap`:

```ts
neokapi({
  componentMap: {
    'Card.Title': 'h2',
    'Dialog.Description': 'p',
  },
});
```

## Plugin Options

```ts
type PluginOptions = {
  mode?: 'inline' | 'runtime';              // Default: 'inline' when locale set
  locale?: string;                           // Target locale (e.g., "de", "qps")
  fallbackLocales?: string[];                // Fallback chain (e.g., ['de', 'en'])
  translationsDir?: string;                  // Default: "./translations"
  strict?: 'warn' | 'error' | false;        // Missing translation handling (default: 'warn')
  componentMap?: Record<string, string>;     // Component → HTML element mapping
  rules?: Array<{                            // Override translatability rules
    selector: string;
    translate?: boolean;
    locNote?: string;
  }>;
  communityManifestDir?: string;             // Path to library i18n manifests
  warnUnmapped?: boolean;                    // Warn about unmapped components (default: true in dev)
};
```

## Storybook Integration

Preview your components in each locale via a toolbar dropdown. Wire up
`.storybook/preview.ts` with the built-in helpers from
`@neokapi/kapi-react/storybook`:

```ts
// .storybook/preview.ts
import type { Preview } from '@storybook/react-vite';
import { neokapiDecorator, neokapiGlobalType } from '@neokapi/kapi-react/storybook';

const i18n = {
  locales: [
    { value: 'en', title: 'English' },
    { value: 'qps', title: 'Pseudo English', url: '/translations/qps.json' },
    { value: 'de', title: 'Deutsch', url: '/translations/de.json' },
  ],
};

const preview: Preview = {
  globalTypes: {
    locale: neokapiGlobalType(i18n),
  },
  decorators: [neokapiDecorator(i18n)],
};

export default preview;
```

The vite plugin stays in `main.ts` as usual — nothing Storybook-specific
there. The decorator lazy-imports the runtime so Storybooks without i18n
pay nothing for the import.

- `neokapiGlobalType(opts)` — returns a `globalTypes` entry registering
  the toolbar dropdown (icon: globe, dynamic title).
- `neokapiDecorator(opts)` — applies translations whenever the user
  picks a new locale. SSR-safe (no-ops when `fetch` is unavailable) and
  falls back to source text if the translation file can't be loaded.

## CLI

Two subcommands, run via `vp run` or `vpx kapi-react`:

```bash
vpx kapi-react extract [options]

Options:
  --src <glob>            Source files to scan (default: "src/**/*.{tsx,jsx}")
  --out <path>            Output archive (default: "i18n/extracted.klz")
  --config <path>         Config file with componentMap, rules, …
  --project <id>          Project id stamped into manifest.project
  --source-locale <bcp>   Source locale (default: "en")
  --target-locale <bcp>   Declared target locale (repeat for multiple)

vpx kapi-react compile <input.klz> [options]

Options:
  --locale <bcp>          Compile only this locale (repeat for multiple).
                          Defaults to every locale found on block.targets
                          and in manifest.project.targetLocales.
  --out <dir>             Output directory (default: "public/translations")
```

The boundary is: `kapi-react` extracts to `.klz` and compiles translated
`.klz` back to the runtime dictionary. Everything in between — pseudo-
translate, AI translate, TM matching, QA, review — goes through the
`kapi` CLI (AD-045).

## Pseudo-Translation Workflow

Test your UI with pseudo-translated text to catch truncation, layout
issues, and hardcoded strings:

```bash
# 1. Extract to a .klz archive
vpx kapi-react extract --out i18n/extracted.klz --target-locale qps

# 2. Pseudo-translate with kapi
kapi pseudo-translate i18n/extracted.klz --target-lang qps -o i18n/translated.klz

# 3. Compile the translated .klz to public/translations/qps.json
vpx kapi-react compile i18n/translated.klz

# 4. Build or dev with the pseudo-locale
LOCALE=qps npm run dev   # (or set the locale via your UI language picker)
```

All translatable text becomes `[àccéntéd ànd pàddéd]` — instantly
visible in the UI. Placeholders like `{user.name}` and inline elements
like `<a>here</a>` are preserved through every step.

## How It Compares

| | @neokapi/kapi-react | react-i18next | Lingui | fbtee |
|---|:---:|:---:|:---:|:---:|
| Source code changes | **None** | Every line | Every line | Every line |
| Manual translation keys | **No** | Yes | No | No |
| Build tool dependency | unplugin (any) | None | Babel/SWC | Babel |
| Runtime bundle (inline) | **0 KB** | ~8 KB | ~3 KB | ~5 KB |
| Runtime bundle (OTA) | **~2 KB** | ~8 KB | ~3 KB | ~5 KB |
| Plural/gender | Translator-driven | Developer-driven | Developer-driven | Developer-driven |
| React version | 18+ | 16.8+ | 16.14+ | 19 only |

## License

Apache-2.0
