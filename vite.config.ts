import { defineConfig } from "vite-plus";

// Root Vite+ config: shared lint + format defaults for the whole workspace.
//
// `vp check` / `vp lint` / `vp fmt` read the lint/fmt blocks from here, so type
// checking (Oxlint type-aware path on the TypeScript-Go toolchain via tsgolint)
// is enabled once, centrally, for every package — `vp check` is the single
// static-check gate. This file is a STATIC defineConfig with no plugins so the
// Oxc/lint integration can load it reliably; per-package vite.config.ts files
// carry only Vite/Vitest/framework config (plugins, build, test).
export default defineConfig({
  lint: {
    ignorePatterns: [
      "**/dist/**",
      "**/build/**",
      "**/.docusaurus/**",
      // Match the per-package tsconfig excludes: stories, story-/test-only
      // sources, and e2e specs are not part of the app type-check surface.
      "**/*.stories.ts",
      "**/*.stories.tsx",
      "**/stories/**",
      "**/__tests__/**",
      "**/*.test.ts",
      "**/*.test.tsx",
      "**/*.spec.ts",
      "**/*.spec.tsx",
      "**/e2e/**",
      // Generated Wails bindings (JS with JSDoc) live outside each app's src/
      // tsconfig scope — they are codegen output, not hand-written sources.
      "**/bindings/**",
    ],
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  fmt: {
    // Markdown/MDX prose is not oxfmt-managed in this repo (it's authored for
    // Docusaurus and would risk MDX/admonition changes); generated and build
    // output is never formatted.
    ignorePatterns: [
      "**/*.md",
      "**/*.mdx",
      "**/dist/**",
      "**/build/**",
      "**/.docusaurus/**",
      "**/bindings/**",
      // Committed-but-generated docs artifacts. These are produced by `make`
      // targets / the format-triage skill / the demo harness and checked in so
      // the sites build in CI without re-running them — but they are codegen
      // output, not hand-authored, so the formatter must leave them untouched
      // (reformatting them is pure churn on every regeneration).
      "**/static/data/**", // dashboard datasets (parity, format-maturity, pseudobench, contract-audit, …)
      "**/scenes/**/messages.json", // generated scene narration (TTS)
      "**/demos/**/out/**", // generated demo outputs
      "**/demos/**/fixtures/**", // authored, byte-sensitive demo inputs (reformatting alters the demo)
      "**/pages/**/_*.json", // colocated generated page data (_eval.json, _benchmark.json)
    ],
  },
});
