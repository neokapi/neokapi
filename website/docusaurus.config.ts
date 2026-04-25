import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

const config: Config = {
  title: "neokapi",
  tagline: "Open, AI-native localization platform in Go",
  favicon: "img/favicon.png",

  url: "https://neokapi.github.io",
  baseUrl: "/web/neokapi/",

  organizationName: "neokapi",
  projectName: "neokapi",
  trailingSlash: false,

  // Set to "warn" during the website split (issue #425, Phase 0). Existing
  // kapi MDX content still has /bowrain/... cross-links that need to be
  // converted to BowrainLink components (or removed per the framework/platform
  // separation rule). Restore "throw" once those are cleaned up.
  onBrokenLinks: "warn",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: "warn",
      onBrokenMarkdownImages: "warn",
    },
  },

  themes: ["@docusaurus/theme-mermaid"],

  plugins: [
    [
      "@docusaurus/plugin-content-docs",
      {
        id: "ad",
        path: "../docs/architecture-decisions",
        routeBasePath: "docs/ad",
        sidebarPath: "./sidebars-ad.ts",
        editUrl: "https://github.com/neokapi/neokapi/tree/main/",
      },
    ],
    [
      "@docusaurus/plugin-content-docs",
      {
        id: "notes",
        path: "../docs/notes",
        routeBasePath: "docs/notes",
        sidebarPath: "./sidebars-notes.ts",
        editUrl: "https://github.com/neokapi/neokapi/tree/main/",
      },
    ],
  ],

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/neokapi/neokapi/tree/main/website/",
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
      title: "neokapi",
      logo: {
        alt: "neokapi",
        src: "img/logo.png",
      },
      items: [
        {
          type: "docSidebar",
          sidebarId: "neokapiSidebar",
          label: "Documentation",
          position: "left",
        },
        {
          type: "docSidebar",
          docsPluginId: "ad",
          sidebarId: "ad",
          label: "Architecture",
          position: "left",
        },
        {
          type: "docSidebar",
          docsPluginId: "notes",
          sidebarId: "notes",
          label: "Notes",
          position: "left",
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
          title: "Neokapi",
          items: [
            {
              label: "Getting Started",
              to: "/docs/getting-started/introduction",
            },
            {
              label: "Kapi CLI",
              to: "/docs/kapi-cli/overview",
            },
            {
              label: "Framework",
              to: "/docs/developer/architecture",
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
      copyright: `Copyright \u00a9 ${new Date().getFullYear()} neokapi contributors. Built with Docusaurus.`,
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
