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
  ce({ type: "pass_start", pass: 1, max_passes: 6, pending: ["de-DE", "ja-JP", "nb-NO"] }),
  ce({ type: "locale_start", pass: 1, locale: "de-DE", units: 42 }),
  ce({
    type: "locale_done",
    pass: 1,
    locale: "de-DE",
    units: 42,
    done: 42,
    via_memory: 30,
    via_ai: 12,
  }),
  ce({ type: "locale_start", pass: 1, locale: "ja-JP", units: 42 }),
  ce({ type: "unit_progress", pass: 1, locale: "ja-JP", done: 18, via_memory: 6, via_ai: 12 }),
];

// A settled first pass, plus a converging second pass.
const TWO_PASSES: RunEvent[] = [
  { type: "state", flow_id: "up", message: "running" },
  ce({ type: "pass_start", pass: 1, max_passes: 6, pending: ["de-DE", "ja-JP"] }),
  ce({
    type: "locale_done",
    pass: 1,
    locale: "de-DE",
    units: 42,
    done: 42,
    via_memory: 30,
    via_ai: 12,
  }),
  ce({
    type: "locale_done",
    pass: 1,
    locale: "ja-JP",
    units: 42,
    done: 40,
    via_memory: 8,
    via_ai: 32,
  }),
  ce({
    type: "pass_done",
    pass: 1,
    produced: 178,
    produced_delta: 178,
    failing_checks: 9,
    pending: ["ja-JP"],
  }),
  ce({ type: "pass_start", pass: 2, max_passes: 6, pending: ["ja-JP"] }),
  ce({ type: "locale_start", pass: 2, locale: "ja-JP", units: 42 }),
  ce({ type: "unit_progress", pass: 2, locale: "ja-JP", done: 27, via_memory: 4, via_ai: 23 }),
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
        via_memory: 8,
        via_ai: 34,
      }),
      ce({
        type: "pass_done",
        pass: 2,
        produced: 205,
        produced_delta: 27,
        failing_checks: 0,
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
          materialized_files: 18,
        },
      },
    ],
  },
};

/** Waiting for review: the loop stalled short of the gate, and each
 *  (collection, locale) scope waiting for review deep-links into the Review page. */
export const WaitingForReview: Story = {
  args: {
    events: [
      ...TWO_PASSES,
      ce({
        type: "locale_done",
        pass: 2,
        locale: "ja-JP",
        units: 42,
        done: 40,
        via_memory: 8,
        via_ai: 32,
      }),
      ce({
        type: "pass_done",
        pass: 2,
        produced: 200,
        produced_delta: 22,
        failing_checks: 2,
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
            { locale: "ja-JP", shippable: false, parked: true, failing_checks: 2 },
          ],
          parked_scopes: [
            { locale: "ja-JP", collection: "docs" },
            { locale: "ja-JP", collection: "ui-strings" },
          ],
        },
      },
    ],
    onOpenReview: () => {},
  },
};
