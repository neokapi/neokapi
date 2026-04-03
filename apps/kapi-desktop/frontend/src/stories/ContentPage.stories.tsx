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
  },
};

export default meta;
type Story = StoryObj<typeof ContentPage>;

export const Default: Story = {
  args: {
    project: {
      version: "v1",
      name: "Acme App Localization",
      source_language: "en-US",
      target_languages: ["fr-FR", "de-DE"],
      preset: "nextjs",
      plugins: ["okapi@1.47.0"],
      content: [
        {
          path: "src/i18n/en/*.json",
          format: "json",
          target: "src/i18n/{lang}/*.json",
          collection: "UI Strings",
        },
        {
          path: "docs/en/**/*.md",
          format: "markdown",
          collection: "Documentation",
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
      source_language: "en-US",
      target_languages: ["fr-FR", "ja-JP"],
      plugins: ["okapi@1.47.0"],
      content: [
        {
          path: "docs/**/*.html",
          format: "okf_html",
          format_preset: "strict-extraction",
          target: "output/{lang}/docs/**/*.html",
          collection: "Documentation",
        },
        {
          path: "emails/*.html",
          format: "okf_html",
          format_config: { useCodeFinder: true, escapeGT: false },
          collection: "Emails",
        },
        {
          path: "src/i18n/en/*.json",
          collection: "UI Strings",
          target: "src/i18n/{lang}/*.json",
        },
      ],
    },
  },
};

export const WithPluginPinning: Story = {
  args: {
    project: {
      version: "v1",
      name: "Pinned Plugins",
      source_language: "en-US",
      target_languages: ["fr-FR"],
      plugins: ["okapi@1.47.0", "custom-filter@2.1.0"],
      content: [
        {
          path: "input/*",
          format: "okf_html",
          format_preset: "default",
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
      source_language: "en",
      target_languages: ["fr-FR"],
      content: [],
    },
  },
};
