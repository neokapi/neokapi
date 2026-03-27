import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StreamCreateDialog } from "../../components/StreamCreateDialog";
import { sampleStreams } from "./fixtures";

const meta: Meta<typeof StreamCreateDialog> = {
  title: "Streams/StreamCreateDialog",
  component: StreamCreateDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof StreamCreateDialog>;

export const Default: Story = {
  args: { streams: sampleStreams, onSubmit: fn(), onClose: fn(), open: true },
};
