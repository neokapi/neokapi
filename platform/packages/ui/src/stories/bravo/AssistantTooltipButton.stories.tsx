import type { Meta, StoryObj } from "@storybook/react-vite";
import { TooltipIconButton } from "../../components/assistant-ui/tooltip-icon-button";
import {
  CopyIcon,
  RefreshCwIcon,
  PencilIcon,
  ArrowUpIcon,
  PlusIcon,
  CheckIcon,
  SquareIcon,
} from "lucide-react";

const meta: Meta<typeof TooltipIconButton> = {
  title: "Bravo/Assistant UI/Tooltip Button",
  component: TooltipIconButton,
  tags: ["autodocs"],
  parameters: {
    layout: "centered",
  },
  decorators: [
    (Story) => (
      <div className="flex items-center gap-4 p-8 bg-background text-foreground">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TooltipIconButton>;

export const Copy: Story = {
  args: {
    tooltip: "Copy",
    children: <CopyIcon />,
  },
};

export const Refresh: Story = {
  args: {
    tooltip: "Refresh",
    children: <RefreshCwIcon />,
  },
};

export const Edit: Story = {
  args: {
    tooltip: "Edit",
    children: <PencilIcon />,
  },
};

export const Send: Story = {
  args: {
    tooltip: "Send message",
    variant: "default",
    className: "aui-button-icon size-8 rounded-full",
    children: <ArrowUpIcon className="size-4" />,
  },
};

export const AddAttachment: Story = {
  args: {
    tooltip: "Add Attachment",
    variant: "ghost",
    className: "aui-button-icon size-8 rounded-full p-1",
    children: <PlusIcon className="size-5 stroke-[1.5px]" />,
  },
};

export const AllVariants: Story = {
  render: () => (
    <div className="flex items-center gap-3 p-4">
      <TooltipIconButton tooltip="Copy">
        <CopyIcon />
      </TooltipIconButton>
      <TooltipIconButton tooltip="Refresh">
        <RefreshCwIcon />
      </TooltipIconButton>
      <TooltipIconButton tooltip="Edit">
        <PencilIcon />
      </TooltipIconButton>
      <TooltipIconButton tooltip="Copied!" variant="ghost">
        <CheckIcon />
      </TooltipIconButton>
      <TooltipIconButton tooltip="Send" variant="default" className="size-8 rounded-full">
        <ArrowUpIcon className="size-4" />
      </TooltipIconButton>
      <TooltipIconButton tooltip="Stop" variant="default" className="size-8 rounded-full">
        <SquareIcon className="size-3 fill-current" />
      </TooltipIconButton>
    </div>
  ),
};
