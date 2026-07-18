import type { Meta, StoryObj } from "@storybook/react-vite";
import { ShipStateBadge } from "../../components/ShipStateBadge";

const meta: Meta<typeof ShipStateBadge> = {
  title: "Components/ShipStateBadge",
  component: ShipStateBadge,
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
type Story = StoryObj<typeof ShipStateBadge>;

export const Governed: Story = {
  args: { state: "governed", approvedBlocks: 50, totalBlocks: 50, failingChecks: 0 },
};

export const AIShippable: Story = {
  args: { state: "ai_shippable", approvedBlocks: 12, totalBlocks: 50, failingChecks: 0 },
};

export const Pending: Story = {
  args: { state: "pending", approvedBlocks: 3, totalBlocks: 50, failingChecks: 2 },
};

export const AllStates: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <ShipStateBadge state="governed" approvedBlocks={50} totalBlocks={50} />
      <ShipStateBadge state="ai_shippable" approvedBlocks={12} totalBlocks={50} />
      <ShipStateBadge state="pending" approvedBlocks={3} totalBlocks={50} failingChecks={2} />
    </div>
  ),
};

export const Compact: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <ShipStateBadge compact state="governed" approvedBlocks={50} totalBlocks={50} />
      <ShipStateBadge compact state="ai_shippable" approvedBlocks={12} totalBlocks={50} />
      <ShipStateBadge compact state="pending" failingChecks={1} />
    </div>
  ),
};
