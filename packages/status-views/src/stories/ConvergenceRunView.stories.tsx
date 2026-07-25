import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergenceRunView } from "../ConvergenceRunView";
import type { ConvergenceRunModel } from "../convergence-model";

const meta: Meta<typeof ConvergenceRunView> = {
  title: "Status/ConvergenceRunView",
  component: ConvergenceRunView,
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

type Story = StoryObj<typeof ConvergenceRunView>;

// A live pass mid-flight: one locale done, one running, one still queued.
const runningModel: ConvergenceRunModel = {
  live: true,
  passes: [
    {
      pass: 1,
      maxPasses: 6,
      settled: false,
      rows: [
        { locale: "de-DE", units: 42, done: 42, viaMemory: 30, viaAI: 12, state: "done" },
        { locale: "ja-JP", units: 42, done: 18, viaMemory: 6, viaAI: 12, state: "running" },
        { locale: "nb-NO", units: 0, done: 0, viaMemory: 0, viaAI: 0, state: "queued" },
      ],
    },
  ],
};

// A settled first pass with a post-derivation summary.
const settledModel: ConvergenceRunModel = {
  live: true,
  passes: [
    {
      pass: 1,
      maxPasses: 6,
      settled: true,
      produced: 178,
      producedDelta: 178,
      failingChecks: 2,
      pending: ["ja-JP"],
      rows: [
        { locale: "de-DE", units: 42, done: 42, viaMemory: 30, viaAI: 12, state: "done" },
        { locale: "ja-JP", units: 42, done: 40, viaMemory: 8, viaAI: 32, state: "done" },
      ],
    },
  ],
};

/** kapi-desktop's Runner surface: live passes, no header. */
export const Running: Story = {
  args: { model: runningModel, running: true },
};

/** A parked outcome with deep-link chips (kapi-desktop's structured result). */
export const Parked: Story = {
  args: {
    model: settledModel,
    result: {
      converged: false,
      passes: 2,
      parkedScopes: [
        { locale: "ja-JP", collection: "docs" },
        { locale: "ja-JP", collection: "marketing" },
      ],
    },
    onOpenReview: (scope) => console.log("open review", scope),
  },
};

/** A converged outcome with materialized-file count. */
export const Converged: Story = {
  args: {
    model: settledModel,
    result: { converged: true, passes: 2, materializedFiles: 6 },
  },
};

/** bowrain's Runs surface: a run header, ship badges, logs, and done footer. */
export const WithHeaderAndLogs: Story = {
  args: {
    run: { id: "run-8f2a1c0e", trigger: "manual", created_at: new Date().toISOString() },
    model: {
      ...settledModel,
      done: true,
      finalState: "parked",
      materializedFiles: 3,
      logs: ["Extracted 128 blocks from 4 files", "Transport: streamed 2 passes"],
      passes: settledModel.passes.map((p) => ({
        ...p,
        rows: p.rows.map((r) => ({
          ...r,
          localeState: r.locale === "ja-JP" ? "parked" : "shippable",
        })),
      })),
    },
  },
};
