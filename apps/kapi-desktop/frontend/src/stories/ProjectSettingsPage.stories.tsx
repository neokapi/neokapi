import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectSettingsPage } from "../components/ProjectSettingsPage";

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

export const MultiplePlugins: Story = {
  args: {
    project: {
      version: "v1",
      name: "Full Stack L10n",
      plugins: {
        okapi: { version: "^0.38.0", framework_version: "^1.47.0", format_priority: 200 },
        "custom-filter": { version: "^1.0.0" },
      },
      defaults: {
        source_language: "en-US",
        target_languages: ["ja-JP", "ko-KR", "zh-CN"],
        concurrency: 2,
        formats: {
          okf_html: { preset: "lenient", priority: 100 },
          okf_xml: { config: { escapeGT: false } },
        },
      },
    },
  },
};
