import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergencePanel } from "../components/ConvergencePanel";
import type { ConvergenceReport } from "../types/api";

const meta: Meta<typeof ConvergencePanel> = {
  title: "Project/ConvergencePanel",
  component: ConvergencePanel,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConvergencePanel>;

const CONVERGING: ConvergenceReport = {
  project: "Acme Docs",
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
      locale: "de",
      total: 42,
      pct: { draft: 100, translated: 100, reviewed: 100, "signed-off": 100 },
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
    { locale: "ja", file: "src/locales/en.json", key: "nav.docs", source: "Documentation" },
  ],
};

export const Converging: Story = {
  args: { tabID: "storybook", report: CONVERGING },
};

export const FullyShippable: Story = {
  args: {
    tabID: "storybook",
    report: {
      project: "Acme Docs",
      locales: [
        {
          locale: "nb",
          total: 42,
          pct: { draft: 100, translated: 100, reviewed: 100, "signed-off": 100 },
          gated: true,
          shippable: true,
        },
      ],
      review: [],
    },
  },
};

export const NothingTrackedYet: Story = {
  args: {
    tabID: "storybook",
    report: { project: "New project", locales: [], review: [] },
  },
};
