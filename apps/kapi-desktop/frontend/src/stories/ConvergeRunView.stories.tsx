import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConvergeRunView } from "../components/ConvergeRunView";
import type { RunEvent } from "../context/JobFeedContext";

const meta: Meta<typeof ConvergeRunView> = {
  title: "Project/ConvergeRunView",
  component: ConvergeRunView,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConvergeRunView>;

const PASSES: RunEvent[] = [
  {
    type: "converge_pass",
    flow_id: "translate",
    converge: {
      pass: 1,
      extractedFiles: 6,
      extractedBlocks: 214,
      produced: 178,
      producedDelta: 178,
      failingChecks: 9,
      pendingLocales: ["de-DE", "ja-JP"],
    },
  },
  {
    type: "converge_pass",
    flow_id: "translate",
    converge: {
      pass: 2,
      produced: 205,
      producedDelta: 27,
      failingChecks: 2,
      pendingLocales: ["ja-JP"],
    },
  },
];

/** Mid-run: passes stream in while the engine loops toward the gate. */
export const Running: Story = {
  args: { events: PASSES, running: true },
};

/** Converged: every gated scope shipped; files materialized from the store. */
export const Converged: Story = {
  args: {
    events: [
      ...PASSES,
      {
        type: "complete",
        flow_id: "translate",
        converge_result: {
          flow: "translate",
          passes: 3,
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
      ...PASSES,
      {
        type: "complete",
        flow_id: "translate",
        converge_result: {
          flow: "translate",
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
