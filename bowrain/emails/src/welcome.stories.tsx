import type { Meta, StoryObj } from "@storybook/react-vite";
import { WelcomeEmail } from "./welcome";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof WelcomeEmail> = {
  title: "Emails/Welcome",
  component: WelcomeEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <WelcomeEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WelcomeEmail>;

export const Default: Story = {
  args: {
    workspaceName: "Acme Translations",
    workspaceURL: "https://app.bowrain.cloud/acme",
  },
};

// The handle a personal workspace gets when the person kept the suggested slug:
// the name is a login, not a company, and the copy has to read the same.
export const PersonalWorkspace: Story = {
  args: {
    workspaceName: "dana",
    workspaceURL: "https://app.bowrain.cloud/dana",
  },
};
