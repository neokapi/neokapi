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
    creditsUsed: 120_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(3),
  },
};

export const MediumUsage: Story = {
  args: {
    creditsUsed: 350_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(2),
  },
};

export const HighUsage: Story = {
  args: {
    creditsUsed: 450_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(1),
  },
};

export const Exhausted: Story = {
  args: {
    creditsUsed: 500_000,
    creditsTotal: 500_000,
    weekEnd: futureDate(4),
  },
};

export const FreeplanSmall: Story = {
  args: {
    creditsUsed: 30_000,
    creditsTotal: 50_000,
    weekEnd: futureDate(5),
  },
};

export const AllLevels: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 24, maxWidth: 400 }}>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Low (24%)</div>
        <UsageBar creditsUsed={120_000} creditsTotal={500_000} weekEnd={futureDate(3)} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Medium (70%)</div>
        <UsageBar creditsUsed={350_000} creditsTotal={500_000} weekEnd={futureDate(2)} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>High (90%)</div>
        <UsageBar creditsUsed={450_000} creditsTotal={500_000} weekEnd={futureDate(1)} />
      </div>
      <div>
        <div style={{ fontSize: 12, marginBottom: 4, color: "#888" }}>Exhausted (100%)</div>
        <UsageBar creditsUsed={500_000} creditsTotal={500_000} weekEnd={futureDate(4)} />
      </div>
    </div>
  ),
};
