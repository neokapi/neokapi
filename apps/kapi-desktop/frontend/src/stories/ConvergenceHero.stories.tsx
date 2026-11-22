import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergenceHero, ConvergePlanDialog, HeldBackDialog } from "../components/ConvergenceHero";
import type { ConvergePlan, ConvergenceReport } from "../types/api";

const meta: Meta<typeof ConvergenceHero> = {
  title: "Project/ConvergenceHero",
  component: ConvergenceHero,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConvergenceHero>;

const PLAN_NOTE =
  "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining blocks (no tokenizer, no provider calls).";

const DRIFTED_PLAN: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: [
      {
        locale: "nb-NO",
        collection: "docs",
        missing_target: 34,
        memory_exact: 12,
        ai_remaining: 22,
        token_estimate: 1840,
      },
      {
        locale: "nb-NO",
        collection: "ui-strings",
        missing_target: 8,
        memory_exact: 5,
        ai_remaining: 3,
        token_estimate: 120,
      },
      {
        locale: "de-DE",
        collection: "docs",
        missing_target: 34,
        memory_exact: 2,
        drafts: 12,
        ai_remaining: 20,
        token_estimate: 1690,
      },
    ],
    totals: {
      missing_target: 76,
      memory_exact: 19,
      drafts: 12,
      ai_remaining: 45,
      token_estimate: 3650,
    },
    note: PLAN_NOTE,
  },
  changedFiles: 0,
  removedFiles: 0,
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
      pending: [{ state: "translated", actual: 72, required: 100 }],
      stale_awaiting_review: 3,
    },
    {
      locale: "de-DE",
      collection: "docs",
      total: 120,
      pct: { translated: 71 },
      gated: true,
      shippable: false,
      pending: [{ state: "translated", actual: 71, required: 100 }],
      failing_checks: 2,
      terms_not_checked: 5,
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
    totals: { missing_target: 0, memory_exact: 0, ai_remaining: 0, token_estimate: 0 },
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
      pct: { translated: 100, established: 100 },
      gated: true,
      shippable: true,
    },
    {
      locale: "de-DE",
      collection: "docs",
      total: 120,
      pct: { translated: 100, established: 100 },
      gated: true,
      shippable: true,
    },
  ],
  review: [],
};

/** Texts to translate, translations waiting for review and languages held
 *  back: three counts, each leading to where it is dealt with, and an armed
 *  "Catch up". */
export const Drifted: Story = {
  args: {
    tabID: "storybook",
    convergence: DRIFTED_REPORT,
    plan: DRIFTED_PLAN,
    onBringUpToDate: () => {},
    onOpenReview: () => {},
    onOpenChecks: () => {},
  },
};

/** The cache is behind the files (a first open, or files changed outside the
 *  app). The app reads them on its own; the hero says so and asks nothing. */
export const ReadingFiles: Story = {
  args: {
    tabID: "storybook",
    convergence: DRIFTED_REPORT,
    plan: { ...DRIFTED_PLAN, storeMissing: true },
    onBringUpToDate: () => {},
  },
};

/** Nothing left to translate: what remains is a person's, so the run action
 *  is absent and the counts lead to Review. */
export const WaitingForAPerson: Story = {
  args: {
    tabID: "storybook",
    convergence: {
      ...DRIFTED_REPORT,
      locales: DRIFTED_REPORT.locales.map((l) => ({
        ...l,
        pending: undefined,
        terms_not_checked: 0,
      })),
    },
    plan: CONVERGED_PLAN,
    onBringUpToDate: () => {},
    onOpenReview: () => {},
  },
};

/** The held-back list: each collection language and what holds it back. */
export const HeldBack: StoryObj<typeof HeldBackDialog> = {
  render: () => (
    <HeldBackDialog
      open
      onOpenChange={() => {}}
      scopes={DRIFTED_REPORT.locales}
      onOpenReview={() => {}}
      onOpenChecks={() => {}}
    />
  ),
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
