import type { Meta, StoryObj } from "@storybook/react-vite";
import { PartInspector } from "../PartInspector";
import type { PartSnapshotSet } from "../traceTypes";

const meta: Meta<typeof PartInspector> = {
  title: "Flow Editor/PartInspector",
  component: PartInspector,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ height: 500, display: "flex" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PartInspector>;

const singleBlockParts: Record<string, PartSnapshotSet> = {
  p1: {
    initial: { id: "p1", type: "Block", summary: "Hello world", sourceText: "Hello world" },
    afterNode: {
      "tool-0": {
        id: "p1",
        type: "Block",
        summary: "Hello world",
        sourceText: "Hello world",
        targetText: "Bonjour le monde",
      },
    },
  },
};

const multiBlockParts: Record<string, PartSnapshotSet> = {
  p1: {
    initial: { id: "p1", type: "Block", summary: "Hello world", sourceText: "Hello world" },
    afterNode: {
      "tool-0": {
        id: "p1",
        type: "Block",
        summary: "Hello world",
        sourceText: "Hello world",
        targetText: "Bonjour le monde",
      },
    },
  },
  p2: {
    initial: {
      id: "p2",
      type: "Block",
      summary: "Click here",
      sourceText: "Click here",
      targetText: "Click here",
    },
    afterNode: {
      "tool-0": {
        id: "p2",
        type: "Block",
        summary: "Click here",
        sourceText: "Click here",
        targetText: "Cliquez ici",
      },
    },
  },
  p3: {
    initial: { id: "p3", type: "Block", summary: "Submit form", sourceText: "Submit form" },
    afterNode: {
      "tool-0": {
        id: "p3",
        type: "Block",
        summary: "Submit form",
        sourceText: "Submit form",
        targetText: "Soumettre le formulaire",
      },
    },
  },
};

export const SingleBlock: Story = {
  args: {
    nodeId: "tool-0",
    nodeName: "translate",
    parts: singleBlockParts,
  },
};

export const MultipleBlocks: Story = {
  args: {
    nodeId: "tool-0",
    nodeName: "translate",
    parts: multiBlockParts,
  },
};

export const Empty: Story = {
  args: {
    nodeId: "tool-0",
    nodeName: "translate",
    parts: {},
  },
};
