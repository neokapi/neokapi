import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StreamMergeDialog } from "../../components/StreamMergeDialog";
import { sampleMergeResult, emptyMergeResult } from "./fixtures";

const meta: Meta<typeof StreamMergeDialog> = {
  title: "Components/StreamMergeDialog",
  component: StreamMergeDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof StreamMergeDialog>;

export const WithChanges: Story = {
  args: {
    result: sampleMergeResult,
    streamName: "feature/translations",
    parentName: "main",
    onConfirm: fn(),
    onClose: fn(),
    open: true,
  },
};

export const NoChanges: Story = {
  args: {
    result: emptyMergeResult,
    streamName: "feature/translations",
    parentName: "main",
    onConfirm: fn(),
    onClose: fn(),
    open: true,
  },
};
