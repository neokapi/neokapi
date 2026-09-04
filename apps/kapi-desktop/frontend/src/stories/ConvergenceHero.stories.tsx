import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergenceHero, ConvergePlanDialog } from "../components/ConvergenceHero";
import type { ConvergePlan, ConvergenceReport } from "../types/api";

const meta: Meta<typeof ConvergenceHero> = {
  title: "Project/ConvergenceHero",
  component: ConvergenceHero,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConvergenceHero>;

const PLAN_NOTE =
  "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls).";

const DRIFTED_PLAN: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: [
      {
        locale: "nb-NO",
        collection: "docs",
        missingTarget: 34,
        tmExact: 12,
        aiRemaining: 22,
        tokenEstimate: 1840,
      },
      {
        locale: "nb-NO",
        collection: "ui-strings",
        missingTarget: 8,
        tmExact: 5,
        aiRemaining: 3,
        tokenEstimate: 120,
      },
      {
        locale: "de-DE",
        collection: "docs",
        missingTarget: 34,
        tmExact: 2,
        drafts: 12,
        aiRemaining: 20,
        tokenEstimate: 1690,
      },
    ],
    totals: {
      missingTarget: 76,
      tmExact: 19,
      drafts: 12,
      aiRemaining: 45,
      tokenEstimate: 3650,
    },
    note: PLAN_NOTE,
  },
  changedFiles: 5,
  removedFiles: 1,
  storeMissing: false,
  versionStale: false,
};

const DRIFTED_REPORT: ConvergenceReport = {
  project: "Acme Docs",
  locales: [
    {
      locale: "nb-NO",
      collection: "docs",
      total: 120,
      pct: { translated: 72 },
      gated: true,
      shippable: false,
    },
    {
      locale: "de-DE",
      collection: "docs",
      total: 120,
      pct: { translated: 71 },
      gated: true,
      shippable: false,
    },
  ],
  review: [
    { locale: "nb-NO", file: "docs/intro.md", key: "h1", source: "Getting started" },
    { locale: "nb-NO", file: "docs/intro.md", key: "p2", source: "Install the CLI" },
    { locale: "de-DE", file: "docs/api.md", key: "h1", source: "API reference" },
  ],
};

const CONVERGED_PLAN: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: null,
    totals: { missingTarget: 0, tmExact: 0, aiRemaining: 0, tokenEstimate: 0 },
    note: PLAN_NOTE,
  },
  changedFiles: 0,
  removedFiles: 0,
  storeMissing: false,
  versionStale: false,
};

const CONVERGED_REPORT: ConvergenceReport = {
  project: "Acme Docs",
  locales: [
    {
      locale: "nb-NO",
      collection: "docs",
      total: 120,
      pct: { translated: 100, reviewed: 100 },
      gated: true,
      shippable: true,
    },
    {
      locale: "de-DE",
      collection: "docs",
      total: 120,
      pct: { translated: 100, reviewed: 100 },
      gated: true,
      shippable: true,
    },
  ],
  review: [],
};

/** Source drift + pending work: the hero leads with the summary and an armed
 *  "Bring up to date". */
export const Drifted: Story = {
  args: {
    tabID: "storybook",
    convergence: DRIFTED_REPORT,
    plan: DRIFTED_PLAN,
    onBringUpToDate: () => {},
  },
};

/** Everything converged and gates green: the calm state, button disabled. */
export const Converged: Story = {
  args: {
    tabID: "storybook",
    convergence: CONVERGED_REPORT,
    plan: CONVERGED_PLAN,
    onBringUpToDate: () => {},
  },
};

/** The pre-flight plan dialog on its own: per-(collection, locale) missing /
 *  content memory-exact / AI-remainder counts, the token estimate, and its heuristic note. */
export const PlanDialog: StoryObj<typeof ConvergePlanDialog> = {
  render: () => (
    <ConvergePlanDialog open onOpenChange={() => {}} plan={DRIFTED_PLAN} onConfirm={() => {}} />
  ),
};

const SUBSCRIPTION_PLAN: ConvergePlan = {
  ...DRIFTED_PLAN,
  plan: {
    ...DRIFTED_PLAN.plan,
    provider: "claude-code",
    subscription: true,
    note: "AI work runs on your Claude subscription — the token estimate is scale, not a metered cost. Content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 (no tokenizer, no provider calls).",
  },
};

/** The plan dialog when the resolved provider is claude-code: the metered-cost
 *  framing is replaced with "runs on your Claude subscription". */
export const PlanDialogSubscription: StoryObj<typeof ConvergePlanDialog> = {
  render: () => (
    <ConvergePlanDialog
      open
      onOpenChange={() => {}}
      plan={SUBSCRIPTION_PLAN}
      onConfirm={() => {}}
    />
  ),
};

/** The hero when AI work bills the user's Claude subscription. */
export const DriftedSubscription: Story = {
  args: {
    tabID: "storybook",
    convergence: DRIFTED_REPORT,
    plan: SUBSCRIPTION_PLAN,
    onBringUpToDate: () => {},
  },
};
