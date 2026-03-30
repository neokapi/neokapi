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
    projectPath: "/Users/dev/acme-app/translation.kapi",
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
      content: [
        { path: "src/i18n/en/*.json", format: "json", target: "src/i18n/{lang}/*.json" },
        { path: "docs/en/**/*.md", format: "markdown" },
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

export const MultiLanguage: Story = {
  args: {
    project: {
      version: "v1",
      name: "Global Platform",
      source_language: "en-US",
      target_languages: [
        "fr-FR",
        "de-DE",
        "ja-JP",
        "zh-CN",
        "ko-KR",
        "es-ES",
        "pt-BR",
        "it-IT",
        "nl-NL",
        "ru-RU",
      ],
      content: [
        { path: "locales/en/**/*.json", format: "json", target: "locales/{lang}/**/*.json" },
      ],
    },
  },
};
