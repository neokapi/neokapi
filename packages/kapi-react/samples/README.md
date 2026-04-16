# Sample Projects

Each directory shows how to configure `@neokapi/kapi-react` with a different build tool.
All samples use the same React component (`shared/App.tsx`) — the only difference
is the build configuration.

| Directory | Build Tool | Config File |
|-----------|-----------|-------------|
| `vite/` | Vite | `vite.config.ts` |
| `webpack/` | Webpack 5 | `webpack.config.js` |
| `nextjs/` | Next.js 15 | `next.config.js` |
| `rollup/` | Rollup 4 | `rollup.config.js` |
| `esbuild/` | esbuild | `build.ts` |
| `rspack/` | Rspack | `rspack.config.js` |

## Shared files

- `shared/App.tsx` — sample component (vanilla JSX, no i18n imports)
- `shared/App.runtime.tsx` — sample with OTA language switcher, locale detection, and `tx()` for rich JSX
- `shared/translations/de.json` — sample German translations

## Features demonstrated

| Feature | Where shown |
|---|---|
| Inline mode (zero runtime) | All samples: `LOCALE=de npm run build` |
| Runtime/OTA mode (`t` + `tx`) | `shared/App.runtime.tsx` |
| Fallback locale chain | `vite/vite.config.ts`, `nextjs/next.config.js` |
| Missing translation warnings | `vite/vite.config.ts` (`strict: 'warn'`) |
| Strict mode in CI | `nextjs/next.config.js` (`strict: process.env.CI ? 'error' : 'warn'`) |
| Regional variants (de-AT) | `nextjs/` sample |
| Rich JSX in runtime mode | Auto-generated `tx()` for text with `<a>`, `<strong>` |

## Quick start (any sample)

```bash
cd samples/vite     # or webpack, nextjs, rollup, esbuild, rspack
npm install
npx kapi-react extract
LOCALE=de npm run build
```

## How locale resolution works

The `locale` option is a **build-time target** — it selects which translation file to
load from disk. The plugin does NOT detect the end user's locale automatically.

### Inline mode (build-time)

The locale comes from an environment variable set during the build:

```bash
LOCALE=de npm run build     # → reads translations/de.json → inlines German
LOCALE=ja npm run build     # → reads translations/ja.json → inlines Japanese
```

How the env var gets set depends on your deployment:
- **CI/CD**: set by the build matrix or deploy script
- **Next.js**: one build per locale via `i18n.locales` config
- **Static hosting**: separate builds uploaded to `/en/`, `/de/`, `/ja/` paths

### Runtime/OTA mode (dynamic)

The plugin doesn't use `locale` at all. Your app detects the user's locale at
runtime and calls `loadTranslations()`. See `shared/App.runtime.tsx` for a
complete example with:
- `localStorage` preference
- `navigator.language` fallback
- Language switcher UI

## The key difference between samples

The **only** thing that changes between build tools is how the plugin is registered:

```ts
// Vite
import neokapi from '@neokapi/kapi-react/vite';
plugins: [neokapi({ locale }), react()]

// Webpack / Next.js / Rspack
const neokapi = require('@neokapi/kapi-react/webpack');
plugins: [neokapi({ locale })]

// Rollup
import neokapi from '@neokapi/kapi-react/rollup';
plugins: [neokapi({ locale }), ...]

// esbuild
import neokapi from '@neokapi/kapi-react/esbuild';
plugins: [neokapi({ locale })]
```

Everything else — the React components, the extraction CLI, the translation
files, the locale detection logic — is identical.
