import type { Meta, StoryObj } from "@storybook/react-vite";
import { UsageBar } from "../../components/billing/UsageBar";

const meta: Meta<typeof UsageBar> = {
  title: "Billing/UsageBar",
  component: UsageBar,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof UsageBar>;

const futureDate = (days: number) => new Date(Date.now() + days * 24 * 60 * 60 * 1000);

export const LowUsage: Story = {
  args: {
    creditsUsed: 490_000,
    creditsTotal: 2_000_000,
    resetsAt: futureDate(12),
  },
};

export const MediumUsage: Story = {
  args: {
    creditsUsed: 1_400_000,
    creditsTotal: 2_000_000,
    resetsAt: futureDate(8),
  },
};

export const HighUsage: Story = {
  args: {
    creditsUsed: 1_800_000,
    creditsTotal: 2_000_000,
    resetsAt: futureDate(3),
  },
};

export const Exhausted: Story = {
  args: {
    creditsUsed: 2_000_000,
    creditsTotal: 2_000_000,
    resetsAt: futureDate(14),
  },
};

export const AllLevels: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 24, maxWidth: 400 }}>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Low (24%)</div>
        <UsageBar creditsUsed={490_000} creditsTotal={2_000_000} resetsAt={futureDate(12)} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Medium (70%)</div>
        <UsageBar creditsUsed={1_400_000} creditsTotal={2_000_000} resetsAt={futureDate(8)} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>High (90%)</div>
        <UsageBar creditsUsed={1_800_000} creditsTotal={2_000_000} resetsAt={futureDate(3)} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Exhausted (100%)</div>
        <UsageBar creditsUsed={2_000_000} creditsTotal={2_000_000} resetsAt={futureDate(14)} />
      </div>
    </div>
  ),
};
