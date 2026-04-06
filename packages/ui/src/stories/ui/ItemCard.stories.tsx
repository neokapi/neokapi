import type { Meta, StoryObj } from "@storybook/react-vite";
import { ItemCard } from "../../components/ui/item-card";
import { ConfirmDeleteButton } from "../../components/ui/confirm-delete-button";
import { Badge } from "../../components/ui/badge";
import { FileText, Workflow } from "lucide-react";

const meta: Meta<typeof ItemCard> = {
  title: "Foundations/ItemCard",
  component: ItemCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Universal card for list items. Consistent padding, hover, selection, and group behavior. Built on shadcn Card.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ItemCard>;

export const Default: Story = {
  render: () => (
    <div className="max-w-md space-y-2">
      <ItemCard>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileText size={14} className="text-primary" />
            <span className="text-sm font-medium">src/i18n/en/*.json</span>
          </div>
          <ConfirmDeleteButton onDelete={() => {}} mode="icon" />
        </div>
        <p className="mt-1 text-xs text-muted-foreground">
          Target: src/i18n/&#123;lang&#125;/*.json
        </p>
      </ItemCard>
      <ItemCard>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Workflow size={14} className="text-primary" />
            <span className="text-sm font-medium">translate-and-qa</span>
          </div>
          <Badge variant="secondary" className="text-[10px]">
            2 steps
          </Badge>
        </div>
      </ItemCard>
    </div>
  ),
};

export const Clickable: Story = {
  render: () => (
    <div className="max-w-md">
      <ItemCard clickable onClick={() => alert("Clicked!")}>
        <span className="text-sm font-medium">Click me</span>
        <p className="text-xs text-muted-foreground">
          This card has a pointer cursor and hover border.
        </p>
      </ItemCard>
    </div>
  ),
};

export const Selected: Story = {
  render: () => (
    <div className="max-w-md">
      <ItemCard selected>
        <span className="text-sm font-medium">Selected card</span>
        <p className="text-xs text-muted-foreground">Primary border and subtle background.</p>
      </ItemCard>
    </div>
  ),
};
