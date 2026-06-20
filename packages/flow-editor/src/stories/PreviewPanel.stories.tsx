import type { Meta, StoryObj } from "@storybook/react-vite";
import { PreviewPanel, type PreviewResult } from "../PreviewPanel";

const nodeNames = new Map([
  ["tool-0", "translate"],
  ["tool-1", "qa"],
]);

const mockResult: PreviewResult = {
  parts: {
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
        "tool-1": {
          id: "p1",
          type: "Block",
          summary: "Hello world",
          sourceText: "Hello world",
          targetText: "Bonjour le monde",
        },
      },
    },
  },
  nodeOrder: ["tool-0", "tool-1"],
};

const meta: Meta<typeof PreviewPanel> = {
  title: "Flow Editor/PreviewPanel",
  component: PreviewPanel,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ width: 700 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PreviewPanel>;

export const WithSampleText: Story = {
  args: {
    onPreview: async () => mockResult,
    sourceLang: "en-US",
    targetLang: "fr-FR",
    nodeNames,
  },
};

export const Empty: Story = {
  args: {
    onPreview: async () => ({ parts: {}, nodeOrder: [] }),
    sourceLang: "en-US",
    targetLang: "fr-FR",
    nodeNames: new Map(),
  },
};
