import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

// The neokapi-web Vite app sits at /web/neokapi/; this Docusaurus instance
// lives one level deeper at /web/neokapi/docs/. PR previews are served from
// /web/prs/<N>/neokapi/docs/ instead, so the deploy workflow overrides the
// base path via DOCS_BASE_URL — without it, internal links would carry the
// production prefix and navigate out of the preview.
const baseUrl = process.env.DOCS_BASE_URL ?? "/web/neokapi/docs/";

const config: Config = {
  title: "neokapi",
  tagline: "Format-aware localization and brand guardrails for people, elves, and agents",
  favicon: "img/favicon.png",

  url: "https://neokapi.github.io",
  baseUrl,

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
          // as /web/neokapi/docs/{topic}.
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
          "neokapi is an open-source, format-aware localization framework in Go. It provides a concurrent streaming pipeline, content model, and composable tools for AI translation, TM leverage, terminology enforcement, and brand-voice governance.",
      },
    ],
    navbar: {
      title: "neokapi",
      logo: {
        alt: "neokapi",
        src: "img/logo.png",
      },
      items: [
        // IA: Get Started is the onboarding funnel; Kapi is the product manual
        // (CLI + Desktop + recipes + projects); Framework is the engine (concepts
        // + extending + architecture + notes, merged); Reference holds the
        // generated/interactive references + MCP; Toolbox holds the CLI utilities
        // and Kapi React.
        {
          type: "docSidebar",
          sidebarId: "getStartedSidebar",
          label: "Get Started",
          position: "left",
        },
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
            // Generated, runnable references + interactive grids. R4 fills the
            // per-entry pages under /reference/{commands,formats,tools}/.
            { label: "Reference Overview", to: "/reference" },
            { label: "Kapi CLI Commands", to: "/commands" },
            { label: "Formats", to: "/formats" },
            { label: "Tools", to: "/tools" },
            { label: "MCP Server", to: "/reference/mcp" },
            { label: "Parity", to: "/parity" },
            { label: "Benchmarks", to: "/pseudobench" },
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
              label: "Get Started",
              to: "/get-started/introduction",
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
              label: "Recipes",
              to: "/kapi/recipes",
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
