import type { Meta, StoryObj } from "@storybook/react-vite";
import { SubscriptionChangedEmail } from "./subscription-changed";

const meta: Meta<typeof SubscriptionChangedEmail> = {
  title: "Emails/Subscription Changed",
  component: SubscriptionChangedEmail,
  tags: ["autodocs"],
  parameters: {
    layout: "padded",
  },
};

export default meta;
type Story = StoryObj<typeof SubscriptionChangedEmail>;

export const UpgradedToPro: Story = {
  args: {
    workspaceName: "Acme Translations",
    planName: "Pro",
    status: "Active",
    billingURL: "https://app.bowrain.com/acme/settings/billing",
  },
};

export const UpgradedToTeam: Story = {
  args: {
    workspaceName: "Globex Corp",
    planName: "Team",
    status: "Active",
    billingURL: "https://app.bowrain.com/globex/settings/billing",
  },
};

export const DowngradedToFree: Story = {
  args: {
    workspaceName: "Startup Inc",
    planName: "Free",
    status: "Active",
    billingURL: "https://app.bowrain.com/startup/settings/billing",
  },
};

export const TrialStarted: Story = {
  args: {
    workspaceName: "New Workspace",
    planName: "Pro (Trial)",
    status: "Trialing",
    billingURL: "https://app.bowrain.com/new-workspace/settings/billing",
  },
};

export const Canceled: Story = {
  args: {
    workspaceName: "Old Project",
    planName: "Pro",
    status: "Canceled",
    billingURL: "https://app.bowrain.com/old-project/settings/billing",
  },
};
