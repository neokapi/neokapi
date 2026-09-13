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

/** Every translation is human-approved, and no terms apply to the language. */
export const Approved: Story = {
  args: { state: "approved", approvedBlocks: 50, totalBlocks: 50, termsNotGoverned: true },
};

export const AIShippable: Story = {
  args: { state: "ai_shippable", approvedBlocks: 12, totalBlocks: 50, failingChecks: 0 },
};

/** Shippable on machine review, in a language no terms govern: the tooltip says so. */
export const AIShippableNotGoverned: Story = {
  args: { state: "ai_shippable", approvedBlocks: 12, totalBlocks: 50, termsNotGoverned: true },
};

/** Pending on terminology: terms govern the language and some blocks have no result. */
export const PendingOnTerminology: Story = {
  args: { state: "pending", approvedBlocks: 50, totalBlocks: 50, termsNotCheckedBlocks: 2 },
};

export const Pending: Story = {
  args: { state: "pending", approvedBlocks: 3, totalBlocks: 50, failingChecks: 2 },
};

/** Pending on staleness: what the pairs wait on, split between the loop and a reviewer. */
export const PendingOnStaleness: Story = {
  args: {
    state: "pending",
    approvedBlocks: 40,
    totalBlocks: 50,
    staleAwaitingDraft: 6,
    staleAwaitingReview: 4,
  },
};

/** Pending on a rejection: a person refused the wording, so a pass owes a draft. */
export const PendingOnRejection: Story = {
  args: {
    state: "pending",
    approvedBlocks: 44,
    totalBlocks: 50,
    rejectedAwaitingDraft: 3,
  },
};

export const AllStates: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <ShipStateBadge state="governed" approvedBlocks={50} totalBlocks={50} />
      <ShipStateBadge state="approved" approvedBlocks={50} totalBlocks={50} termsNotGoverned />
      <ShipStateBadge state="ai_shippable" approvedBlocks={12} totalBlocks={50} />
      <ShipStateBadge state="pending" approvedBlocks={3} totalBlocks={50} failingChecks={2} />
    </div>
  ),
};

export const Compact: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <ShipStateBadge compact state="governed" approvedBlocks={50} totalBlocks={50} />
      <ShipStateBadge
        compact
        state="approved"
        approvedBlocks={50}
        totalBlocks={50}
        termsNotGoverned
      />
      <ShipStateBadge compact state="ai_shippable" approvedBlocks={12} totalBlocks={50} />
      <ShipStateBadge compact state="pending" failingChecks={1} />
    </div>
  ),
};
