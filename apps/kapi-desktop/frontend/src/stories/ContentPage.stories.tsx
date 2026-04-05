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
    formatList: [
      {
        name: "json",
        display_name: "JSON",
        extensions: [".json"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "xliff",
        display_name: "XLIFF 1.2",
        extensions: [".xlf", ".xliff"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "xliff2",
        display_name: "XLIFF 2.0",
        extensions: [".xlf"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "po",
        display_name: "Gettext PO",
        extensions: [".po", ".pot"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "properties",
        display_name: "Java Properties",
        extensions: [".properties"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "markdown",
        display_name: "Markdown",
        extensions: [".md"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "html",
        display_name: "HTML",
        extensions: [".html", ".htm"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "xml",
        display_name: "XML",
        extensions: [".xml"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "csv",
        display_name: "CSV",
        extensions: [".csv"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "yaml",
        display_name: "YAML",
        extensions: [".yaml", ".yml"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
      {
        name: "okf_html",
        display_name: "HTML (Okapi)",
        extensions: [".html"],
        has_reader: true,
        has_writer: true,
        source: "okapi",
        has_schema: true,
      },
      {
        name: "okf_xml",
        display_name: "XML (Okapi)",
        extensions: [".xml"],
        has_reader: true,
        has_writer: true,
        source: "okapi",
        has_schema: true,
      },
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
