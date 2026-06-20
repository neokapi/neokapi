import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectPage } from "../components/ProjectPage";

const meta: Meta<typeof ProjectPage> = {
  title: "Pages/ProjectPage",
  component: ProjectPage,
  tags: ["autodocs"],
  args: {
    tabID: "story-tab",
  },
};

export default meta;
type Story = StoryObj<typeof ProjectPage>;

export const WithContent: Story = {
  args: {
    project: {
      version: "v1",
      name: "Acme App Localization",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE", "ja-JP"],
      },
      content: [
        {
          path: "src/i18n/en/*.json",
          format: { name: "json" },
          target: "src/i18n/{lang}/*.json",
        },
        {
          name: "Documentation",
          items: [{ path: "docs/en/**/*.md", format: { name: "markdown" } }],
        },
      ],
      preset: "nextjs",
      plugins: {
        okapi: { framework_version: "^1.47.0" },
      },
      flows: {
        translate: {
          steps: [{ tool: "translate", config: { provider: "anthropic" } }],
        },
        "translate-and-qa": {
          steps: [{ tool: "translate", config: { provider: "anthropic" } }, { tool: "qa" }],
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
      defaults: {
        source_language: "en",
      },
    },
    projectPath: "",
  },
};

export const WithFlowsOnly: Story = {
  args: {
    project: {
      version: "v1",
      name: "QA Pipeline",
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"],
      },
      flows: {
        qa: {
          steps: [{ tool: "qa" }],
        },
        pseudo: {
          steps: [{ tool: "pseudo-translate", config: { expansion_rate: 1.3 } }],
        },
      },
    },
    projectPath: "/tmp/qa.kapi",
  },
};
