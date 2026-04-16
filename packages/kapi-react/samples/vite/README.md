# Vite + React + @neokapi/kapi-react

## Inline mode (static builds)

```bash
npm install
npm run dev              # Dev mode — source text, no transformation
npx kapi-react extract      # Extract strings → i18n/strings.json
LOCALE=de npm run build  # Build with German translations inlined (zero runtime)
```

The `LOCALE` env var is the build-time target locale. It tells the plugin which
`translations/{locale}.json` file to read. It is not automatic browser detection.

## Fallback locales

The config uses `fallbackLocales: ['en']` — if a string is missing in the
target locale, English is used as fallback before showing source text. This
is useful for partial translations:

```bash
LOCALE=de-AT npm run build  # de-AT.json < de.json < en.json (most specific wins)
```

## Missing translation warnings

With `strict: 'warn'` (default), the build output shows warnings for untranslated
strings. Use `strict: 'error'` in CI to fail the build on missing translations.

## Runtime/OTA mode (SPA with language switcher)

For apps that need to switch languages without rebuilding:

```ts
// vite.config.ts — use mode: 'runtime' instead of locale
neokapi({ mode: 'runtime' })
```

```tsx
// src/main.tsx — detect user locale and load translations
import { loadTranslations } from '@neokapi/kapi-react/runtime';

const locale = localStorage.getItem('locale')
  || navigator.language.split('-')[0]
  || 'en';

if (locale !== 'en') {
  loadTranslations(locale, `/translations/${locale}.json`);
}
```

Inline elements like `<a>` and `<strong>` are fully supported in runtime mode
via the `tx()` function — the plugin handles this automatically.
