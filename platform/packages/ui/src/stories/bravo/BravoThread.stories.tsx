import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoThread } from "../../components/bravo/BravoThread";
import { sampleMessages } from "./fixtures";

const meta: Meta<typeof BravoThread> = {
  title: "Bravo/BravoThread",
  component: BravoThread,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ maxWidth: 480, padding: 0, border: "1px solid #ddd", borderRadius: 8, minHeight: 400 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof BravoThread>;

export const WithMessages: Story = {
  args: {
    messages: sampleMessages,
    onApprove: fn(),
    onDeny: fn(),
  },
};

export const Streaming: Story = {
  args: {
    messages: sampleMessages.slice(0, 1),
    streaming: true,
    streamingContent: "I'm analyzing the project structure to find all French locale files...",
    onApprove: fn(),
    onDeny: fn(),
  },
};

export const Empty: Story = {
  args: {
    messages: [],
  },
};
