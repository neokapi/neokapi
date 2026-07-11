import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergenceRunView } from "@neokapi/status-views";
import { reduceRun } from "@neokapi/status-views";
import type { ConvergenceEvent, ConvergenceRun } from "../../types/api";

const run: ConvergenceRun = {
  id: "run-8f2a1c0e",
  project_id: "proj-demo",
  trigger: "manual",
  state: "running",
  passes: 1,
  created_at: new Date().toISOString(),
};

const meta: Meta<typeof ConvergenceRunView> = {
  title: "Pages/Convergence/ConvergenceRunView",
  component: ConvergenceRunView,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 720, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConvergenceRunView>;

// A live run mid-pass: fr-FR done, de-DE still translating.
const liveEvents: ConvergenceEvent[] = [
  { type: "pass_start", pass: 1, maxPasses: 3, pending: ["fr-FR", "de-DE"] },
  { type: "locale_start", pass: 1, locale: "fr-FR", units: 12 },
  {
    type: "locale_done",
    pass: 1,
    locale: "fr-FR",
    units: 12,
    done: 12,
    viaTM: 8,
    viaAI: 4,
    state: "shippable",
  },
  { type: "locale_start", pass: 1, locale: "de-DE", units: 9 },
  { type: "unit_progress", pass: 1, locale: "de-DE", done: 5, viaTM: 2, viaAI: 3 },
];

const convergedEvents: ConvergenceEvent[] = [
  ...liveEvents,
  {
    type: "locale_done",
    pass: 1,
    locale: "de-DE",
    units: 9,
    done: 9,
    viaTM: 3,
    viaAI: 6,
    state: "shippable",
  },
  { type: "pass_done", pass: 1, produced: 21, producedDelta: 21, failingChecks: 0, pending: [] },
  { type: "materialized", files: 2 },
  { type: "done", state: "converged" },
];

const parkedEvents: ConvergenceEvent[] = [
  { type: "pass_start", pass: 1, maxPasses: 2, pending: ["ja-JP"] },
  { type: "locale_start", pass: 1, locale: "ja-JP", units: 10 },
  { type: "locale_done", pass: 1, locale: "ja-JP", units: 10, done: 6, viaAI: 6, state: "parked" },
  {
    type: "pass_done",
    pass: 1,
    produced: 6,
    producedDelta: 6,
    failingChecks: 4,
    pending: ["ja-JP"],
  },
  { type: "log", message: "ja-JP short of ship gate after max passes" },
  { type: "done", state: "parked" },
];

export const Running: Story = {
  args: { model: reduceRun(liveEvents), run, connecting: false },
};

export const Connecting: Story = {
  args: { model: reduceRun([]), run, connecting: true },
};

export const Converged: Story = {
  args: {
    model: reduceRun(convergedEvents),
    run: { ...run, state: "converged", finished_at: new Date().toISOString() },
  },
};

export const Parked: Story = {
  args: {
    model: reduceRun(parkedEvents),
    run: { ...run, trigger: "cli", state: "parked", finished_at: new Date().toISOString() },
  },
};
