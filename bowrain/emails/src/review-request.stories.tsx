import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReviewRequestEmail } from "./review-request";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof ReviewRequestEmail> = {
  title: "Emails/ReviewRequest",
  component: ReviewRequestEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <ReviewRequestEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ReviewRequestEmail>;

export const Default: Story = {
  args: {
    workspaceName: "Acme Translations",
    changeSetName: "kapi push — governed concepts",
    authorName: "Asgeir",
    changeCount: "57 changes",
    reviewURL: "https://app.bowrain.cloud/acme/context/changes/FCfv5QTy",
  },
};

export const SingleChange: Story = {
  args: {
    workspaceName: "Globex Corp",
    changeSetName: "Ban “utilize” in product surfaces",
    authorName: "Dana",
    changeCount: "1 change",
    reviewURL: "https://app.bowrain.cloud/globex/context/changes/aB3xQ9",
  },
};
