import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContentPage } from "../components/ContentPage";

const meta: Meta<typeof ContentPage> = {
  title: "Pages/ContentPage",
  component: ContentPage,
  tags: ["autodocs"],
  args: {
    onUpdate: fn(),
    tabID: "tab-1",
    projectPath: "/Users/dev/acme-app/project.kapi",
    presetList: [
      { name: "nextjs", description: "Next.js i18n with JSON files" },
      { name: "react-intl", description: "React-Intl message files" },
      { name: "flutter", description: "Flutter ARB localization" },
    ],
    formatNames: [
      "json",
      "xliff",
      "xliff2",
      "po",
      "properties",
      "markdown",
      "html",
      "xml",
      "csv",
      "yaml",
      "resx",
      "strings",
      "arb",
      "ts",
      "android",
    ],
    basePath: "/Users/dev/acme-app",
  },
};

export default meta;
type Story = StoryObj<typeof ContentPage>;

export const Default: Story = {
  args: {
    project: {
      version: "v1",
      name: "Acme App Localization",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE"],
      },
      preset: "nextjs",
      plugins: {
        okapi: { framework_version: "^1.47.0", format_priority: 200 },
      },
      content: [
        {
          path: "src/i18n/en/*.json",
          format: { name: "json" },
          target: "src/i18n/{lang}/*.json",
        },
        {
          path: "docs/en/**/*.md",
          format: { name: "markdown" },
        },
      ],
    },
  },
};

export const WithCollections: Story = {
  args: {
    project: {
      version: "v1",
      name: "Multi-Collection Project",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE", "ja-JP"],
        formats: {
          okf_html: { preset: "strict-extraction" },
        },
      },
      plugins: {
        okapi: { framework_version: "^1.47.0", format_priority: 200 },
      },
      content: [
        {
          path: "src/i18n/en/*.json",
          target: "src/i18n/{lang}/*.json",
        },
        {
          name: "Marketing",
          target_languages: ["fr-FR", "de-DE"],
          items: [
            {
              path: "marketing/**/*.html",
              format: { name: "okf_html", preset: "lenient" },
              target: "marketing/{lang}/**/*.html",
            },
            {
              path: "marketing/**/*.json",
              target: "marketing/{lang}/**/*.json",
            },
          ],
        },
        {
          name: "China",
          source_language: "zh-CN",
          target_languages: ["en-US"],
          items: [
            {
              path: "china/**/*",
              target: "china/output/{lang}/**/*",
            },
          ],
        },
      ],
    },
  },
};

export const WithFormatPresets: Story = {
  args: {
    project: {
      version: "v1",
      name: "Okapi Bridge Project",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "ja-JP"],
      },
      plugins: {
        okapi: { framework_version: "^1.47.0" },
      },
      content: [
        {
          name: "Documentation",
          items: [
            {
              path: "docs/**/*.html",
              format: { name: "okf_html", preset: "strict-extraction" },
              target: "output/{lang}/docs/**/*.html",
            },
          ],
        },
        {
          name: "Emails",
          items: [
            {
              path: "emails/*.html",
              format: { name: "okf_html", config: { useCodeFinder: true, escapeGT: false } },
            },
          ],
        },
        {
          path: "src/i18n/en/*.json",
          target: "src/i18n/{lang}/*.json",
        },
      ],
    },
  },
};

export const WithPlugins: Story = {
  args: {
    project: {
      version: "v1",
      name: "Pinned Plugins",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"],
      },
      plugins: {
        okapi: { version: "^0.38.0", framework_version: "^1.47.0", format_priority: 200 },
        "custom-filter": { version: "^2.1.0" },
      },
      content: [
        {
          path: "input/*",
          format: { name: "okf_html" },
          target: "output/{lang}/*",
        },
      ],
    },
  },
};

export const Empty: Story = {
  args: {
    project: {
      version: "v1",
      name: "New Project",
      defaults: {
        source_language: "en",
        target_languages: ["fr-FR"],
      },
      content: [],
    },
  },
};
