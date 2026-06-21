import { defineConfig } from "vite-plus";

// Root Vite+ config: shared lint + format defaults for the whole workspace.
//
// `vp check` / `vp lint` / `vp fmt` read the lint/fmt blocks from here, so type
// checking (Oxlint type-aware path on the TypeScript-Go toolchain via tsgolint)
// is enabled once, centrally, for every package — `vp check` is the single
// static-check gate. This file is a STATIC defineConfig with no plugins so the
// Oxc/lint integration can load it reliably; per-package vite.config.ts files
// carry only Vite/Vitest/framework config (plugins, build, test).

// The vp surface is the frontend packages — each is checked per-package by
// `make frontend-check-all` (`cd <pkg> && vp check`), never from the repo root.
// A bare root `vp check` otherwise reaches OUTSIDE that surface into the Go
// modules' data files, the demo/tooling scripts, and infra config — dirs with
// their own toolchains (gofmt, byte-sensitive fixtures, hand-curated YAML) that
// no vp target gates. Reformatting them is pure churn (e.g. flipping every
// quote style in 100+ corpus/spec YAMLs). Keep them out of BOTH lint and fmt so
// a root run matches the per-package surface instead of rewriting the tree.
const OUT_OF_SURFACE = [
  "core/**", // Go framework module — corpus/spec/structure YAML + JSON fixtures & format-ops data
  "sievepen/**", // Go module data (tmx mappings)
  "termbase/**", // Go module data
  "providers/**", // Go module data
  "plugins/**", // Go plugin modules — manifests/testdata
  "examples/**", // example plugin manifests
  "harness/**", // demo-recording tooling (its own build/format toolchain)
  "scripts/**", // Node tooling + codegen scripts
  "specs/**", // spec catalog data
  "docs/**", // repo-internal docs data (the published site is web/, not docs/)
  ".skills/**", // skill data packs + scripts
  ".github/**", // workflow YAML
  ".claude/**", // workflow/agent scripts
  "deploy/**", // infra config (wrangler, preview worker)
  "apps/kapi-desktop/backend/**", // Go backend + sample fixtures (the frontend at apps/kapi-desktop/frontend stays in surface)
  // Generated data that lives INSIDE frontend packages (so the dir itself is in
  // surface, but these specific outputs are codegen and must stay untouched).
  "**/reference-data/data/**", // gen-refs output (formats/tools/commands JSON)
  "**/*.gen.ts", // codegen output (contract.gen.ts, …)
  // bowrain's NON-app dirs only — bowrain/apps/* + bowrain/packages/* + emails +
  // storybook + web/landing inherit this config and ARE checked (no bowrain root
  // config), so never exclude them wholesale. These are Go CLI, email templates,
  // infra compose, and the bowrain docs site (built by `make -C bowrain`).
  "bowrain/cli/**",
  "bowrain/mailer/**",
  "bowrain/deploy/**",
  "bowrain/web/docs/**",
  "bowrain/compose*.yaml",
];

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
      ...OUT_OF_SURFACE,
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
      // Package-manager-owned manifests — pnpm rewrites these on install, so the
      // formatter must not fight it (the array/quote style differs and churns).
      "**/package.json",
      "pnpm-workspace.yaml",
      ...OUT_OF_SURFACE,
    ],
  },
});
