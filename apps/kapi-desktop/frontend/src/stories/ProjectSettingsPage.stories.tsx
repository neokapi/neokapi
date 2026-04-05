import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectSettingsPage } from "../components/ProjectSettingsPage";

const installedPlugins = [
  {
    name: "okapi",
    id: "okapi:1.47.0",
    version: "2.20.0",
    framework_version: "1.47.0",
    description: "Okapi Framework bridge — 57+ format filters for document processing",
    type: "format",
    formats: ["okf_html", "okf_xml", "okf_xliff", "okf_po", "okf_properties"],
  },
  {
    name: "custom-filter",
    id: "custom-filter:1.0.0",
    version: "1.2.0",
    description: "Custom XML filter for proprietary format",
    type: "format",
    formats: ["custom_xml"],
  },
];

const meta: Meta<typeof ProjectSettingsPage> = {
  title: "Pages/ProjectSettingsPage",
  component: ProjectSettingsPage,
  tags: ["autodocs"],
  args: {
    onUpdate: fn(),
    tabID: "story-tab",
    presetList: [
      { name: "nextjs", description: "Next.js i18n with JSON files" },
      { name: "react-intl", description: "React-Intl message files" },
      { name: "flutter", description: "Flutter ARB localization" },
    ],
    installedPlugins,
  },
};

export default meta;
type Story = StoryObj<typeof ProjectSettingsPage>;

export const WithPlugins: Story = {
  args: {
    project: {
      version: "v1",
      name: "Acme App",
      plugins: {
        okapi: { framework_version: "^1.47.0", format_priority: 200 },
      },
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE"],
        concurrency: 4,
        parallel_blocks: 16,
        encoding: "UTF-8",
        formats: {
          json: { preset: "i18next", priority: 10 },
          xliff: { preset: "default", config: { preserveWhitespace: true } },
        },
      },
    },
  },
};

export const MissingPlugin: Story = {
  args: {
    project: {
      version: "v1",
      name: "Missing Deps",
      plugins: {
        okapi: { framework_version: "^1.47.0" },
        "unknown-plugin": { version: "^1.0.0" },
      },
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"],
      },
    },
  },
};

export const Minimal: Story = {
  args: {
    project: {
      version: "v1",
      name: "New Project",
      defaults: {
        source_language: "en",
      },
    },
  },
};

export const NoPluginsInstalled: Story = {
  args: {
    installedPlugins: [],
    project: {
      version: "v1",
      name: "Fresh Install",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"],
      },
    },
  },
};
