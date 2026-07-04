import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergeRunView } from "../components/ConvergeRunView";
import type { RunEvent } from "../context/JobFeedContext";
import type { ConvergeEvent } from "../types/api";

const meta: Meta<typeof ConvergeRunView> = {
  title: "Project/ConvergeRunView",
  component: ConvergeRunView,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConvergeRunView>;

/** Wrap a typed convergence event as the RunEvent the job feed stores. */
function ce(event: ConvergeEvent): RunEvent {
  return { type: "converge_event", flow_id: "up", converge_event: event };
}

// A live pass mid-flight: one locale done, one running, one still queued.
const RUNNING_PASS: RunEvent[] = [
  { type: "state", flow_id: "up", message: "running" },
  ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["de-DE", "ja-JP", "nb-NO"] }),
  ce({ type: "locale_start", pass: 1, locale: "de-DE", units: 42 }),
  ce({ type: "locale_done", pass: 1, locale: "de-DE", units: 42, done: 42, viaTM: 30, viaAI: 12 }),
  ce({ type: "locale_start", pass: 1, locale: "ja-JP", units: 42 }),
  ce({ type: "unit_progress", pass: 1, locale: "ja-JP", done: 18, viaTM: 6, viaAI: 12 }),
];

// A settled first pass, plus a converging second pass.
const TWO_PASSES: RunEvent[] = [
  { type: "state", flow_id: "up", message: "running" },
  ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["de-DE", "ja-JP"] }),
  ce({ type: "locale_done", pass: 1, locale: "de-DE", units: 42, done: 42, viaTM: 30, viaAI: 12 }),
  ce({ type: "locale_done", pass: 1, locale: "ja-JP", units: 42, done: 40, viaTM: 8, viaAI: 32 }),
  ce({
    type: "pass_done",
    pass: 1,
    produced: 178,
    producedDelta: 178,
    failingChecks: 9,
    pending: ["ja-JP"],
  }),
  ce({ type: "pass_start", pass: 2, maxPasses: 6, pending: ["ja-JP"] }),
  ce({ type: "locale_start", pass: 2, locale: "ja-JP", units: 42 }),
  ce({ type: "unit_progress", pass: 2, locale: "ja-JP", done: 27, viaTM: 4, viaAI: 23 }),
];

/** Mid-run: a live pass with per-locale progress bars streaming in. */
export const Running: Story = {
  args: { events: RUNNING_PASS, running: true },
};

/** Mid-run, second pass: the first pass settled, the loop converges the rest. */
export const RunningSecondPass: Story = {
  args: { events: TWO_PASSES, running: true },
};

/** Converged: every gated scope shipped; files materialized from the store. */
export const Converged: Story = {
  args: {
    events: [
      ...TWO_PASSES,
      ce({
        type: "locale_done",
        pass: 2,
        locale: "ja-JP",
        units: 42,
        done: 42,
        viaTM: 8,
        viaAI: 34,
      }),
      ce({
        type: "pass_done",
        pass: 2,
        produced: 205,
        producedDelta: 27,
        failingChecks: 0,
        pending: [],
      }),
      ce({ type: "materialized", files: 18 }),
      ce({ type: "done", state: "converged" }),
      {
        type: "complete",
        flow_id: "up",
        converge_result: {
          flow: "up",
          passes: 2,
          converged: true,
          locales: [
            { locale: "nb-NO", shippable: true },
            { locale: "de-DE", shippable: true },
            { locale: "ja-JP", shippable: true },
          ],
          materializedFiles: 18,
        },
      },
    ],
  },
};

/** Parked: the loop stalled short of the gate — each parked (collection,
 *  locale) scope deep-links into the Review page. */
export const Parked: Story = {
  args: {
    events: [
      ...TWO_PASSES,
      ce({
        type: "locale_done",
        pass: 2,
        locale: "ja-JP",
        units: 42,
        done: 40,
        viaTM: 8,
        viaAI: 32,
      }),
      ce({
        type: "pass_done",
        pass: 2,
        produced: 200,
        producedDelta: 22,
        failingChecks: 2,
        pending: ["ja-JP"],
      }),
      ce({ type: "done", state: "parked" }),
      {
        type: "complete",
        flow_id: "up",
        converge_result: {
          flow: "up",
          passes: 5,
          converged: false,
          locales: [
            { locale: "nb-NO", shippable: true },
            { locale: "ja-JP", shippable: false, parked: true, failingChecks: 2 },
          ],
          parkedScopes: [
            { locale: "ja-JP", collection: "docs" },
            { locale: "ja-JP", collection: "ui-strings" },
          ],
        },
      },
    ],
    onOpenReview: () => {},
  },
};

/** Compatibility: an older run that stored only per-pass summaries (no typed
 *  locale stream) still renders its passes. */
export const CompatPassSummaries: Story = {
  args: {
    events: [
      {
        type: "converge_pass",
        flow_id: "up",
        converge: {
          pass: 1,
          extractedBlocks: 214,
          produced: 178,
          producedDelta: 178,
          failingChecks: 9,
          pendingLocales: ["de-DE", "ja-JP"],
        },
      },
      {
        type: "converge_pass",
        flow_id: "up",
        converge: {
          pass: 2,
          produced: 205,
          producedDelta: 27,
          failingChecks: 2,
          pendingLocales: ["ja-JP"],
        },
      },
    ],
    running: true,
  },
};
