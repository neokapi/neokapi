import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@neokapi/ui-primitives";
import { Plus } from "../../components/icons";
import { BrandHub } from "./BrandHub";
import { TermStatusBadge, ChangeSetStatusBadge, RelationBadge, EmptyState } from "./atoms";

const meta: Meta<typeof BrandHub> = {
  title: "Brand Hub/Shell/BrandHub",
  component: BrandHub,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BrandHub>;

export const Default: Story = {
  args: {
    title: "Concepts",
    description:
      "The language-neutral units of your brand — each with its terms, status by locale, and place in the graph.",
    actions: (
      <Button size="sm">
        <Plus />
        New concept
      </Button>
    ),
    children: (
      <div className="rounded-lg border p-6 text-sm text-muted-foreground">Section content.</div>
    ),
  },
};

export const WithAtoms: Story = {
  args: {
    title: "Atoms",
    description: "The shared status and relation vocabulary used across the hub.",
    children: (
      <div className="flex flex-wrap gap-2">
        <TermStatusBadge status="preferred" />
        <TermStatusBadge status="forbidden" />
        <TermStatusBadge status="proposed" />
        <ChangeSetStatusBadge status="in_review" />
        <ChangeSetStatusBadge status="merged" />
        <RelationBadge type="REPLACED_BY" />
        <RelationBadge type="COMPETITOR" />
      </div>
    ),
  },
};

export const Empty: Story = {
  args: {
    title: "Experiments",
    children: (
      <EmptyState
        title="No experiments yet"
        description="Start a change-set to propose a governed change."
      />
    ),
  },
};
