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
        source_language: "en",
        target_languages: ["fr", "de", "ja"],
      },
      plugins: {
        okapi: { framework_version: "^1.47.0", format_priority: 200 },
      },
      preset: "nextjs",
      collections: [
        {
          name: "Website",
          content: [{ path: "docs/en/**/*.md", format: { name: "markdown" } }],
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
        source_language: "en",
        target_languages: ["fr"],
      },
      collections: [{ path: "src/locales/en.json", format: { name: "json" } }],
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
        source_language: "en",
        target_languages: ["de", "fr", "ja", "nb", "ar"],
      },
      collections: [
        {
          name: "Website",
          base: "web/en",
          content: [{ path: "web/en/**/*.md", target: "web/{lang}", format: { name: "markdown" } }],
        },
        {
          name: "Online Store",
          base: "src/en",
          content: [{ path: "src/en/*.{json,yaml,properties,html}", target: "src/{lang}" }],
        },
        {
          name: "Contracts",
          base: "legal/en",
          content: [{ path: "legal/en/*.{docx,xlsx}", target: "legal/{lang}" }],
        },
        {
          name: "Templates",
          base: "marketing/en",
          content: [{ path: "marketing/en/*.{pptx,docx}", target: "marketing/{lang}" }],
        },
      ],
      flows: {
        "pseudo-translate": { steps: [{ tool: "pseudo-translate" }] },
        translate: { steps: [{ tool: "translate" }] },
      },
    },
    status: {
      projectPath: "/Users/dev/projects/kapimart/kapi.yaml",
      projectName: "KapiMart",
      hasData: true,
      collections: [
        {
          name: "Website",
          blockCount: 245,
          coverage: { de: 245, fr: 191, ja: 110, nb: 100, ar: 0 },
          targetLanguages: ["de", "fr", "ja", "nb", "ar"],
        },
        {
          name: "Online Store",
          blockCount: 349,
          coverage: { de: 349, fr: 349, ja: 175, nb: 175, ar: 0 },
          targetLanguages: ["de", "fr", "ja", "nb", "ar"],
        },
        {
          name: "Contracts",
          blockCount: 80,
          coverage: { de: 80, fr: 0, ja: 0, nb: 0, ar: 0 },
          targetLanguages: ["de", "fr", "ja", "nb", "ar"],
        },
        {
          name: "Templates",
          blockCount: 25,
          coverage: { de: 25, fr: 12, ja: 0, nb: 0, ar: 0 },
          targetLanguages: ["de", "fr", "ja", "nb", "ar"],
        },
      ],
    },
    convergence: {
      project: "KapiMart",
      review: [],
      locales: [
        // de: fully shippable across every collection.
        {
          collection: "Website",
          locale: "de",
          total: 245,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Online Store",
          locale: "de",
          total: 349,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Contracts",
          locale: "de",
          total: 80,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Templates",
          locale: "de",
          total: 25,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        // fr: high coverage, partly reviewed → in review.
        {
          collection: "Website",
          locale: "fr",
          total: 245,
          pct: { translated: 78, reviewed: 30 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Online Store",
          locale: "fr",
          total: 349,
          pct: { translated: 100, reviewed: 60 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Contracts",
          locale: "fr",
          total: 80,
          pct: {},
          gated: true,
          shippable: false,
        },
        {
          collection: "Templates",
          locale: "fr",
          total: 25,
          pct: { translated: 48 },
          gated: true,
          shippable: false,
        },
        // ja / nb: translated only, no review yet.
        {
          collection: "Website",
          locale: "ja",
          total: 245,
          pct: { translated: 45 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Online Store",
          locale: "ja",
          total: 349,
          pct: { translated: 50 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Website",
          locale: "nb",
          total: 245,
          pct: { translated: 41 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Online Store",
          locale: "nb",
          total: 349,
          pct: { translated: 50 },
          gated: true,
          shippable: false,
        },
        // ar: not started.
        {
          collection: "Website",
          locale: "ar",
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
      defaults: { source_language: "en", target_languages: ["fr", "de", "ja"] },
      collections: [
        { name: "Website", content: [{ path: "docs/**/*.md", format: { name: "markdown" } }] },
        { name: "UI Strings", content: [{ path: "src/i18n/en/*.json" }] },
        { name: "Emails", content: [{ path: "emails/**/*.html" }] },
      ],
      flows: { translate: { steps: [{ tool: "translate" }] } },
    },
    status: {
      projectPath: "/Users/dev/projects/acme/kapi.yaml",
      projectName: "Acme App Localization",
      hasData: true,
      collections: [
        {
          name: "Website",
          blockCount: 245,
          coverage: { fr: 245, de: 191, ja: 110 },
          targetLanguages: ["fr", "de", "ja"],
        },
        {
          name: "UI Strings",
          blockCount: 88,
          coverage: { fr: 88, de: 40, ja: 0 },
          targetLanguages: ["fr", "de", "ja"],
        },
        {
          name: "Emails",
          blockCount: 32,
          coverage: { fr: 16, de: 0, ja: 0 },
          targetLanguages: ["fr", "de", "ja"],
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
          locale: "fr",
          total: 245,
          pct: { translated: 100, reviewed: 100, "signed-off": 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "Website",
          locale: "de",
          total: 245,
          pct: { translated: 78, reviewed: 40 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Website",
          locale: "ja",
          total: 245,
          pct: { translated: 45 },
          gated: true,
          shippable: false,
        },
        {
          collection: "UI Strings",
          locale: "fr",
          total: 88,
          pct: { translated: 100, reviewed: 100 },
          gated: true,
          shippable: true,
        },
        {
          collection: "UI Strings",
          locale: "de",
          total: 88,
          pct: { translated: 45 },
          gated: true,
          shippable: false,
        },
        {
          collection: "UI Strings",
          locale: "ja",
          total: 88,
          pct: {},
          gated: true,
          shippable: false,
        },
        {
          collection: "Emails",
          locale: "fr",
          total: 32,
          pct: { translated: 50 },
          gated: true,
          shippable: false,
        },
        {
          collection: "Emails",
          locale: "de",
          total: 32,
          pct: {},
          gated: true,
          shippable: false,
        },
        {
          collection: "Emails",
          locale: "ja",
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
      projectPath: "/Users/dev/projects/acme/kapi.yaml",
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
