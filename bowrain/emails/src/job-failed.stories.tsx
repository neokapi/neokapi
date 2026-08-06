import type { Meta, StoryObj } from "@storybook/react-vite";
import { JobFailedEmail } from "./job-failed";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof JobFailedEmail> = {
  title: "Emails/JobFailed",
  component: JobFailedEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <JobFailedEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof JobFailedEmail>;

export const Translation: Story = {
  args: {
    workspaceName: "Acme Translations",
    jobKind: "translation",
    subject: "en.json → nb",
    reason: "openai: API error 401: invalid api key",
    jobURL: "https://app.bowrain.cloud/acme/p/proj_7hK2/s/main/runs",
  },
};

export const Push: Story = {
  args: {
    workspaceName: "Globex Corp",
    jobKind: "push",
    subject: "Marketing site",
    reason: "parse manifest: unexpected end of JSON input",
    jobURL: "https://app.bowrain.cloud/globex/p/proj_9Qm4/s/main/runs",
  },
};
