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

// URL of the kapi/neokapi docs site, used for cross-site links.
// Defaults to the GitHub Pages production URL; override via env var locally
// to point at a localhost build of the kapi site.
const KAPI_WEB_SITE = process.env.KAPI_WEB_SITE || "https://neokapi.github.io/web/neokapi/";

// URL of the Bowrain marketing landing page (the bowrain-web Vite app that sits
// one level up from these docs, at /web/bowrain/). The top-left navbar logo
// links here so it navigates back out to the product landing page; override via
// env var locally to point at a localhost build of the bowrain site.
const BOWRAIN_WEB_SITE = process.env.BOWRAIN_WEB_SITE || "https://neokapi.github.io/web/bowrain/";

const config: Config = {
  title: "Bowrain",
  tagline: "Govern and steward brand voice, terminology, and translation — as a team",
  favicon: "img/favicon.png",

  url: "https://neokapi.github.io",
  // The bowrain-web Vite app sits at /web/bowrain/; this Docusaurus instance
  // lives one level deeper at /web/bowrain/docs/. PR previews are served from
  // /web/prs/<N>/bowrain/docs/ instead, so the deploy workflow overrides the
  // base path via DOCS_BASE_URL — without it, a preview build would bake the
  // production prefix and 404 every asset (mirrors the kapi docs site).
  baseUrl: process.env.DOCS_BASE_URL ?? "/web/bowrain/docs/",

  organizationName: "neokapi",
  projectName: "neokapi",
  trailingSlash: false,

  onBrokenLinks: "throw",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  customFields: {
    kapiWebSite: KAPI_WEB_SITE,
    // Offload walkthrough videos to the shared CDN (Cloudflare R2) when
    // configured — empty DOCS_CDN_URL keeps them same-origin (the default). This
    // site has no wasm/models; only ThemedVideo reads these. See the kapi config
    // and web/docs/contribute/notes-internal/cdn-assets.md.
    cdnBaseUrl: process.env.DOCS_CDN_URL ?? "",
    cdnSitePrefix: "bowrain",
    cdnWasmVersion: process.env.DOCS_CDN_VERSION ?? "dev",
  },

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: "warn",
      onBrokenMarkdownImages: "warn",
    },
  },

  themes: ["@docusaurus/theme-mermaid"],

  presets: [
    [
      "classic",
      {
        docs: {
          path: "docs",
          routeBasePath: "/",
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/neokapi/neokapi/tree/main/bowrain/web/docs/",
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    // @neokapi/docs-shared ships raw TS (main: src/index.ts). In this standalone
    // (non-workspace) docs build it resolves under node_modules/.pnpm, which
    // Docusaurus's babel-loader excludes by default — so its `export type {…}`
    // failed webpack parsing. Transpile that package explicitly via Docusaurus's
    // own JS loader. (The kapi docs build resolves docs-shared from packages/,
    // outside node_modules, so it doesn't need this.)
    function transpileDocsShared() {
      return {
        name: "transpile-docs-shared",
        configureWebpack(
          _config: unknown,
          isServer: boolean,
          { getJSLoader }: { getJSLoader: (opts: { isServer: boolean }) => unknown },
        ) {
          return {
            module: {
              rules: [
                {
                  test: /\.tsx?$/,
                  include: [
                    /[\\/]@neokapi[\\/]docs-shared[\\/]/,
                    /[\\/]packages[\\/]docs-shared[\\/]/,
                  ],
                  use: [getJSLoader({ isServer })],
                },
              ],
            },
          };
        },
      };
    },
  ],

  themeConfig: {
    navbar: {
      title: "Bowrain",
      logo: {
        alt: "Bowrain",
        src: "img/logo.png",
        // The logo navigates back out to the Bowrain marketing landing page
        // (one level up from the docs), not to the docs root — the "Home" item
        // below covers the docs landing page. target _self keeps it in-tab.
        href: BOWRAIN_WEB_SITE,
        target: "_self",
      },
      items: [
        {
          // Docs landing page (src/pages/index.tsx). Added because the logo now
          // leaves the docs site for the marketing landing page.
          to: "/",
          label: "Home",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "gettingStartedSidebar",
          label: "Get Started",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "usingBowrainSidebar",
          label: "Using Bowrain",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "cliSidebar",
          label: "Connect (CLI)",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "forDevelopersSidebar",
          label: "For Developers",
          position: "left",
        },
        {
          href: KAPI_WEB_SITE,
          label: "Neokapi & Kapi",
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
          title: "Bowrain",
          items: [
            { label: "Getting Started", to: "/" },
            { label: "Using Bowrain", to: "/server/web-overview" },
            { label: "Connectors", to: "/server/connectors" },
            { label: "Connect (CLI)", to: "/cli/overview" },
          ],
        },
        {
          title: "Framework",
          items: [{ label: "Neokapi & Kapi", href: KAPI_WEB_SITE }],
        },
        {
          title: "More",
          items: [
            { label: "GitHub", href: "https://github.com/neokapi/neokapi" },
            { label: "Homebrew Tap", href: "https://github.com/neokapi/homebrew-tap" },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} neokapi contributors. Built with Docusaurus. · built ${buildStamp}`,
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
