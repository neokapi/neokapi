import { defineConfig } from "vite-plus";
import type { PluginOption } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import neokapi from "@neokapi/kapi-react/vite";
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
// variant: the @neokapi/kapi-react plugin inlines the compiled catalog
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

// Per-locale <html lang>, <title>/description/og swaps (from the human-owned
// locale-meta.json sidecar), and hreflang alternate links on every variant.
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
        const meta = localeMeta[locale as keyof typeof localeMeta];
        out = out
          .replace('<html lang="en">', `<html lang="${locale}">`)
          .replace(/<title>[^<]*<\/title>/, `<title>${meta.title}</title>`)
          .replace(/(<meta\s+name="description"\s+content=")[^"]*(")/, `$1${meta.description}$2`)
          .replace(/(<meta property="og:title" content=")[^"]*(")/, `$1${meta.ogTitle}$2`)
          .replace(
            /(<meta\s+property="og:description"\s+content=")[^"]*(")/,
            `$1${meta.ogDescription}$2`,
          );
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
  lint: {
    ignorePatterns: ["dist/**", "i18n/**", "i18n-*/**", "translations/**"],
  },
  fmt: {
    ignorePatterns: ["dist/**", "i18n/**", "i18n-*/**", "translations/**"],
  },
});
