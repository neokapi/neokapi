import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectPage } from "../components/ProjectPage";

const meta: Meta<typeof ProjectPage> = {
  title: "Pages/ProjectPage",
  component: ProjectPage,
  tags: ["autodocs"],
  args: {},
};

export default meta;
type Story = StoryObj<typeof ProjectPage>;

export const WithContent: Story = {
  args: {
    project: {
      version: "v1",
      name: "Acme App Localization",
      source_language: "en-US",
      target_languages: ["fr-FR", "de-DE", "ja-JP"],
      content: [
        { path: "src/i18n/en/*.json", format: "json", target: "src/i18n/{lang}/*.json" },
        { path: "docs/en/**/*.md", format: "markdown" },
      ],
      preset: "nextjs",
      plugins: ["okapi@1.47.0"],
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
    projectPath: "/Users/dev/acme-app/translation.kapi",
  },
};

export const Minimal: Story = {
  args: {
    project: {
      version: "v1",
      name: "New Project",
      source_language: "en",
    },
    projectPath: "",
  },
};

export const WithFlowsOnly: Story = {
  args: {
    project: {
      version: "v1",
      name: "QA Pipeline",
      source_language: "en-US",
      target_languages: ["fr-FR"],
      flows: {
        "qa-check": {
          steps: [{ tool: "qa-check" }],
        },
        pseudo: {
          steps: [{ tool: "pseudo-translate", config: { expansion_rate: 1.3 } }],
        },
      },
    },
    projectPath: "/tmp/qa.kapi",
  },
};
