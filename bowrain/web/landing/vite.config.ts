import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import neokapi from "@neokapi/i18n-react/vite";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import localeMeta from "./locale-meta.json" with { type: "json" };

// Build freshness stamp ("<YYYY-MM-DD HH:MM> UTC · <short-sha>"), injected at
// build time so the deployed page shows when and from what commit it was built.
const gitSha = (() => {
  try {
    return execFileSync("git", ["rev-parse", "--short", "HEAD"], {
      stdio: ["ignore", "pipe", "ignore"],
    })
      .toString()
      .trim();
  } catch {
    return process.env.GITHUB_SHA?.slice(0, 9) ?? "dev";
  }
})();
const buildStamp = `${new Date().toISOString().slice(0, 16).replace("T", " ")} UTC · ${gitSha}`;

// ── Locale (dogfood l10n) ────────────────────────────────────────────────────
// The landing ships as one static build per locale: English at the base path,
// each target locale (currently nb) at <base>/<locale>/. LOCALE selects the
// variant: the @neokapi/i18n-react plugin inlines the compiled catalog
// (translations/<locale>.json, committed + drift-gated via `make
// l10n-landing`) into the JSX at build time — no i18n runtime ships. Missing
// translations warn and fall back to English (target drift never blocks the
// build). `vp run build && vp run build:nb` emits dist/ + dist/nb/.
const locale = process.env.LOCALE ?? "";
const targetLocales = Object.keys(localeMeta) as (keyof typeof localeMeta)[];
const rootBase = process.env.VITE_BASE ?? "/";
const base = locale ? `${rootBase}${locale}/` : rootBase;
// Absolute origin for hreflang alternates (they must be fully qualified).
// Defaults to the GitHub Pages host the workflow deploys to; override with
// LANDING_ORIGIN when the site moves to its own domain.
const origin = process.env.LANDING_ORIGIN ?? "https://bowrain.cloud";

// Per-locale <html lang>, optional <title>/description/og swaps, and hreflang
// alternate links on every variant.
//
// locale-meta.json declares which target locales the site builds; each may
// carry head strings, and each field is applied only when present. The head is
// the one place a target locale's prose is not catalog-driven, so an absent
// field falls back to English exactly as a missing catalog entry does — no
// hand-translation, and no locale left announcing a framing the page retired.
type LocaleMeta = Partial<{
  title: string;
  description: string;
  ogTitle: string;
  ogDescription: string;
}>;

function localeHtml(): PluginOption {
  return {
    name: "landing-locale-html",
    transformIndexHtml(html: string): string {
      const alternates = [
        `    <link rel="alternate" hreflang="en" href="${origin}${rootBase}" />`,
        ...targetLocales.map(
          (l) => `    <link rel="alternate" hreflang="${l}" href="${origin}${rootBase}${l}/" />`,
        ),
        `    <link rel="alternate" hreflang="x-default" href="${origin}${rootBase}" />`,
      ].join("\n");
      let out = html.replace("</head>", `${alternates}\n  </head>`);
      if (locale && locale in localeMeta) {
        const meta: LocaleMeta = localeMeta[locale as keyof typeof localeMeta];
        out = out.replace('<html lang="en">', `<html lang="${locale}">`);
        const swaps: Array<[string | undefined, RegExp, (v: string) => string]> = [
          [meta.title, /<title>[^<]*<\/title>/, (v) => `<title>${v}</title>`],
          [meta.description, /(<meta\s+name="description"\s+content=")[^"]*(")/, (v) => `$1${v}$2`],
          [meta.ogTitle, /(<meta property="og:title" content=")[^"]*(")/, (v) => `$1${v}$2`],
          [
            meta.ogDescription,
            /(<meta\s+property="og:description"\s+content=")[^"]*(")/,
            (v) => `$1${v}$2`,
          ],
        ];
        for (const [value, pattern, replacement] of swaps) {
          if (value) out = out.replace(pattern, replacement(value));
        }
      }
      return out;
    },
  };
}

export default defineConfig({
  base,
  define: { __BUILD_STAMP__: JSON.stringify(buildStamp) },
  // neokapi() is an unplugin `.vite` adapter; bound to vite's own
  // PluginOption to keep vite-plus's config types from recursing (same
  // pattern as apps/kapi-desktop/frontend/vite.config.ts).
  plugins: [
    neokapi({
      locale: locale || undefined,
      translationsDir: "./translations",
    }) as PluginOption,
    localeHtml(),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: locale ? `dist/${locale}` : "dist",
  },
  test: {
    // Pure-logic tests run in node; DOM tests opt into jsdom per-file with a
    // `// @vitest-environment jsdom` pragma. `e2e/` is Playwright's, and its
    // specs would otherwise be collected here and fail on a missing runner.
    environment: "node",
    exclude: ["dist/**", "e2e/**", "node_modules/**"],
  },
  lint: {
    ignorePatterns: ["dist/**", "i18n/**", "i18n-*/**", "translations/**"],
  },
  fmt: {
    ignorePatterns: ["dist/**", "i18n/**", "i18n-*/**", "translations/**"],
  },
});
