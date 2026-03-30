import type { Meta, StoryObj } from "@storybook/react-vite";
import { TermStatusBadge } from "../../components/resource-browser/TermStatusBadge";

const meta: Meta<typeof TermStatusBadge> = {
  title: "Resource Browser/TermStatusBadge",
  component: TermStatusBadge,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Semantic status badge for term lifecycle status. Uses OKLCH color space with CSS custom properties for dark mode. Deprecated and forbidden statuses display with a strikethrough.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof TermStatusBadge>;

export const Preferred: Story = {
  args: { status: "preferred" },
};

export const Approved: Story = {
  args: { status: "approved" },
};

export const Admitted: Story = {
  args: { status: "admitted" },
};

export const Proposed: Story = {
  args: { status: "proposed" },
};

export const Deprecated: Story = {
  args: { status: "deprecated" },
};

export const Forbidden: Story = {
  args: { status: "forbidden" },
};

/** All six term statuses shown together for comparison. */
export const AllStatuses: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <TermStatusBadge status="preferred" />
      <TermStatusBadge status="approved" />
      <TermStatusBadge status="admitted" />
      <TermStatusBadge status="proposed" />
      <TermStatusBadge status="deprecated" />
      <TermStatusBadge status="forbidden" />
    </div>
  ),
};
