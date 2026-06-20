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
    onResetSample: fn(),
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
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR", "de-DE", "ja-JP"],
      },
      plugins: {
        okapi: { framework_version: "^1.47.0", format_priority: 200 },
      },
      preset: "nextjs",
      content: [
        {
          path: "src/i18n/en/*.json",
          format: { name: "json" },
          target: "src/i18n/{lang}/*.json",
        },
        { path: "docs/en/**/*.md", format: { name: "markdown" } },
      ],
      flows: {
        translate: {
          steps: [{ tool: "translate", config: { provider: "anthropic" } }],
        },
        "translate-and-qa": {
          steps: [{ tool: "translate", config: { provider: "anthropic" } }, { tool: "qa" }],
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
      defaults: {
        source_language: "en-US",
        target_languages: ["fr-FR"],
      },
      content: [{ path: "src/locales/en.json", format: { name: "json" } }],
    },
  },
};

/** Per-collection, per-locale coverage from a completed extraction. */
export const WithCoverage: Story = {
  args: {
    ...Default.args,
    status: {
      projectPath: "/Users/dev/projects/acme/acme.kapi",
      projectName: "Acme App Localization",
      hasData: true,
      collections: [
        {
          name: "ui-strings",
          blockCount: 240,
          coverage: { "fr-FR": 240, "de-DE": 180, "ja-JP": 96 },
          targetLanguages: ["fr-FR", "de-DE", "ja-JP"],
        },
        {
          name: "docs",
          blockCount: 88,
          coverage: { "fr-FR": 40, "de-DE": 0, "ja-JP": 0 },
          targetLanguages: ["fr-FR", "de-DE", "ja-JP"],
        },
      ],
    },
  },
};

/** Project configured but never extracted — prompt to run extract. */
export const NeverExtracted: Story = {
  args: {
    ...Default.args,
    status: {
      projectPath: "/Users/dev/projects/acme/acme.kapi",
      projectName: "Acme App Localization",
      hasData: false,
      collections: [],
    },
  },
};

/** A sample opened by a newer kapi than the one that scaffolded it. */
export const SampleUpgradeAvailable: Story = {
  args: {
    ...Default.args,
    displayName: "KapiMart",
    sampleInfo: {
      is_sample: true,
      name: "kapimart",
      display_name: "KapiMart",
      on_disk_revision: 1,
      current_revision: 2,
      upgrade_available: true,
    },
  },
};
