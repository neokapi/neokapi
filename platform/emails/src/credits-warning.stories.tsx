import type { Meta, StoryObj } from "@storybook/react-vite";
import { CreditsWarningEmail } from "./credits-warning";

const meta: Meta<typeof CreditsWarningEmail> = {
  title: "Emails/Credits Warning",
  component: CreditsWarningEmail,
  tags: ["autodocs"],
  parameters: {
    layout: "padded",
  },
};

export default meta;
type Story = StoryObj<typeof CreditsWarningEmail>;

export const At80Percent: Story = {
  args: {
    workspaceName: "Acme Translations",
    usedCredits: "400,000",
    totalCredits: "500,000",
    usagePercent: "80",
    resetDate: "Monday, March 23, 2026",
    upgradeURL: "https://app.bowrain.com/acme/settings/billing",
  },
};

export const At90Percent: Story = {
  args: {
    workspaceName: "Globex Corp",
    usedCredits: "1,800,000",
    totalCredits: "2,000,000",
    usagePercent: "90",
    resetDate: "Monday, March 30, 2026",
    upgradeURL: "https://app.bowrain.com/globex/settings/billing",
  },
};

export const FreePlan: Story = {
  args: {
    workspaceName: "My Project",
    usedCredits: "40,000",
    totalCredits: "50,000",
    usagePercent: "80",
    resetDate: "Monday, March 23, 2026",
    upgradeURL: "https://app.bowrain.com/my-project/settings/billing",
  },
};
