import type { Meta, StoryObj } from "@storybook/react-vite";
import { NotificationEmail } from "./notification";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof NotificationEmail> = {
  title: "Emails/Notification",
  component: NotificationEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <NotificationEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof NotificationEmail>;

export const QualityGateFailed: Story = {
  args: {
    title: "Quality gate failed: Terminology check",
    body: "3 terminology violations were found in fr-FR for the Website project. The terms \"cloud computing\", \"machine learning\", and \"dashboard\" do not match your approved glossary entries. Please review and correct these before the next build.",
    category: "Quality",
    priority: "high",
    actionURL: "https://app.bowrain.com/ws/acme/projects/website/quality",
    actionLabel: "Review Issues",
  },
};

export const FlowFailed: Story = {
  args: {
    title: "Flow failed: Auto-translate (ja-JP)",
    body: "The auto-translate flow for Japanese in the Mobile App project failed after processing 42 of 128 blocks. The MT provider returned a rate-limit error. You can retry the flow or switch to a different provider.",
    category: "Automation",
    priority: "high",
    actionURL: "https://app.bowrain.com/ws/acme/projects/mobile/flows",
    actionLabel: "View Flow Details",
  },
};

export const DeadlineApproaching: Story = {
  args: {
    title: "Deadline approaching: Review mobile content",
    body: "The task \"Review mobile content\" for ja-JP is due in less than 24 hours. There are 42 blocks remaining to review. Please complete your review to avoid delays in the release schedule.",
    category: "Task",
    priority: "high",
    actionURL: "https://app.bowrain.com/ws/acme/tasks/task-123",
    actionLabel: "Open Task",
  },
};

export const TaskAssigned: Story = {
  args: {
    title: "New task: Review French translations",
    body: "Alice assigned you to review 24 blocks in fr-FR for the Mobile App project. The blocks are part of the new onboarding flow and include UI labels and help text.",
    category: "Task",
    priority: "normal",
    actionURL: "https://app.bowrain.com/ws/acme/tasks/task-456",
    actionLabel: "View Task",
  },
};

export const ContentAvailable: Story = {
  args: {
    title: "New content available for translation",
    body: "12 new blocks have been pushed to the Mobile App project. The content includes updated checkout flow labels and error messages. These blocks are ready for translation into your assigned languages.",
    category: "Project",
    priority: "normal",
    actionURL: "https://app.bowrain.com/ws/acme/projects/mobile/editor",
    actionLabel: "Start Translating",
  },
};

export const MentionNotification: Story = {
  args: {
    title: "Alice mentioned you",
    body: "\"@charlie can you review the updated glossary terms for the German locale? I've added 15 new entries based on the brand guide update from last week.\"",
    category: "Mention",
    priority: "normal",
    actionURL: "https://app.bowrain.com/ws/acme/projects/website/editor/block/123",
    actionLabel: "View Comment",
  },
};
