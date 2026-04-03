import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConfirmDialog } from "../../components/ConfirmDialog";

const meta: Meta<typeof ConfirmDialog> = {
  title: "Foundations/ConfirmDialog",
  component: ConfirmDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof ConfirmDialog>;

export const Default: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Confirm Action",
    description: "Are you sure you want to proceed? This action cannot be undone.",
    onConfirm: fn(),
  },
};

export const Destructive: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Delete Project",
    description: "This will permanently delete the project and all its translations.",
    confirmLabel: "Delete",
    variant: "destructive",
    onConfirm: fn(),
  },
};

export const Loading: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Delete Project",
    description: "This will permanently delete the project.",
    confirmLabel: "Delete",
    variant: "destructive",
    loading: true,
    onConfirm: fn(),
  },
};

export const CustomLabels: Story = {
  args: {
    open: true,
    onOpenChange: fn(),
    title: "Archive Stream",
    description: "Archive this stream? It can be restored later from the bin.",
    confirmLabel: "Yes, archive it",
    cancelLabel: "Keep it",
    onConfirm: fn(),
  },
};
