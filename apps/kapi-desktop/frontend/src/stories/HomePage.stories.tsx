import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { HomePage } from "../components/HomePage";

const meta: Meta<typeof HomePage> = {
  title: "Pages/HomePage",
  component: HomePage,
  tags: ["autodocs"],
  args: {
    onRunFlow: fn(),
    onNavigate: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof HomePage>;

export const Default: Story = {
  args: {
    displayName: "Acme App Localization",
    project: {
      version: "v1",
      name: "Acme App Localization",
      source_language: "en-US",
      target_languages: ["fr-FR", "de-DE", "ja-JP"],
      content: [
        { path: "src/i18n/en/*.json", format: "json", target: "src/i18n/{lang}/*.json" },
        { path: "docs/en/**/*.md", format: "markdown" },
      ],
      flows: {
        translate: {
          steps: [{ tool: "ai-translate", config: { provider: "anthropic" } }],
        },
        "translate-and-qa": {
          steps: [
            { tool: "ai-translate", config: { provider: "anthropic" } },
            { tool: "qa-check" },
          ],
        },
      },
    },
  },
};

export const NoFlows: Story = {
  args: {
    displayName: "Starter Project",
    project: {
      version: "v1",
      name: "Starter Project",
      source_language: "en-US",
      target_languages: ["fr-FR"],
      content: [{ path: "src/locales/en.json", format: "json" }],
    },
  },
};
