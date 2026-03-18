import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoToolCall } from "../../components/bravo/BravoToolCall";
import { sampleToolCall, sampleApprovalToolCall } from "./fixtures";

const meta: Meta<typeof BravoToolCall> = {
  title: "Bravo/BravoToolCall",
  component: BravoToolCall,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ maxWidth: 400, padding: 16 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof BravoToolCall>;

export const Completed: Story = {
  args: {
    toolCall: sampleToolCall,
  },
};

export const NeedsApproval: Story = {
  args: {
    toolCall: sampleApprovalToolCall,
    onApprove: fn(),
    onDeny: fn(),
  },
};

export const Running: Story = {
  args: {
    toolCall: { ...sampleToolCall, status: "running", output: undefined, duration: 0 },
  },
};

export const Failed: Story = {
  args: {
    toolCall: {
      ...sampleToolCall,
      status: "failed",
      error: "connection timeout: could not reach connector endpoint",
      output: undefined,
    },
  },
};
