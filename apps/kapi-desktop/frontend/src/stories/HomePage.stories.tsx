import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { HomePage } from "../components/HomePage";

const meta: Meta<typeof HomePage> = {
  title: "Pages/HomePage",
  component: HomePage,
  tags: ["autodocs"],
  args: {
    tabID: "tab-1",
    onUpdate: fn(),
    onRunFlow: fn(),
    onNavigate: fn(),
    onResetSample: fn(),
    // Preload so the merged CollectionsPanel renders without a Wails backend.
    basePath: "/Users/dev/projects/acme",
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
        name: "markdown",
        display_name: "Markdown",
        extensions: [".md"],
        has_reader: true,
        has_writer: true,
        has_schema: false,
      },
    ],
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
          name: "Website",
          items: [{ path: "docs/en/**/*.md", format: { name: "markdown" } }],
        },
        {
          path: "src/i18n/en/*.json",
          format: { name: "json" },
          target: "src/i18n/{lang}/*.json",
        },
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

/** The collection-centric spine with extracted block counts + coverage — the
 *  merged surface from issue #1068 (Option A). */
export const WithCoverage: Story = {
  args: {
    ...Default.args,
    displayName: "KapiMart",
    project: {
      version: "v1",
      name: "KapiMart",
      defaults: {
        source_language: "en-US",
        target_languages: ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"],
      },
      content: [
        { name: "Website", items: [{ path: "input/docs/**/*.md", format: { name: "markdown" } }] },
        { name: "Online Store", items: [{ path: "input/store/**/*.json" }] },
        { name: "Contracts", items: [{ path: "input/contracts/**/*.docx" }] },
        { name: "Templates", items: [{ path: "input/templates/**/*.html" }] },
      ],
      flows: {
        "pseudo-translate": { steps: [{ tool: "pseudo-translate" }] },
        translate: { steps: [{ tool: "translate" }] },
      },
    },
    status: {
      projectPath: "/Users/dev/projects/kapimart/kapimart.kapi",
      projectName: "KapiMart",
      hasData: true,
      collections: [
        {
          name: "Website",
          blockCount: 245,
          coverage: { "de-DE": 245, "fr-FR": 191, "ja-JP": 110, "nb-NO": 100, "ar-SA": 0 },
          targetLanguages: ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"],
        },
        {
          name: "Online Store",
          blockCount: 349,
          coverage: { "de-DE": 349, "fr-FR": 349, "ja-JP": 175, "nb-NO": 175, "ar-SA": 0 },
          targetLanguages: ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"],
        },
        {
          name: "Contracts",
          blockCount: 80,
          coverage: { "de-DE": 80, "fr-FR": 0, "ja-JP": 0, "nb-NO": 0, "ar-SA": 0 },
          targetLanguages: ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"],
        },
        {
          name: "Templates",
          blockCount: 25,
          coverage: { "de-DE": 25, "fr-FR": 12, "ja-JP": 0, "nb-NO": 0, "ar-SA": 0 },
          targetLanguages: ["de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"],
        },
      ],
    },
  },
};

/** Project configured but never extracted — the strip prompts a run. */
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

/** Counts produced by an older kapi — the stale banner offers a re-extract. */
export const StaleCounts: Story = {
  args: {
    ...WithCoverage.args,
    status: {
      ...WithCoverage.args!.status!,
      stale: true,
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
