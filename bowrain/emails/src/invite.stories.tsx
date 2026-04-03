import type { Meta, StoryObj } from "@storybook/react-vite";
import { InviteEmail } from "./invite";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof InviteEmail> = {
  title: "Emails/Invite",
  component: InviteEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <InviteEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InviteEmail>;

export const Default: Story = {
  args: {
    workspaceName: "Acme Translations",
    role: "editor",
    joinURL: "https://app.bowrain.com/invite/abc123",
  },
};

export const OwnerInvite: Story = {
  args: {
    workspaceName: "Globex Corp",
    role: "owner",
    joinURL: "https://app.bowrain.com/invite/xyz789",
  },
};

export const ViewerInvite: Story = {
  args: {
    workspaceName: "Startup Inc",
    role: "viewer",
    joinURL: "https://app.bowrain.com/invite/viewer-456",
  },
};
