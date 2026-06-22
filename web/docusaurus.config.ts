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

// Large immutable assets (the wasm engine, ONNX vision models, walkthrough
// videos) can be offloaded to an external CDN (Cloudflare R2) to keep the
// GitHub Pages artifact small and the deploy fast. An empty DOCS_CDN_URL — the
// default, and the local-dev case — leaves every asset same-origin, so nothing
// changes until the CDN is configured. DOCS_CDN_VERSION cache-busts the
// per-build wasm under /kapi/wasm/<version>/.
const cdnBaseUrl = process.env.DOCS_CDN_URL ?? "";
const cdnWasmVersion = process.env.DOCS_CDN_VERSION ?? "dev";

// ICU4X (the `icu` npm package, used by the Segmentation Lab) loads its wasm via
// a hardcoded `new URL('icu_capi.wasm', import.meta.url)` — no wasmPaths-style
// override like onnxruntime-web. When the CDN is enabled we rewrite that asset's
// URL to R2 at build time (cdnIcuWasm plugin); pin the package version so the R2
// path is immutable and cache-busts on an icu bump. Publish the file with
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

const config: Config = {
  title: "neokapi",
  tagline: "The open-source, format-aware content engine — for people and AI agents",
  favicon: "img/favicon.png",

  url: "https://neokapi.github.io",
  baseUrl,

  organizationName: "neokapi",
  projectName: "neokapi",
  trailingSlash: false,

  onBrokenLinks: "throw",

  i18n: {
    defaultLocale: "en",
    locales: ["en", "nb"],
    localeConfigs: {
      en: { label: "English" },
      nb: { label: "Norsk (bokmål)", htmlLang: "nb" },
    },
  },

  customFields: {
    cdnBaseUrl,
    cdnSitePrefix: "kapi",
    cdnWasmVersion,
    cdnModelsVersion,
  },

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: "warn",
      onBrokenMarkdownImages: "warn",
    },
  },

  themes: ["@docusaurus/theme-mermaid"],

  // The framework-first IA restructure (issue #670) moved pages freely. The
  // site is not yet live, so no client-side redirects are kept — old URLs are
  // simply gone.
  plugins: [
    // Cloudflare Web Analytics: inject the beacon script just before </body> on
    // every page (postBodyTags renders at the end of <body>). Gated to production
    // builds so it ships on the deployed site + PR previews but not in local
    // `vp run start` dev, where the beacon would report nothing useful.
    function cloudflareWebAnalytics() {
      return {
        name: "cloudflare-web-analytics",
        injectHtmlTags() {
          if (process.env.NODE_ENV !== "production") return {};
          return {
            postBodyTags: [
              {
                tagName: "script",
                attributes: {
                  defer: true,
                  src: "https://static.cloudflareinsights.com/beacon.min.js",
                  "data-cf-beacon": '{"token": "3b1c27d17cee44beb47518685678a1e6"}',
                },
              },
            ],
          };
        },
      };
    },
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
    // asset's URL to R2 (kapi/icu/<version>/icu_capi.wasm) and skip writing the
    // ~16 MB file; otherwise it stays same-origin (local dev unchanged). The R2
    // object must exist first (`make publish-cdn-icu`) or the lab 404s. R2 serves
    // it with Content-Type application/wasm so instantiateStreaming accepts it.
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
    // The hero-logo.png is used as the default og:image — it is a wide,
    // high-contrast image suitable for link previews. Individual pages can
    // override this via their own frontmatter `image:` field.
    image: "img/hero-logo.png",
    metadata: [
      { name: "twitter:card", content: "summary_large_image" },
      { name: "twitter:site", content: "@neokapi" },
      {
        name: "description",
        content:
          "neokapi is an open-source, format-aware content engine in Go. It parses any format into one unified content model, lets you or your AI agent edit and check the content inside it, and writes it back byte-for-byte. The same engine makes that content work in every language.",
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
        // and Kapi React.
        {
          type: "docSidebar",
          sidebarId: "kapiSidebar",
          label: "Kapi",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "frameworkSidebar",
          label: "Framework",
          position: "left",
        },
        {
          type: "dropdown",
          label: "Labs",
          position: "left",
          items: [
            // Consolidated into natural categories. AI/ML (local LLM, OCR, ASR)
            // is embedded inside the relevant labs rather than split into its own
            // Gemma/Multimodal pages; plugins load on demand from the navbar
            // status widget. Old per-topic routes redirect to their new home.
            { label: "Core Framework", to: "/lab" },
            { label: "Models & Providers", to: "/lab/models" },
            { label: "Segmentation", to: "/lab/segmentation" },
            { label: "File Conversion", to: "/lab/convert" },
            { label: "Structure & Layout", to: "/lab/structure" },
            { label: "Kapi Vision", to: "/lab/vision" },
            { label: "Audio & Video", to: "/lab/media" },
            { label: "CLI Playground", to: "/playground-cli" },
            { label: "Kapi L10N Format", to: "/klf-lab" },
          ],
        },
        {
          type: "dropdown",
          label: "Reference",
          position: "left",
          items: [
            // Kept in sync with the /reference overview page and the
            // referenceSidebar in sidebars.ts — all three list the same set.
            // Generated, runnable references + interactive grids; R4 fills the
            // per-entry pages under /reference/{commands,formats,tools}/.
            { label: "Reference Overview", to: "/reference" },
            { label: "Kapi CLI Commands", to: "/commands" },
            { label: "Formats", to: "/formats" },
            { label: "Tools", to: "/tools" },
            { label: "Project file", to: "/reference/project-file" },
            { label: "KLF format", to: "/reference/klf/overview" },
            { label: "MCP Server", to: "/reference/mcp" },
            { label: "Parity", to: "/parity" },
            { label: "Format Maturity", to: "/format-maturity" },
            { label: "Benchmarks", to: "/pseudobench" },
            { label: "ML Benchmark", to: "/ml-benchmark" },
            { label: "Check Eval", to: "/check-eval" },
            { label: "Test Results", to: "/test-comparison" },
          ],
        },
        {
          type: "dropdown",
          label: "Toolbox",
          position: "left",
          items: [
            {
              // Format-aware CLI utilities (kgrep/ksed/kcat/kconv).
              type: "docSidebar",
              sidebarId: "toolboxSidebar",
              label: "CLI tools",
            },
            {
              type: "docSidebar",
              sidebarId: "reactSidebar",
              label: "Kapi React",
            },
          ],
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
              label: "Kapi React",
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
