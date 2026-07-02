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
    convergence: {
      project: "KapiMart",
      review: [],
      locales: [
        // de-DE: fully shippable across every collection.
        {
          collection: "Website",
          locale: "de-DE",
          total: 245,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Online Store",
          locale: "de-DE",
          total: 349,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Contracts",
          locale: "de-DE",
          total: 80,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Templates",
          locale: "de-DE",
          total: 25,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        // fr-FR: high coverage, partly reviewed → in review.
        {
          collection: "Website",
          locale: "fr-FR",
          total: 245,
          pct: { translated: 78, reviewed: 30 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Online Store",
          locale: "fr-FR",
          total: 349,
          pct: { translated: 100, reviewed: 60 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Contracts",
          locale: "fr-FR",
          total: 80,
          pct: {},
          gated: true,
          shippable: false,
        },
        {
          collection: "Templates",
          locale: "fr-FR",
          total: 25,
          pct: { translated: 48 },
          gated: true,
          shippable: false,
        },
        // ja-JP / nb-NO: translated only, no review yet.
        {
          collection: "Website",
          locale: "ja-JP",
          total: 245,
          pct: { translated: 45 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Online Store",
          locale: "ja-JP",
          total: 349,
          pct: { translated: 50 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Website",
          locale: "nb-NO",
          total: 245,
          pct: { translated: 41 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Online Store",
          locale: "nb-NO",
          total: 349,
          pct: { translated: 50 },
          gated: true,
          shippable: false,
        },
        // ar-SA: not started.
        {
          collection: "Website",
          locale: "ar-SA",
          total: 245,
          pct: {},
          gated: true,
          shippable: false,
        },
      ],
    },
  },
};

/** Three target languages — the per-language bar columns (Option A). */
export const ThreeLanguages: Story = {
  args: {
    ...Default.args,
    displayName: "Acme App Localization",
    project: {
      version: "v1",
      name: "Acme App Localization",
      defaults: { source_language: "en-US", target_languages: ["fr-FR", "de-DE", "ja-JP"] },
      content: [
        { name: "Website", items: [{ path: "docs/**/*.md", format: { name: "markdown" } }] },
        { name: "UI Strings", items: [{ path: "src/i18n/en/*.json" }] },
        { name: "Emails", items: [{ path: "emails/**/*.html" }] },
      ],
      flows: { translate: { steps: [{ tool: "translate" }] } },
    },
    status: {
      projectPath: "/Users/dev/projects/acme/acme.kapi",
      projectName: "Acme App Localization",
      hasData: true,
      collections: [
        {
          name: "Website",
          blockCount: 245,
          coverage: { "fr-FR": 245, "de-DE": 191, "ja-JP": 110 },
          targetLanguages: ["fr-FR", "de-DE", "ja-JP"],
        },
        {
          name: "UI Strings",
          blockCount: 88,
          coverage: { "fr-FR": 88, "de-DE": 40, "ja-JP": 0 },
          targetLanguages: ["fr-FR", "de-DE", "ja-JP"],
        },
        {
          name: "Emails",
          blockCount: 32,
          coverage: { "fr-FR": 16, "de-DE": 0, "ja-JP": 0 },
          targetLanguages: ["fr-FR", "de-DE", "ja-JP"],
        },
      ],
    },
  },
};

// Coverage cells reframed as ship-gate ladder states (Shippable / In review /
// Draft / —) from the convergence report, with the project strip summarizing
// shippable-ness per language. Three languages ⇒ the labelled (Option A) layout.
export const WithShipGates: Story = {
  args: {
    ...ThreeLanguages.args,
    convergence: {
      project: "Acme App Localization",
      review: [],
      locales: [
        {
          collection: "Website",
          locale: "fr-FR",
          total: 245,
          pct: { translated: 100, reviewed: 100, "signed-off": 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Website",
          locale: "de-DE",
          total: 245,
          pct: { translated: 78, reviewed: 40 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Website",
          locale: "ja-JP",
          total: 245,
          pct: { translated: 45 },
          gated: true,
          shippable: false,
        },
        {
          collection: "UI Strings",
          locale: "fr-FR",
          total: 88,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "UI Strings",
          locale: "de-DE",
          total: 88,
          pct: { translated: 45 },
          gated: true,
          shippable: false,
        },
        {
          collection: "UI Strings",
          locale: "ja-JP",
          total: 88,
          pct: {},
          gated: true,
          shippable: false,
        },
        {
          collection: "Emails",
          locale: "fr-FR",
          total: 32,
          pct: { translated: 50 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Emails",
          locale: "de-DE",
          total: 32,
          pct: {},
          gated: true,
          shippable: false,
        },
        {
          collection: "Emails",
          locale: "ja-JP",
          total: 32,
          pct: {},
          gated: true,
          shippable: false,
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
