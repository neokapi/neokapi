import type { Meta, StoryObj } from "@storybook/react-vite";
import { Trash2 } from "lucide-react";
import { SimpleTooltip, TooltipProvider } from "../../components/ui/tooltip";
import { Button } from "../../components/ui/button";

const meta: Meta<typeof SimpleTooltip> = {
  title: "Foundations/SimpleTooltip",
  component: SimpleTooltip,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "One-liner tooltip wrapper over the Tooltip primitive — the standard replacement for native `title=` attributes. Uses the app-wide TooltipProvider when present and falls back to a local one otherwise. Falsy `content` renders the trigger unchanged.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimpleTooltip>;

export const IconButton: Story = {
  name: "Icon button (title= replacement)",
  render: () => (
    <SimpleTooltip content="Delete preset">
      <Button variant="ghost" size="icon-sm" aria-label="Delete preset">
        <Trash2 size={14} />
      </Button>
    </SimpleTooltip>
  ),
};

export const DisabledTrigger: Story = {
  name: "Disabled trigger (span-wrap pattern)",
  render: () => (
    <SimpleTooltip content="Select a project to author flows">
      <span className="inline-flex">
        <Button disabled>New flow</Button>
      </span>
    </SimpleTooltip>
  ),
};

export const ConditionalContent: Story = {
  name: "Falsy content renders trigger alone",
  render: () => (
    <SimpleTooltip content={undefined}>
      <Button variant="outline">No tooltip here</Button>
    </SimpleTooltip>
  ),
};

export const Sides: Story = {
  name: "Placement",
  render: () => (
    <TooltipProvider>
      <div className="flex items-center gap-4 p-12">
        {(["top", "right", "bottom", "left"] as const).map((side) => (
          <SimpleTooltip key={side} content={`side="${side}"`} side={side}>
            <Button variant="outline" size="sm">
              {side}
            </Button>
          </SimpleTooltip>
        ))}
      </div>
    </TooltipProvider>
  ),
};

export const TruncatedText: Story = {
  name: "Truncated text",
  render: () => (
    <SimpleTooltip content="src/locales/en-US/very-long-namespace/checkout.json">
      <span className="block max-w-40 truncate text-sm">
        src/locales/en-US/very-long-namespace/checkout.json
      </span>
    </SimpleTooltip>
  ),
};
