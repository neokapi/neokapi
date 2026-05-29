import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

// URL of the kapi/neokapi docs site, used for cross-site links.
// Defaults to the GitHub Pages production URL; override via env var locally
// to point at a localhost build of the kapi site.
const KAPI_WEB_SITE = process.env.KAPI_WEB_SITE || "https://neokapi.github.io/web/neokapi/";

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

  themeConfig: {
    navbar: {
      title: "Bowrain",
      logo: {
        alt: "Bowrain",
        src: "img/logo.png",
      },
      items: [
        {
          type: "docSidebar",
          sidebarId: "gettingStartedSidebar",
          label: "Get Started",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "cliSidebar",
          label: "Project Sync",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "webSidebar",
          label: "Web App",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "desktopSidebar",
          label: "Desktop",
          position: "left",
        },
        {
          type: "docSidebar",
          sidebarId: "selfHostingSidebar",
          label: "Self-Hosting",
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
            { label: "Project sync (CLI)", to: "/cli/overview" },
            { label: "Web app", to: "/server/web-overview" },
          ],
        },
        {
          title: "Framework",
          items: [
            { label: "Neokapi & Kapi", href: KAPI_WEB_SITE },
          ],
        },
        {
          title: "More",
          items: [
            { label: "GitHub", href: "https://github.com/neokapi/neokapi" },
            { label: "Homebrew Tap", href: "https://github.com/neokapi/homebrew-tap" },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} neokapi contributors. Built with Docusaurus.`,
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
