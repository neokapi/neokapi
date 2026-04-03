import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StreamEditDialog } from "../../components/StreamEditDialog";
import { featureStream } from "./fixtures";

const meta: Meta<typeof StreamEditDialog> = {
  title: "Streams/StreamEditDialog",
  component: StreamEditDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof StreamEditDialog>;

export const Default: Story = {
  args: { stream: featureStream, onSubmit: fn(), onClose: fn(), open: true },
};
