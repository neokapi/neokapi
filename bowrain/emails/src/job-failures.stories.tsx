import type { Meta, StoryObj } from "@storybook/react-vite";
import { JobFailuresEmail } from "./job-failures";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof JobFailuresEmail> = {
  title: "Emails/JobFailures",
  component: JobFailuresEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <JobFailuresEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof JobFailuresEmail>;

export const UsageLimit: Story = {
  args: {
    workspaceName: "Acme Translations",
    jobKind: "translation",
    count: "412",
    reason: "workspace AI quota exceeded",
    jobURL: "https://app.bowrain.cloud/acme/p/proj_7hK2/s/main/runs",
  },
};

export const ProviderKey: Story = {
  args: {
    workspaceName: "Globex Corp",
    jobKind: "translation",
    count: "18",
    reason: "openai: API error 401: invalid api key",
    jobURL: "https://app.bowrain.cloud/globex/p/proj_9Qm4/s/main/runs",
  },
};
