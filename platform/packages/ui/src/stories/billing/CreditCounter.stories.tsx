import type { Meta, StoryObj } from "@storybook/react-vite";
import { CreditCounter } from "../../components/billing/CreditCounter";

const meta: Meta<typeof CreditCounter> = {
  title: "Billing/CreditCounter",
  component: CreditCounter,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof CreditCounter>;

export const Default: Story = {
  args: {
    creditsUsed: 77_000,
    creditsTotal: 500_000,
  },
};

export const HighUsage: Story = {
  args: {
    creditsUsed: 450_000,
    creditsTotal: 500_000,
  },
};

export const Compact: Story = {
  args: {
    creditsUsed: 77_000,
    creditsTotal: 500_000,
    compact: true,
  },
};

export const CompactHighUsage: Story = {
  args: {
    creditsUsed: 450_000,
    creditsTotal: 500_000,
    compact: true,
  },
};

export const Variants: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Default</div>
        <CreditCounter creditsUsed={77_000} creditsTotal={500_000} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Compact</div>
        <CreditCounter creditsUsed={77_000} creditsTotal={500_000} compact />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Compact (high usage)</div>
        <CreditCounter creditsUsed={450_000} creditsTotal={500_000} compact />
      </div>
    </div>
  ),
};
