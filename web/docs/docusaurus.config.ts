import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

const config: Config = {
  title: "neokapi",
  tagline: "Open, AI-native localization platform in Go",
  favicon: "img/favicon.png",

  url: "https://neokapi.github.io",
  // The neokapi-web Vite app sits at /web/neokapi/; this Docusaurus
  // instance lives one level deeper at /web/neokapi/docs/.
  baseUrl: "/web/neokapi/docs/",

  organizationName: "neokapi",
  projectName: "neokapi",
  trailingSlash: false,

  onBrokenLinks: "throw",

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

  // Architecture docs and implementation notes were absorbed into the
  // main docs tree (issue #425 followup). The `ad` and `notes` plugin
  // instances are no longer needed.
  plugins: [],

  presets: [
    [
      "classic",
      {
        docs: {
          // routeBasePath "/" puts docs at the root of the Docusaurus
          // instance, which itself is mounted at baseUrl. So URLs end up
          // as /web/neokapi/docs/{topic}.
          routeBasePath: "/",
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/neokapi/neokapi/tree/main/web/docs/",
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
              to: "/getting-started/introduction",
            },
            {
              label: "Kapi CLI",
              to: "/kapi-cli/overview",
            },
            {
              label: "Framework",
              to: "/developer/architecture",
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
