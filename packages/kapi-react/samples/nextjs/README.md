# Next.js + @neokapi/kapi-react

## Inline mode (SSG — one build per locale)

```bash
npm install
npx kapi-react extract --src 'app/**/*.{tsx,jsx}'

# Build each locale separately:
LOCALE=en npm run build
LOCALE=de npm run build
LOCALE=de-AT npm run build   # Falls back to de.json for missing strings
LOCALE=ja npm run build

# Or build all at once:
npm run build:all
```

Next.js `i18n` routing resolves the user's locale from the URL path (`/de/about`).
Each locale build inlines translations at build time — zero runtime overhead.

## Fallback locales

The config uses `fallbackLocales: ['de', 'en']` so regional variants like
`de-AT` inherit from standard German. Only strings that differ between
Austrian and standard German need to be in `de-AT.json`:

```
translations/
  en.json        ← 500 strings
  de.json        ← 500 strings
  de-AT.json     ← 12 strings (overrides only)
```

## Strict mode in CI

The config uses `strict: process.env.CI ? 'error' : 'warn'`:
- **Locally**: warnings for missing translations (fast iteration)
- **In CI**: build fails if any string is untranslated (no half-translated deploys)

## Runtime mode (SPA with language switcher)

If you need dynamic locale switching without rebuilding:

```js
// next.config.js
config.plugins.push(neokapi({ mode: 'runtime' }));
```

```tsx
// app/layout.tsx
import { loadTranslations } from '@neokapi/kapi-react/runtime';

const locale = detectLocale();
if (locale !== 'en') {
  loadTranslations(locale, `/translations/${locale}.json`);
}
```

## How locale flows

```
User visits /de-AT/about
    → Next.js i18n routing detects locale "de-AT"
    → LOCALE=de-AT set during build
    → Plugin loads: de-AT.json + de.json + en.json (merged, de-AT wins)
    → Output: translated JSX with Austrian-specific overrides
```
