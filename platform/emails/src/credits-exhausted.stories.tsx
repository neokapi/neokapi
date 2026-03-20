import type { Meta, StoryObj } from "@storybook/react-vite";
import { CreditsExhaustedEmail } from "./credits-exhausted";

const meta: Meta<typeof CreditsExhaustedEmail> = {
  title: "Emails/Credits Exhausted",
  component: CreditsExhaustedEmail,
  tags: ["autodocs"],
  parameters: {
    layout: "padded",
  },
};

export default meta;
type Story = StoryObj<typeof CreditsExhaustedEmail>;

export const Default: Story = {
  args: {
    workspaceName: "Acme Translations",
    resetDate: "Monday, March 23, 2026",
    upgradeURL: "https://app.bowrain.com/acme/settings/billing",
    buyCreditsURL: "https://app.bowrain.com/acme/settings/billing?buy-credits=1",
  },
};

export const FreePlanExhausted: Story = {
  args: {
    workspaceName: "My Hobby Project",
    resetDate: "Monday, March 30, 2026",
    upgradeURL: "https://app.bowrain.com/hobby/settings/billing",
    buyCreditsURL: "https://app.bowrain.com/hobby/settings/billing?buy-credits=1",
  },
};
