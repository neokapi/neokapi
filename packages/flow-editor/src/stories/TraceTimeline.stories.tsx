import type { Meta, StoryObj } from "@storybook/react-vite";
import { TraceTimeline } from "../TraceTimeline";
import type { TraceEvent } from "../traceTypes";

const meta: Meta<typeof TraceTimeline> = {
  title: "Flow Editor/TraceTimeline",
  component: TraceTimeline,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ width: 600 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TraceTimeline>;

const completedEvents: TraceEvent[] = [
  { ts: 0, type: "enter", node_id: "tool-0", part_id: "p1" },
  { ts: 500, type: "exit", node_id: "tool-0", part_id: "p1" },
  { ts: 600, type: "enter", node_id: "tool-0", part_id: "p2" },
  { ts: 900, type: "exit", node_id: "tool-0", part_id: "p2" },
  { ts: 550, type: "enter", node_id: "tool-1", part_id: "p1" },
  { ts: 1200, type: "exit", node_id: "tool-1", part_id: "p1" },
  { ts: 950, type: "enter", node_id: "tool-1", part_id: "p2" },
  { ts: 1800, type: "exit", node_id: "tool-1", part_id: "p2" },
  { ts: 1250, type: "enter", node_id: "tool-2", part_id: "p1" },
  { ts: 1400, type: "exit", node_id: "tool-2", part_id: "p1" },
  { ts: 1850, type: "enter", node_id: "tool-2", part_id: "p2" },
  { ts: 2000, type: "exit", node_id: "tool-2", part_id: "p2" },
];

const nodeNames = new Map([
  ["tool-0", "translate"],
  ["tool-1", "qa"],
  ["tool-2", "word-count"],
]);

export const Completed: Story = {
  args: {
    events: completedEvents,
    nodeNames,
    totalDurationUs: 2000,
  },
};

export const WithError: Story = {
  args: {
    events: [
      ...completedEvents.slice(0, 4),
      { ts: 550, type: "enter", node_id: "tool-1", part_id: "p1" },
      {
        ts: 800,
        type: "error",
        node_id: "tool-1",
        part_id: "p1",
        meta: { error: "check failed: missing translation" },
      },
    ],
    nodeNames,
    totalDurationUs: 800,
  },
};

export const SingleNode: Story = {
  args: {
    events: [
      { ts: 0, type: "enter", node_id: "tool-0", part_id: "p1" },
      { ts: 300, type: "exit", node_id: "tool-0", part_id: "p1" },
    ],
    nodeNames: new Map([["tool-0", "pseudo-translate"]]),
    totalDurationUs: 300,
  },
};
