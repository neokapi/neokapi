import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";
import { execFileSync } from "node:child_process";

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
// /web/neokapi/ root and its home page (src/pages/index.tsx) is the product
// landing page. (A separate Vite landing app previously occupied this root; it
// was retired and its content folded into the docs home — see
// src/components/home/.) PR previews are instead served from
// /web/prs/<N>/neokapi/docs/ (the deploy step runs from the default branch and
// slots PR docs there), so the deploy workflow overrides the base path via
// DOCS_BASE_URL — without it, internal links would carry the production prefix
// and navigate out of the preview.
const baseUrl = process.env.DOCS_BASE_URL ?? "/web/neokapi/";

const config: Config = {
  title: "neokapi",
  tagline: "The faithful, format-aware content engine for people and AI agents",
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
    // Silence the benign "Critical dependency" webpack warning emitted by the
    // UMD wrapper in vscode-languageserver-types (pulled in transitively via
    // @docusaurus/theme-mermaid → mermaid → langium). The `require` it flags is
    // the module's CommonJS/AMD format detection, not a missing dependency.
    // Scoped to that module's path so an equivalent warning from our own code
    // is NOT suppressed.
    function ignoreWebpackWarnings() {
      return {
        name: "ignore-webpack-warnings",
        configureWebpack() {
          return {
            ignoreWarnings: [
              (warning: { message?: string; module?: { resource?: string } }) =>
                /Critical dependency: require function is used/.test(warning.message ?? "") &&
                /[\\/]node_modules[\\/]vscode-languageserver-types[\\/]/.test(
                  warning.module?.resource ?? "",
                ),
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
                // dynamic import.
                { externals: ["icu", "gliner"] }
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
  ],

  presets: [
    [
      "classic",
      {
        docs: {
          // routeBasePath "/" puts docs at the root of the Docusaurus
          // instance, which itself is mounted at baseUrl. So URLs end up
          // as /web/neokapi/{topic}, and the home page (src/pages/index.tsx)
          // sits at /web/neokapi/.
          routeBasePath: "/",
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/neokapi/neokapi/tree/main/web/docs/",
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
          "neokapi is an open-source, format-aware content engine in Go. It parses localization, document, and data formats into a faithful content model, then translates, leverages translation memory, and runs verification checks for terminology, QA, and brand voice that act like tests for AI output.",
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
          to: "/lab",
          label: "Lab",
          position: "left",
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
              // Format-aware CLI utilities (kgrep/ksed/kcat).
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
          type: "localeDropdown",
          position: "right",
        },
        {
          href: "https://github.com/neokapi/neokapi",
          label: "GitHub",
          position: "right",
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
