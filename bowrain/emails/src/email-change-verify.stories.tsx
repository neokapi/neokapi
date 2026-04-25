import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmailChangeVerifyEmail } from "./email-change-verify";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof EmailChangeVerifyEmail> = {
  title: "Emails/Email Change Verify",
  component: EmailChangeVerifyEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <EmailChangeVerifyEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof EmailChangeVerifyEmail>;

export const Default: Story = {
  args: {
    newEmail: "asgeir@frimannsson.com",
    confirmURL: "https://app.bowrain.cloud/account/confirm-email?token=abc123",
    expiresIn: "24 hours",
  },
};
