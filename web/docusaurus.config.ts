import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

// Build freshness stamp ("<YYYY-MM-DD HH:MM> UTC · <short-sha>"), appended to
// the footer copyright so the deployed docs reveal when/from-what they built.
const buildStamp = (() => {
  let sha = process.env.GITHUB_SHA?.slice(0, 9) ?? "dev";
  try {
    sha = execFileSync("git", ["rev-parse", "--short", "HEAD"], {
      stdio: ["ignore", "pipe", "ignore"],
    })
      .toString()
      .trim();
  } catch {
    /* not a git checkout — keep the env/dev fallback */
  }
  return `${new Date().toISOString().slice(0, 16).replace("T", " ")} UTC · ${sha}`;
})();

// This Docusaurus instance IS the neokapi site: in production it sits at the
// site root (/) of neokapi.github.io and its home page (src/pages/index.tsx)
// is the product landing page. (A separate Vite landing app previously
// occupied this root; it
// was retired and its content folded into the docs home — see
// src/components/home/.) Production serves at the site ROOT ("/") of
// neokapi.github.io; PR previews are instead served from
// /web/prs/<N>/neokapi/docs/ (the deploy step runs from the default branch and
// slots PR docs there), so the deploy workflow overrides the base path via
// DOCS_BASE_URL — without it, internal links would carry the wrong prefix
// and navigate out of the preview.
const baseUrl = process.env.DOCS_BASE_URL ?? "/";

// Cookieless analytics (pageviews only on this site; see
// src/clientModules/analytics.ts and @neokapi/docs-shared). Key-gated: with no
// POSTHOG_KEY at build time — local dev, fork builds, PR previews — the client
// module never initializes and no analytics code loads. The host defaults to
// the PostHog EU ingestion endpoint. PR previews are keyless in CI; the /prs/
// base-path marker additionally tags any keyed preview build as "preview".
const posthogKey = process.env.POSTHOG_KEY ?? "";
const posthogHost = process.env.POSTHOG_HOST ?? "https://eu.i.posthog.com";
const analyticsEnvironment = baseUrl.includes("/prs/") ? "preview" : "prod";

// Large immutable assets (the wasm engine, ONNX vision models, walkthrough
// videos) can be offloaded to an external CDN (S3 + CloudFront, cdn.<domain>) to
// keep the GitHub Pages artifact small and the deploy fast. An empty
// DOCS_CDN_URL — the default, and the local-dev case — leaves every asset
// same-origin, so nothing changes until the CDN is configured. DOCS_CDN_VERSION
// cache-busts the per-build wasm under /kapi/wasm/<version>/ and is set only on
// push-to-main (where CI publishes that sha's wasm); empty on PRs/local, so the
// playground serves wasm same-origin (see KapiPlayground/config.ts).
const cdnBaseUrl = process.env.DOCS_CDN_URL ?? "";
const cdnWasmVersion = process.env.DOCS_CDN_VERSION ?? "dev";

// ICU4X (the `icu` npm package, used by the Segmentation Lab) loads its wasm via
// a hardcoded `new URL('icu_capi.wasm', import.meta.url)` — no wasmPaths-style
// override like onnxruntime-web. When the CDN is enabled we rewrite that asset's
// URL to the CDN at build time (cdnIcuWasm plugin); pin the package version so
// the path is immutable and cache-busts on an icu bump. Publish the file with
// `make publish-cdn-icu`.
const icuVersion = (() => {
  // `icu`'s package.json `exports` only declares the "import" entry, so
  // require.resolve("icu") throws ERR_PACKAGE_PATH_NOT_EXPORTED. Read the
  // manifest by path instead (the pnpm node_modules/icu symlink), trying the
  // config dir then the cwd.
  const bases = [typeof __dirname !== "undefined" ? __dirname : "", process.cwd()].filter(Boolean);
  for (const base of bases) {
    try {
      const pkg = path.join(base, "node_modules", "icu", "package.json");
      return JSON.parse(fs.readFileSync(pkg, "utf8")).version as string;
    } catch {
      /* try the next base */
    }
  }
  return "0";
})();

// Vision Lab ONNX model-set version. Pinned in the committed web/models.version
// so bumping it is a reviewable PR diff: that PR's preview then loads the models
// from /kapi/models/vision/<version>/ on the CDN (publish the set there once
// with `make publish-cdn-vision-models`). $DOCS_VISION_MODELS_VERSION overrides
// it for ad-hoc builds. Falls back to "v1" if the file is somehow unreadable.
const cdnModelsVersion = (() => {
  if (process.env.DOCS_VISION_MODELS_VERSION) return process.env.DOCS_VISION_MODELS_VERSION;
  try {
    return fs.readFileSync(path.join(__dirname, "models.version"), "utf8").trim() || "v1";
  } catch {
    return "v1";
  }
})();

// Source-locale integrity is strict; target-locale drift only warns. Applied to
// every link-integrity setting below, so one policy governs them all.
const linkIntegrity = (process.env.DOCUSAURUS_CURRENT_LOCALE ?? "en") === "en" ? "throw" : "warn";

// The tagline lives in brand.json rather than here, so kapi governs it. This
// file is TypeScript and kapi has no reader for it; brand.json is content in a
// format it reads, declared as a collection and checked on every PR. Reading a
// sibling file at config-eval time is the same thing models.version does below.
const brand = JSON.parse(fs.readFileSync(path.join(__dirname, "brand.json"), "utf8")) as {
  tagline: string;
};

const config: Config = {
  title: "neokapi",
  tagline: brand.tagline,
  favicon: "img/favicon.png",

  url: "https://neokapi.github.io",
  baseUrl,

  organizationName: "neokapi",
  projectName: "neokapi",
  trailingSlash: false,

  // Source-locale integrity is strict; target-locale drift only warns.
  //
  // `web/i18n/<lang>/` is a build artefact — generated by the dogfood recipe
  // from the English source, never hand-authored. So a source-side page rename
  // leaves the generated target copies briefly pointing at the old filename.
  // That is ordinary, continuous translation lag, and per CLAUDE.md ("Target-
  // language drift must never block the build") it must not fail a build: the
  // target locale falls back to source until it is regenerated.
  //
  // A single global "throw" made exactly that fail. Renaming an AD in #1444
  // broke the nb build on four generated links while en built clean — a source
  // change blocked by its own translations not having caught up.
  //
  // Docusaurus builds one locale per invocation and exposes which via
  // DOCUSAURUS_CURRENT_LOCALE, so the policy can be expressed directly. English
  // stays strict: a broken link in the source IS a defect and should stop the
  // build.
  onBrokenLinks: linkIntegrity,

  i18n: {
    defaultLocale: "en",
    locales: ["en", "nb", "qps"],
    localeConfigs: {
      en: { label: "English" },
      nb: { label: "Norsk (bokmål)", htmlLang: "nb" },
      // The pseudo-locale probe. Its label is written in the pseudo alphabet, so
      // the switcher demonstrates the transformation it selects: a reader who
      // cannot parse this entry has learned what the build does before clicking
      // it. htmlLang stays "en" because the text IS English, mangled — telling a
      // screen reader otherwise would be a lie.
      qps: { label: "Þšéüđö Éñĝļîšĥ", htmlLang: "en" },
    },
  },

  customFields: {
    cdnBaseUrl,
    cdnSitePrefix: "kapi",
    cdnWasmVersion,
    cdnModelsVersion,
    posthogKey,
    posthogHost,
    analyticsEnvironment,
  },

  // Cookieless pageview analytics (key-gated no-op without POSTHOG_KEY).
  clientModules: ["./src/clientModules/analytics.ts"],

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: linkIntegrity,
      onBrokenMarkdownImages: linkIntegrity,
    },
  },

  themes: ["@docusaurus/theme-mermaid"],

  plugins: [
    // Redirects for pages merged or moved by the desktop-first docs revamp:
    // the Projects concept page absorbs "Projects vs ad-hoc" + "Modes &
    // bindings", and "Use with Claude" absorbs the skills + MCP pages.
    [
      "@docusaurus/plugin-client-redirects",
      {
        redirects: [
          { from: "/kapi/get-started/project-vs-adhoc", to: "/kapi/projects" },
          { from: "/kapi/modes", to: "/kapi/projects" },
          { from: "/kapi/get-started/use-with-skills", to: "/kapi/get-started/use-with-claude" },
          { from: "/kapi/get-started/use-with-mcp", to: "/kapi/get-started/use-with-claude" },
          // The Models & Providers lab was removed (it demonstrated provider
          // APIs, not neokapi functionality); provider setup lives in the
          // Use-with-Claude/models docs, and /labs maps the remaining labs.
          { from: "/lab/models", to: "/labs" },
          // The file-format vocabulary rename (.klf → .kbf → .kbf.json, .klz → .kpz):
          // the *format* is a hard rename with no back-compat, but the
          // published doc URLs are indexed, so the old routes redirect. These
          // "klf" strings are historical route names, not live vocabulary.
          // Both the .klf-era routes and the /reference/kbf/* routes that
          // replaced them now land in /reference/serialization/*, which covers
          // the whole format family rather than the content bundle alone.
          // Redirected straight to the final target rather than chained, so an
          // old inbound link costs one hop.
          { from: "/reference/klf/overview", to: "/reference/serialization/overview" },
          { from: "/reference/klf/spec", to: "/reference/serialization/content-bundle" },
          { from: "/reference/klf/examples", to: "/reference/serialization/content-bundle" },
          { from: "/reference/klf/vs-xliff", to: "/reference/serialization/choosing" },
          { from: "/reference/klf/package", to: "/reference/serialization/project-archive" },
          { from: "/reference/kbf/overview", to: "/reference/serialization/overview" },
          // spec and examples merge: the schema page now carries the worked
          // example, per "spec + annotated example" on one page.
          { from: "/reference/kbf/spec", to: "/reference/serialization/content-bundle" },
          { from: "/reference/kbf/examples", to: "/reference/serialization/content-bundle" },
          { from: "/reference/kbf/vs-xliff", to: "/reference/serialization/choosing" },
          { from: "/reference/kbf/package", to: "/reference/serialization/project-archive" },
          { from: "/reference/formats/klf", to: "/reference/formats/kbf" },
          { from: "/klf-lab", to: "/kbf-lab" },
          { from: "/klf-tests", to: "/kbf-tests" },
          {
            from: "/contribute/architecture/025-klf-package",
            to: "/contribute/architecture/multilingual/m-06-content-packages",
          },
          // The Okapi-parity dashboard was retired with the bridge's product
          // surface (#1073). /format-maturity carries the quality story now, so
          // external inbound links land there instead of 404ing.
          { from: "/parity", to: "/format-maturity" },
          { from: "/parity/fixtures", to: "/format-maturity" },
          // The bridge-specific protocol note became the versioned, plugin-wide
          // protocol v1 specification (#1073 D2).
          {
            from: "/contribute/notes-internal/plugin-bridge-protocol",
            to: "/contribute/implementation/engine/plugin-protocol-v1",
          },
          // The bridge's own page folded into the plugin overview, where it is
          // the canonical Mode-C entry under "Standard plugins".
          { from: "/contribute/java-bridge", to: "/contribute/plugins" },
          // The product-vocabulary rename (translation memory → content memory,
          // termbase/glossary → terms; #1462) and the de-localization of the
          // framing (#1463). The published routes are indexed, so every retired
          // page keeps a redirect. The "tm"/"termbase"/"localization" strings
          // below are historical route names, not live vocabulary.
          { from: "/framework/translation-memory", to: "/framework/content-memory" },
          { from: "/framework/glossary", to: "/framework/concepts" },
          {
            from: "/kapi/recipes/tm-termbase-storage",
            to: "/kapi/recipes/memory-and-terms-storage",
          },
          {
            from: "/kapi/recipes/pre-translate-with-tm",
            to: "/kapi/recipes/pre-translate-from-memory",
          },
          { from: "/kapi/recipes/localize-media", to: "/kapi/recipes/translate-media" },
          {
            from: "/kapi/recipes/gate-localization-in-ci",
            to: "/kapi/recipes/ship-gates-and-ci",
          },
          // The architecture corpus is organized by concern in six series, each
          // a directory. Every retired numeric slug keeps a redirect, and the
          // pre-existing renames below point straight at the final route rather
          // than chaining through one that no longer exists.
          {
            from: "/contribute/architecture/009-translation-memory",
            to: "/contribute/architecture/context/c-09-content-memory",
          },
          {
            from: "/contribute/architecture/008-project-model",
            to: "/contribute/architecture/context/c-01-project-model",
          },
          {
            from: "/contribute/architecture/009-content-memory",
            to: "/contribute/architecture/context/c-09-content-memory",
          },
          {
            from: "/contribute/architecture/010-terminology",
            to: "/contribute/architecture/context/c-08-terms",
          },
          {
            from: "/contribute/architecture/020-redaction",
            to: "/contribute/architecture/context/c-10-redaction",
          },
          {
            from: "/contribute/architecture/022-voice-profile",
            to: "/contribute/architecture/context/c-07-voice-profiles",
          },
          {
            from: "/contribute/architecture/033-project-state-model",
            to: "/contribute/architecture/context/c-04-unit-state-and-decisions",
          },
          {
            from: "/contribute/architecture/037-context-retrieval-surface",
            to: "/contribute/architecture/context/c-06-retrieval",
          },
          {
            from: "/contribute/architecture/039-local-context-graph-store",
            to: "/contribute/architecture/context/c-03-context-store-and-graph",
          },
          {
            from: "/contribute/architecture/001-vision-and-modules",
            to: "/contribute/architecture/foundations/f-01-framework-and-modules",
          },
          {
            from: "/contribute/architecture/002-content-model",
            to: "/contribute/architecture/foundations/f-02-content-model",
          },
          {
            from: "/contribute/architecture/003-identity",
            to: "/contribute/architecture/foundations/f-03-identity",
          },
          {
            from: "/contribute/architecture/034-content-model-wire-schema",
            to: "/contribute/architecture/foundations/f-04-wire-schema",
          },
          {
            from: "/contribute/architecture/004-processing-engine",
            to: "/contribute/architecture/engine/e-01-processing-engine",
          },
          {
            from: "/contribute/architecture/005-format-system",
            to: "/contribute/architecture/engine/e-02-format-system",
          },
          // Content-fidelity surfacing is a reader behaviour, so it is a section
          // of the format system rather than a decision of its own.
          {
            from: "/contribute/architecture/031-content-fidelity-surfacing",
            to: "/contribute/architecture/engine/e-02-format-system",
          },
          {
            from: "/contribute/architecture/006-tool-system",
            to: "/contribute/architecture/engine/e-03-tool-system",
          },
          {
            from: "/contribute/architecture/026-flow-io-binding",
            to: "/contribute/architecture/engine/e-04-flows-and-io-binding",
          },
          {
            from: "/contribute/architecture/007-plugin-system",
            to: "/contribute/architecture/engine/e-05-plugin-system",
          },
          {
            from: "/contribute/architecture/038-execution-trust",
            to: "/contribute/architecture/engine/e-06-execution-trust",
          },
          // The model and machine-translation provider decisions are one
          // decision: two interfaces reached the same way.
          {
            from: "/contribute/architecture/011-ai-providers",
            to: "/contribute/architecture/engine/e-07-model-providers",
          },
          {
            from: "/contribute/architecture/012-mt-providers",
            to: "/contribute/architecture/engine/e-07-model-providers",
          },
          {
            from: "/contribute/architecture/028-pdf-reader-plugin",
            to: "/contribute/architecture/engine/e-08-document-structure-tiers",
          },
          {
            from: "/contribute/architecture/013-kapi-cli",
            to: "/contribute/architecture/surfaces/s-01-kapi-cli",
          },
          {
            from: "/contribute/architecture/014-kapi-desktop",
            to: "/contribute/architecture/surfaces/s-02-kapi-desktop",
          },
          {
            from: "/contribute/architecture/024-agent-skills",
            to: "/contribute/architecture/surfaces/s-03-agent-surfaces",
          },
          {
            from: "/contribute/architecture/023-toolbox-utilities",
            to: "/contribute/architecture/surfaces/s-04-toolbox",
          },
          {
            from: "/contribute/architecture/019-i18n-react",
            to: "/contribute/architecture/surfaces/s-05-i18n-runtime",
          },
          // In-context review is what the runtime does in a dev server, so it
          // is a section of the runtime decision.
          {
            from: "/contribute/architecture/035-in-context-review",
            to: "/contribute/architecture/surfaces/s-05-i18n-runtime",
          },
          {
            from: "/contribute/architecture/027-visual-editor-data-model",
            to: "/contribute/architecture/surfaces/s-06-visual-editor",
          },
          {
            from: "/contribute/architecture/017-bilingual-format-interop",
            to: "/contribute/architecture/multilingual/m-01-bilingual-interop",
          },
          // The segmenter plugin is one engine tier of segmentation, which now
          // has a decision covering the whole concern.
          {
            from: "/contribute/architecture/021-sat-segmenter-plugin",
            to: "/contribute/architecture/multilingual/m-02-segmentation",
          },
          {
            from: "/contribute/architecture/029-vision-and-image-adaptation",
            to: "/contribute/architecture/multilingual/m-03-multimodal-content",
          },
          {
            from: "/contribute/architecture/030-multimodal-extraction-and-llm-refinement",
            to: "/contribute/architecture/multilingual/m-03-multimodal-content",
          },
          {
            from: "/contribute/architecture/032-math-and-equations",
            to: "/contribute/architecture/multilingual/m-04-math-and-equations",
          },
          {
            from: "/contribute/architecture/036-llm-prompts-and-batching",
            to: "/contribute/architecture/multilingual/m-05-prompts-and-batching",
          },
          {
            from: "/contribute/architecture/025-kbf-package",
            to: "/contribute/architecture/multilingual/m-06-content-packages",
          },
          {
            from: "/contribute/architecture/016-metadata-i18n",
            to: "/contribute/architecture/multilingual/m-07-metadata-i18n",
          },
          {
            from: "/contribute/architecture/015-testing-and-documentation",
            to: "/contribute/architecture/assurance/a-01-testing-and-documentation",
          },
          {
            from: "/contribute/architecture/018-parity-testing",
            to: "/contribute/architecture/assurance/a-02-parity",
          },
          {
            from: "/contribute/architecture/029-vision-and-image-localization",
            to: "/contribute/architecture/multilingual/m-03-multimodal-content",
          },
          {
            from: "/contribute/notes-internal/multimodal-localization",
            to: "/contribute/implementation/multilingual/multimodal-content",
          },
          {
            from: "/contribute/notes-internal/tm-matching-algorithm",
            to: "/contribute/implementation/context/memory-matching-algorithm",
          },
          // Generated command-reference pages retired with the `kapi tm` →
          // `kapi memory` and `kapi termbase` → `kapi terms` verb rename
          // (#1462, which renamed the pages without redirecting the old URLs).
          { from: "/reference/commands/tm", to: "/reference/commands/memory" },
          { from: "/reference/commands/tm-audit", to: "/reference/commands/memory-audit" },
          { from: "/reference/commands/tm-export", to: "/reference/commands/memory-export" },
          { from: "/reference/commands/tm-import", to: "/reference/commands/memory-import" },
          {
            from: "/reference/commands/tm-import-dir",
            to: "/reference/commands/memory-import-dir",
          },
          { from: "/reference/commands/tm-list", to: "/reference/commands/memory-list" },
          { from: "/reference/commands/tm-lookup", to: "/reference/commands/memory-lookup" },
          { from: "/reference/commands/tm-search", to: "/reference/commands/memory-search" },
          { from: "/reference/commands/tm-stats", to: "/reference/commands/memory-stats" },
          {
            from: "/reference/commands/tm-sessions",
            to: "/reference/commands/memory-sessions",
          },
          {
            from: "/reference/commands/tm-sessions-list",
            to: "/reference/commands/memory-sessions-list",
          },
          {
            from: "/reference/commands/tm-sessions-show",
            to: "/reference/commands/memory-sessions-show",
          },
          {
            from: "/reference/commands/tm-sessions-delete",
            to: "/reference/commands/memory-sessions-delete",
          },
          { from: "/reference/commands/termbase", to: "/reference/commands/terms" },
          {
            from: "/reference/commands/termbase-export",
            to: "/reference/commands/terms-export",
          },
          {
            from: "/reference/commands/termbase-import",
            to: "/reference/commands/terms-import",
          },
          { from: "/reference/commands/termbase-list", to: "/reference/commands/terms-list" },
          {
            from: "/reference/commands/termbase-lookup",
            to: "/reference/commands/terms-lookup",
          },
          {
            from: "/reference/commands/termbase-search",
            to: "/reference/commands/terms-search",
          },
          {
            from: "/reference/commands/termbase-stats",
            to: "/reference/commands/terms-stats",
          },
          // The notes are grouped into subdirectories mirroring the architecture
          // series (F/E/C/S/M) plus a repo-infrastructure group, so a note sits
          // in the sidebar under the same heading as the AD it details. Each
          // note's former flat route redirects to its grouped one.
          //
          // These entries carry the redirect chain, they do not extend it:
          // client-redirects resolves one hop, so a route that already had a
          // predecessor (notes-internal, below) points at the grouped path
          // directly rather than at the flat path that no longer resolves.
          {
            from: "/contribute/implementation/content-parity",
            to: "/contribute/implementation/foundations/content-parity",
          },
          {
            from: "/contribute/implementation/implementing-formats",
            to: "/contribute/implementation/engine/implementing-formats",
          },
          {
            from: "/contribute/implementation/skeleton-store",
            to: "/contribute/implementation/engine/skeleton-store",
          },
          {
            from: "/contribute/implementation/streaming-tree-formats",
            to: "/contribute/implementation/engine/streaming-tree-formats",
          },
          {
            from: "/contribute/implementation/content-fidelity",
            to: "/contribute/implementation/engine/content-fidelity",
          },
          {
            from: "/contribute/implementation/flow-steps-format",
            to: "/contribute/implementation/engine/flow-steps-format",
          },
          {
            from: "/contribute/implementation/session-tool-authoring",
            to: "/contribute/implementation/engine/session-tool-authoring",
          },
          {
            from: "/contribute/implementation/tool-data-model-redesign",
            to: "/contribute/implementation/engine/tool-data-model-redesign",
          },
          {
            from: "/contribute/implementation/plugin-model",
            to: "/contribute/implementation/engine/plugin-model",
          },
          {
            from: "/contribute/implementation/plugin-protocol-v1",
            to: "/contribute/implementation/engine/plugin-protocol-v1",
          },
          {
            from: "/contribute/implementation/kapi-project-file",
            to: "/contribute/implementation/context/kapi-project-file",
          },
          {
            from: "/contribute/implementation/terminology-data-model",
            to: "/contribute/implementation/context/terminology-data-model",
          },
          {
            from: "/contribute/implementation/memory-matching-algorithm",
            to: "/contribute/implementation/context/memory-matching-algorithm",
          },
          {
            from: "/contribute/implementation/cli-conventions",
            to: "/contribute/implementation/surfaces/cli-conventions",
          },
          {
            from: "/contribute/implementation/mcp-tools-reference",
            to: "/contribute/implementation/surfaces/mcp-tools-reference",
          },
          {
            from: "/contribute/implementation/wasm-engine-abi",
            to: "/contribute/implementation/surfaces/wasm-engine-abi",
          },
          {
            from: "/contribute/implementation/multimodal-content",
            to: "/contribute/implementation/multilingual/multimodal-content",
          },
          {
            from: "/contribute/implementation/omml-math",
            to: "/contribute/implementation/multilingual/omml-math",
          },
          {
            from: "/contribute/implementation/cdn-assets",
            to: "/contribute/implementation/repo/cdn-assets",
          },
          {
            from: "/contribute/implementation/markdown-in-ui",
            to: "/contribute/implementation/repo/markdown-in-ui",
          },
          // notes-internal/ → implementation/. The pages stay public — publishing
          // implementation detail is deliberate for an open-source engine — but
          // the old directory name told readers the content was internal, which
          // is neither true nor useful. Every one of the published routes keeps
          // a redirect; the entries immediately below mirror the files in
          // web/docs/contribute/implementation/ one-for-one, and the four whose
          // note has since left this site are grouped separately further down.
          // The bare /contribute/notes-internal covers the directory-index form.
          { from: "/contribute/notes-internal", to: "/contribute/implementation/index" },
          {
            from: "/contribute/notes-internal/cdn-assets",
            to: "/contribute/implementation/repo/cdn-assets",
          },
          {
            from: "/contribute/notes-internal/cli-conventions",
            to: "/contribute/implementation/surfaces/cli-conventions",
          },
          {
            from: "/contribute/notes-internal/content-fidelity",
            to: "/contribute/implementation/engine/content-fidelity",
          },
          {
            from: "/contribute/notes-internal/content-parity",
            to: "/contribute/implementation/foundations/content-parity",
          },
          {
            from: "/contribute/notes-internal/flow-steps-format",
            to: "/contribute/implementation/engine/flow-steps-format",
          },
          {
            from: "/contribute/notes-internal/implementing-formats",
            to: "/contribute/implementation/engine/implementing-formats",
          },
          { from: "/contribute/notes-internal/index", to: "/contribute/implementation/index" },
          {
            from: "/contribute/notes-internal/kapi-project-file",
            to: "/contribute/implementation/context/kapi-project-file",
          },
          {
            from: "/contribute/notes-internal/markdown-in-ui",
            to: "/contribute/implementation/repo/markdown-in-ui",
          },
          {
            from: "/contribute/notes-internal/mcp-tools-reference",
            to: "/contribute/implementation/surfaces/mcp-tools-reference",
          },
          {
            from: "/contribute/notes-internal/memory-matching-algorithm",
            to: "/contribute/implementation/context/memory-matching-algorithm",
          },
          {
            from: "/contribute/notes-internal/multimodal-content",
            to: "/contribute/implementation/multilingual/multimodal-content",
          },
          {
            from: "/contribute/notes-internal/omml-math",
            to: "/contribute/implementation/multilingual/omml-math",
          },
          {
            from: "/contribute/notes-internal/plugin-model",
            to: "/contribute/implementation/engine/plugin-model",
          },
          {
            from: "/contribute/notes-internal/plugin-protocol-v1",
            to: "/contribute/implementation/engine/plugin-protocol-v1",
          },
          {
            from: "/contribute/notes-internal/session-tool-authoring",
            to: "/contribute/implementation/engine/session-tool-authoring",
          },
          {
            from: "/contribute/notes-internal/skeleton-store",
            to: "/contribute/implementation/engine/skeleton-store",
          },
          {
            from: "/contribute/notes-internal/streaming-tree-formats",
            to: "/contribute/implementation/engine/streaming-tree-formats",
          },
          {
            from: "/contribute/notes-internal/terminology-data-model",
            to: "/contribute/implementation/context/terminology-data-model",
          },
          {
            from: "/contribute/notes-internal/tool-data-model-redesign",
            to: "/contribute/implementation/engine/tool-data-model-redesign",
          },
          {
            from: "/contribute/notes-internal/wasm-engine-abi",
            to: "/contribute/implementation/surfaces/wasm-engine-abi",
          },
          // Four notes documented the hosted platform rather than the framework
          // — its product-analytics taxonomy, the nightly convergence of this
          // repo's own content against it, a review of the loop documentation
          // across both sites, and the messaging canon. This site publishes
          // framework documentation, so they are no longer part of it. Their
          // routes were indexed (and two of them also had a notes-internal
          // predecessor route), so every one lands on the implementation index
          // rather than 404ing.
          {
            from: "/contribute/implementation/analytics-events",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/implementation/dogfood-sync",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/implementation/loop-docs-review",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/implementation/positioning",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/notes-internal/analytics-events",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/notes-internal/dogfood-sync",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/notes-internal/loop-docs-review",
            to: "/contribute/implementation/index",
          },
          {
            from: "/contribute/notes-internal/positioning",
            to: "/contribute/implementation/index",
          },
          // Workspace paths was contributor machine setup (NEOKAPI_* variables
          // and the absolute-path guard), not product documentation, so it was
          // unpublished and now lives in the repository at
          // docs/internals/workspace-paths.md. The route was indexed, so it
          // redirects to the file's new home rather than 404ing. An absolute
          // "to" is passed through verbatim by the redirects plugin (only
          // values starting with "/" are resolved against site routes).
          {
            from: "/contribute/workspace-paths",
            to: "https://github.com/neokapi/neokapi/blob/main/docs/internals/workspace-paths.md",
          },
        ],
      },
    ],
    // Automatic i18n for this site's own React components: every JSX text
    // child becomes translatable with no wrapper to forget. Loaded by path
    // because this config is evaluated by jiti, where the plugin's ESM dist
    // cannot run — see the file for the detail.
    "./plugins/neokapi-i18n.mjs",

    // Silence a few benign third-party webpack warnings. Each predicate is
    // scoped to the specific offending module/message so an equivalent warning
    // from our OWN code is never suppressed.
    function ignoreWebpackWarnings() {
      return {
        name: "ignore-webpack-warnings",
        configureWebpack() {
          return {
            ignoreWarnings: [
              // The UMD wrapper in vscode-languageserver-types (pulled in via
              // @docusaurus/theme-mermaid → mermaid → langium) flags its own
              // CommonJS/AMD format-detection `require`, not a missing dependency.
              (warning: { message?: string; module?: { resource?: string } }) =>
                /Critical dependency: require function is used/.test(warning.message ?? "") &&
                /[\\/]node_modules[\\/]vscode-languageserver-types[\\/]/.test(
                  warning.module?.resource ?? "",
                ),
              // @ffmpeg/ffmpeg (the WASM ffmpeg behind the media/video lab) loads
              // its worker + core via `new Worker(new URL(…))` / dynamic import
              // that webpack can't statically analyze. Benign — the URL is real
              // at runtime.
              (warning: { message?: string; module?: { resource?: string } }) =>
                /Critical dependency: the request of a dependency is an expression/.test(
                  warning.message ?? "",
                ) && /[\\/]@ffmpeg[\\/]ffmpeg[\\/]/.test(warning.module?.resource ?? ""),
              // The same ffmpeg/emscripten build emits a circular chunk dependency
              // for its pthread runtime (em-pthread ↔ runtime~main). Inherent to
              // emscripten's threaded WASM output; harmless here.
              (warning: { message?: string }) =>
                /Circular dependency between chunks with runtime.*em-pthread/.test(
                  warning.message ?? "",
                ),
              // onnxruntime-web (the WASM ML runtime behind the vision/segmentation
              // labs) flags a `require` in its node-target build that the browser
              // build never reaches. Benign — scoped to onnxruntime-web's path.
              (warning: { message?: string; module?: { resource?: string } }) =>
                /Critical dependency: require function is used/.test(warning.message ?? "") &&
                /[\\/]onnxruntime-web[\\/]/.test(warning.module?.resource ?? ""),
            ],
          };
        },
      };
    },
    // Enable the webpack experiments the ICU4X (`icu`) npm package needs: it
    // imports its wasm via `new URL('icu_capi.wasm', import.meta.url)` and uses
    // top-level await. The segmentation lab dynamic-imports `icu` on the client
    // only (it's SSR-fragile), but the bundler must still permit async WASM +
    // top-level await for that chunk to build.
    function icu4xWasm() {
      return {
        name: "icu4x-wasm-experiments",
        configureWebpack(_config: unknown, isServer: boolean) {
          return {
            experiments: { asyncWebAssembly: true, topLevelAwait: true },
            ...(isServer
              ? // `icu` is ESM-only (no require/node export condition), so the
                // SSR/node webpack build can't resolve it. It's dynamic-imported
                // client-only (the segmentation lab is BrowserOnly), so externalize
                // it on the server: never resolved or executed there. `gliner`
                // (the lab's on-device NER, via onnxruntime-web) is in the same
                // boat: browser-only export conditions and bundled wasm binaries
                // that don't resolve under node, loaded by a BrowserOnly
                // dynamic import. `@huggingface/transformers` (the Vision Lab's
                // TrOCR handwriting fallback) is the same: its Node build pulls in
                // `sharp` + native `.node` binaries webpack can't parse, but it's
                // only ever dynamic-imported on the client.
                { externals: ["icu", "gliner", "@huggingface/transformers"] }
              : // On the client, `icu`'s loader has a Node branch importing `fs`;
                // the browser branch uses fetch, so stub the Node builtins out.
                { resolve: { fallback: { fs: false, path: false } } }),
          };
        },
      };
    },
    // Run Tailwind v4 through Docusaurus's PostCSS pipeline so the reference
    // pages (/formats, /tools) can render the shared ui-primitives SchemaForm.
    // The tailwind.css customCss entry imports Tailwind WITHOUT preflight and
    // scopes color tokens to `.kapi-reference`, leaving Infima/normal docs
    // pages untouched.
    function tailwindPostCss() {
      return {
        name: "tailwind-postcss",
        configurePostCss(postcssOptions: { plugins: unknown[] }) {
          // eslint-disable-next-line @typescript-eslint/no-require-imports
          postcssOptions.plugins.push(require("@tailwindcss/postcss"));
          return postcssOptions;
        },
      };
    },
    // Keep the dev-server's red error overlay for REAL runtime errors, but
    // drop the benign Chrome "ResizeObserver loop completed with undelivered
    // notifications" report — it means an observer skipped a frame (React
    // Flow and the lab's panel resizes trigger it routinely), not that
    // anything failed. Docusaurus merges a webpack config's `devServer` into
    // its own dev-server options (start/webpack.js).
    function devServerOverlayFilter() {
      return {
        name: "dev-server-overlay-filter",
        // Docusaurus merges this into the dev-server config; `devServer` isn't on
        // webpack's base Configuration type (webpack-dev-server augments it), so
        // widen the return type to carry it.
        configureWebpack(): import("webpack").Configuration & { devServer?: unknown } {
          return {
            devServer: {
              client: {
                overlay: {
                  errors: true,
                  warnings: false,
                  runtimeErrors: (error: Error) =>
                    !/ResizeObserver loop (completed with undelivered notifications|limit exceeded)/.test(
                      error?.message ?? "",
                    ),
                },
              },
            },
          };
        },
      };
    },
    // The Vision Lab's ONNX models (~150 MB) live in static/models/vision, which
    // Docusaurus copies into EVERY locale's output — doubling them on the GitHub
    // Pages site (a real size problem; Pages builds were failing). The lab fetches
    // them from the default-locale (root) path regardless of locale, so the
    // per-locale copies are dead weight. Drop them from non-default locale builds.
    function dropLocaleVisionModels(context: {
      i18n: { currentLocale: string; defaultLocale: string };
    }) {
      return {
        name: "drop-locale-vision-models",
        async postBuild({ outDir }: { outDir: string }) {
          if (context.i18n.currentLocale === context.i18n.defaultLocale) return;
          await fs.promises.rm(path.join(outDir, "models", "vision"), {
            recursive: true,
            force: true,
          });
        },
      };
    },
    // Drop wasm that webpack emits but the runtime never fetches from the bundle:
    //
    //   • onnxruntime-web (Vision Lab OCR via kapi-playground's visionBridge, and
    //     the TrOCR handwriting fallback via @huggingface/transformers) references
    //     every wasm variant (jsep/asyncify/jspi/simd-threaded) via
    //     `new URL('ort-*.wasm', import.meta.url)` — ~100 MB, ×2 locales. But
    //     visionBridge sets `ort.env.wasm.wasmPaths` to the jsdelivr CDN, so the
    //     emitted copies are never loaded.
    //   • @embedpdf/pdfium (PDF Lab) — pdfiumBridge `fetch()`es pdfium.wasm from an
    //     explicit URL and passes the bytes to `init({ wasmBinary })`, so Emscripten
    //     never uses the bundled `new URL` reference.
    //
    // Keep the URL references resolving (so the build doesn't break) but skip
    // WRITING the files (asset/resource + generator.emit:false). Scoped by path so
    // ICU4X's wasm — which the segmentation lab DOES load same-origin from the
    // emitted file — is untouched.
    function dropUnusedBundledWasm() {
      return {
        name: "drop-unused-bundled-wasm",
        configureWebpack() {
          return {
            module: {
              rules: [
                {
                  test: /\.wasm$/,
                  include: [/[\\/]onnxruntime-web[\\/]/, /[\\/]@embedpdf[\\/]pdfium[\\/]/],
                  type: "asset/resource",
                  generator: { emit: false },
                },
              ],
            },
          };
        },
      };
    },
    // ICU4X wasm (Segmentation Lab). The `icu` package hardcodes
    // `new URL('icu_capi.wasm', import.meta.url)`, so — unlike onnxruntime-web —
    // there's no runtime path hook. When the CDN is enabled, rewrite the emitted
    // asset's URL to the CDN (kapi/icu/<version>/icu_capi.wasm) and skip writing
    // the ~16 MB file; otherwise it stays same-origin (local dev unchanged). The
    // CDN object must exist first (`make publish-cdn-icu`) or the lab 404s; it is
    // served with Content-Type application/wasm so instantiateStreaming accepts it.
    function cdnIcuWasm() {
      return {
        name: "cdn-icu-wasm",
        configureWebpack() {
          if (!cdnBaseUrl) return {};
          return {
            module: {
              rules: [
                {
                  test: /icu_capi\.wasm$/,
                  type: "asset/resource",
                  generator: {
                    emit: false,
                    filename: "icu_capi.wasm",
                    publicPath: `${cdnBaseUrl}/kapi/icu/${icuVersion}/`,
                  },
                },
              ],
            },
          };
        },
      };
    },
  ],

  presets: [
    [
      "classic",
      {
        docs: {
          // routeBasePath "/" puts docs at the root of the Docusaurus
          // instance, which itself is mounted at baseUrl. In production
          // (baseUrl "/") URLs end up as /{topic}, and the home page
          // (src/pages/index.tsx) sits at the site root /.
          routeBasePath: "/",
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/neokapi/neokapi/tree/main/web/",
        },
        blog: false,
        theme: {
          customCss: ["./src/css/custom.css", "./src/css/tailwind.css"],
        },
        // @docusaurus/plugin-sitemap is bundled in preset-classic. Explicit
        // config here activates it and sets the change frequency hint that
        // search-engine crawlers use when deciding how often to re-index pages.
        sitemap: {
          changefreq: "weekly",
          priority: 0.5,
          ignorePatterns: [],
          filename: "sitemap.xml",
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    // Open Graph + Twitter Card metadata for social sharing.
    // og-card.png is a 1200×630 (1.91:1) banner — the standard social-card
    // aspect ratio — so Slack/Twitter/LinkedIn render a tidy landscape preview
    // with the mascot at a natural size, rather than blowing up the square
    // hero-logo.png full-bleed. Individual pages can override via their own
    // frontmatter `image:` field.
    image: "img/og-card.png",
    metadata: [
      { name: "twitter:card", content: "summary_large_image" },
      { name: "twitter:site", content: "@neokapi" },
      {
        name: "description",
        content:
          "neokapi is an open-source, format-aware content engine in Go. It parses any format into one unified content model, resolves the context that applies to the content inside it, lets you or your AI agent edit and check it, and writes it back byte-for-byte. The same engine makes that content work in every language.",
      },
    ],
    navbar: {
      title: "neokapi",
      logo: {
        alt: "neokapi",
        src: "img/logo.png",
        // The logo links to the home page (src/pages/index.tsx), which is now the
        // product landing page at the site root.
        href: "/",
      },
      items: [
        // IA: Kapi is the product (getting started + CLI + Desktop + recipes +
        // projects); Framework is the engine (an Overview, then concepts +
        // extending + architecture + notes); Reference holds the
        // generated/interactive references + MCP; Toolbox holds the CLI utilities
        // and neokapi-i18n.
        {
          type: "docSidebar",
          sidebarId: "kapiSidebar",
          label: "Kapi",
          position: "left",
        },
        {
          type: "dropdown",
          label: "Labs",
          position: "left",
          items: [
            // A Labs overview heads the list (what each lab teaches + a
            // suggested order). AI/ML (local LLM, OCR, ASR) is embedded inside
            // the relevant labs; plugins load on demand from the navbar status
            // widget. Old per-topic routes redirect to their new home.
            { label: "Labs overview", to: "/labs" },
            { label: "Content Model Workspace", to: "/lab" },
            { label: "Segmentation", to: "/lab/segmentation" },
            { label: "File Conversion", to: "/lab/convert" },
            { label: "Structure & Layout", to: "/lab/structure" },
            { label: "Vision", to: "/lab/vision" },
            { label: "Audio & Video", to: "/lab/media" },
            { label: "CLI Playground", to: "/playground-cli" },
            { label: "KBF Anatomy", to: "/kbf-lab" },
          ],
        },
        {
          type: "dropdown",
          label: "Reference",
          position: "left",
          items: [
            // Reader-facing reference only. The individual dashboards (parity,
            // benchmarks, per-eval results) stay out of this dropdown: one of
            // them is a slice of telemetry, not something a reader arrives
            // looking for.
            //
            // /evals is the exception, and the reason is that it is not a
            // dashboard. It is the cover page over all of them, and the only
            // surface that can say what is NOT measured, which is the half a
            // reader cannot reconstruct by browsing the others.
            { label: "Reference Overview", to: "/reference" },
            { label: "Tests and Evals", to: "/evals" },
            { label: "Kapi CLI Commands", to: "/commands" },
            { label: "Formats", to: "/formats" },
            { label: "Tools", to: "/tools" },
            { label: "AI Models", to: "/models" },
            { label: "Project file", to: "/reference/project-file" },
            { label: "kapi serialization formats", to: "/reference/serialization/overview" },
            { label: "MCP Server", to: "/reference/mcp" },
            { label: "Scripting & JSON contract", to: "/reference/cli-contract" },
            { label: "Engine service (gRPC)", to: "/reference/engine-service" },
            { label: "Telemetry", to: "/reference/telemetry" },
          ],
        },
        {
          // Toolbox holds the format-aware CLI utilities and neokapi-i18n.
          // neokapi-i18n is a self-contained library — its own thing, kept out
          // of the core Kapi narrative — so it lives here, not in the Kapi
          // section.
          type: "dropdown",
          label: "Toolbox",
          position: "left",
          items: [
            {
              // Format-aware CLI utilities (kgrep/ksed/kcat/kconv/kdiff).
              type: "docSidebar",
              sidebarId: "toolboxSidebar",
              label: "CLI tools",
            },
            {
              type: "docSidebar",
              sidebarId: "reactSidebar",
              label: "neokapi-i18n",
            },
          ],
        },
        {
          // The engine internals — demoted behind the product sections: readers
          // arrive for Kapi; contributors and embedders find the framework here.
          type: "docSidebar",
          sidebarId: "frameworkSidebar",
          label: "Framework",
          position: "left",
        },
        {
          // Neokapi WebAssembly Lab status widget — engine + plugin state for
          // this browser tab, with explicit per-plugin Download (custom type
          // registered in src/theme/NavbarItem/ComponentTypes.tsx).
          type: "custom-kapiStatus",
          position: "right",
        },
        {
          type: "localeDropdown",
          position: "right",
          // Icon-only: the translate glyph stands in for the active-locale label
          // (see .navbar-locale-icon in custom.css); the menu still lists locales.
          className: "navbar-locale-icon",
        },
        {
          href: "https://github.com/neokapi/neokapi",
          position: "right",
          // Icon-only GitHub link (mask icon via .header-github-link in custom.css).
          className: "header-github-link",
          "aria-label": "GitHub repository",
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          title: "Documentation",
          items: [
            {
              label: "Get started",
              to: "/kapi/get-started/quickstart",
            },
            {
              label: "Kapi",
              to: "/kapi/overview",
            },
            {
              label: "Framework",
              to: "/framework/architecture",
            },
            {
              label: "Kapi CLI",
              to: "/kapi/cli",
            },
            {
              label: "CLI tools",
              to: "/toolbox/overview",
            },
            {
              label: "neokapi-i18n",
              to: "/react/introduction",
            },
            {
              label: "Reference",
              to: "/reference",
            },
            {
              label: "Format Reference",
              to: "/formats",
            },
          ],
        },
        {
          title: "More",
          items: [
            {
              label: "GitHub",
              href: "https://github.com/neokapi/neokapi",
            },
            {
              label: "Homebrew Tap",
              href: "https://github.com/neokapi/homebrew-tap",
            },
          ],
        },
      ],
      copyright: `Copyright \u00a9 ${new Date().getFullYear()} neokapi contributors. Built with Docusaurus. \u00b7 built ${buildStamp}`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ["go", "protobuf", "yaml", "bash", "json"],
    },
    mermaid: {
      theme: { light: "neutral", dark: "dark" },
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
