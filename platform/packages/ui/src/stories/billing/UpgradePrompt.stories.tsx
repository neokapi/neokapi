import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { UpgradePrompt } from "../../components/billing/UpgradePrompt";

const meta: Meta<typeof UpgradePrompt> = {
  title: "Billing/UpgradePrompt",
  component: UpgradePrompt,
  tags: ["autodocs"],
  args: { onUpgrade: fn() },
};

export default meta;
type Story = StoryObj<typeof UpgradePrompt>;

export const GitConnectors: Story = {
  args: {
    feature: "Git connectors",
    minimumPlan: "pro",
    currentPlan: "free",
  },
};

export const CodeExecution: Story = {
  args: {
    feature: "@bravo code execution",
    minimumPlan: "team",
    currentPlan: "pro",
  },
};

export const SSOSAML: Story = {
  args: {
    feature: "SSO/SAML",
    minimumPlan: "enterprise",
    currentPlan: "team",
  },
};

export const APIAccess: Story = {
  args: {
    feature: "API access",
    minimumPlan: "pro",
    currentPlan: "free",
  },
};

export const AllPrompts: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 400 }}>
      <UpgradePrompt feature="Git connectors" minimumPlan="pro" currentPlan="free" />
      <UpgradePrompt feature="@bravo code execution" minimumPlan="team" currentPlan="pro" />
      <UpgradePrompt feature="SSO/SAML" minimumPlan="enterprise" currentPlan="team" />
    </div>
  ),
};
