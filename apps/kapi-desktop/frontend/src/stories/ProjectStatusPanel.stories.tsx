import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectStatusPanel } from "../components/ProjectStatusPanel";
import type { ProjectStatus } from "../components/TranslationStatusPanel";
import type { ConvergenceReport } from "../types/api";

const meta: Meta<typeof ProjectStatusPanel> = {
  title: "Project/ProjectStatusPanel",
  component: ProjectStatusPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 720 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ProjectStatusPanel>;

const WORKING: ProjectStatus = {
  projectPath: "/Users/dev/app/translation.kapi",
  projectName: "My App Localization",
  collections: [
    {
      name: "ui",
      blockCount: 1007,
      coverage: { fr: 987, ja: 1007 },
      targetLanguages: ["fr", "de", "ja"],
    },
    { name: "marketing", blockCount: 42, coverage: {}, targetLanguages: ["fr", "de"] },
  ],
};

const SHIP: ConvergenceReport = {
  project: "My App Localization",
  source: {
    total: 42,
    pct: { authored: 100, checked: 90, approved: 0 },
    gated: true,
    shippable: false,
    pending: [{ state: "checked", actual: 90, required: 100 }],
  },
  locales: [
    {
      locale: "nb",
      total: 42,
      pct: { draft: 100, translated: 100, reviewed: 80, "signed-off": 0 },
      gated: true,
      shippable: true,
    },
    {
      locale: "ja",
      total: 42,
      pct: { draft: 100, translated: 55, reviewed: 0, "signed-off": 0 },
      gated: true,
      shippable: false,
      pending: [{ state: "translated", actual: 55, required: 100 }],
    },
  ],
  review: [
    {
      locale: "nb",
      file: "src/locales/en.json",
      key: "hero.title",
      source: "Ship localized content without the toil",
    },
    { locale: "ja", file: "src/locales/en.json", key: "cta.primary", source: "Get started" },
  ],
};

/** Opens on the Ship stage (the released-state view); toggle to Working. */
export const ShipDefault: Story = {
  args: { tabID: "storybook", defaultView: "ship", status: WORKING, report: SHIP },
};

/** Opens on the Working stage (block-store coverage, pre-merge). */
export const WorkingDefault: Story = {
  args: { tabID: "storybook", defaultView: "working", status: WORKING, report: SHIP },
};
