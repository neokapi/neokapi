import type { Meta, StoryObj } from "@storybook/react-vite";
import { SubscriptionBadge } from "../../components/billing/SubscriptionBadge";

const meta: Meta<typeof SubscriptionBadge> = {
  title: "Billing/SubscriptionBadge",
  component: SubscriptionBadge,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof SubscriptionBadge>;

export const FreeActive: Story = {
  args: { plan: "free", status: "active" },
};

export const ProActive: Story = {
  args: { plan: "pro", status: "active" },
};

export const TeamTrialing: Story = {
  args: { plan: "team", status: "trialing" },
};

export const EnterprisePastDue: Story = {
  args: { plan: "enterprise", status: "past_due" },
};

export const ProCanceled: Story = {
  args: { plan: "pro", status: "canceled" },
};

export const PlanOnly: Story = {
  args: { plan: "team" },
};

export const AllCombinations: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {(["free", "pro", "team", "enterprise"] as const).map((plan) =>
        (["active", "trialing", "past_due", "canceled"] as const).map((status) => (
          <div key={`${plan}-${status}`} style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{ width: 140, fontSize: 12, color: "#888" }}>
              {plan} / {status}
            </span>
            <SubscriptionBadge plan={plan} status={status} />
          </div>
        )),
      )}
    </div>
  ),
};
