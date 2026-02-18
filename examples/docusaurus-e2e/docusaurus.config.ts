import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

const config: Config = {
  title: "Acme Docs",
  tagline: "Build amazing things with Acme",
  favicon: "img/favicon.ico",
  url: "https://docs.acme.example",
  baseUrl: "/",

  onBrokenLinks: "throw",
  onBrokenMarkdownLinks: "warn",

  i18n: {
    defaultLocale: "en",
    locales: ["en", "nb", "fr"],
    localeConfigs: {
      en: { label: "English" },
      nb: { label: "Norsk Bokm\u00e5l" },
      fr: { label: "Fran\u00e7ais" },
    },
  },

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
        },
        blog: {
          showReadingTime: true,
        },
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    navbar: {
      title: "Acme Docs",
      items: [
        {
          type: "docSidebar",
          sidebarId: "tutorialSidebar",
          position: "left",
          label: "Documentation",
        },
        { to: "/blog", label: "Blog", position: "left" },
        {
          type: "localeDropdown",
          position: "right",
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          title: "Docs",
          items: [
            { label: "Getting Started", to: "/docs/intro" },
            { label: "Features", to: "/docs/features" },
          ],
        },
        {
          title: "Community",
          items: [
            { label: "GitHub", href: "https://github.com/acme/acme" },
            { label: "Discord", href: "https://discord.gg/acme" },
          ],
        },
        {
          title: "More",
          items: [
            { label: "Blog", to: "/blog" },
          ],
        },
      ],
      copyright: `Copyright \u00a9 ${new Date().getFullYear()} Acme, Inc.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
