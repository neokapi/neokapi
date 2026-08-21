import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ContextScanProgress } from "../../context-scan/ContextScanProgress";

const meta: Meta<typeof ContextScanProgress> = {
  title: "Context Scan/ContextScanProgress",
  component: ContextScanProgress,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 520, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextScanProgress>;

export const Queued: Story = {
  args: {
    job: { id: "scan-1", status: "queued", progress: 0, message: "", tokens_used: 0 },
  },
};

export const ReadingSources: Story = {
  args: {
    job: {
      id: "scan-1",
      status: "processing",
      progress: 10,
      message: "reading-sources",
      tokens_used: 0,
    },
  },
};

export const DraftingVoice: Story = {
  args: {
    job: {
      id: "scan-1",
      status: "processing",
      progress: 60,
      message: "drafting-voice",
      tokens_used: 8400,
    },
  },
};

export const Failed: Story = {
  args: {
    job: {
      id: "scan-1",
      status: "failed",
      progress: 40,
      message: "assembling-corpus",
      tokens_used: 0,
      error: "none of the sources produced any text",
    },
    onRetry: fn(),
  },
};
