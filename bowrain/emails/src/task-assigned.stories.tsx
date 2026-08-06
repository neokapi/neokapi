import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskAssignedEmail } from "./task-assigned";
import { EmailPreview } from "./storybook-decorator";

const meta: Meta<typeof TaskAssignedEmail> = {
  title: "Emails/TaskAssigned",
  component: TaskAssignedEmail,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (_, { args }) => (
      <EmailPreview>
        <TaskAssignedEmail {...args} />
      </EmailPreview>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TaskAssignedEmail>;

export const Urgent: Story = {
  args: {
    workspaceName: "Acme Translations",
    taskTitle: "Fix terminology in the Norwegian release notes",
    taskDescription: "Three forbidden terms shipped in the 1.4 notes and need replacing today.",
    priority: "urgent",
    assignerName: "Dana",
    taskURL: "https://app.bowrain.cloud/acme/tasks",
  },
};

export const High: Story = {
  args: {
    workspaceName: "Globex Corp",
    taskTitle: "Review the source change proposed on the pricing page",
    taskDescription: "",
    priority: "high",
    assignerName: "Asgeir",
    taskURL: "https://app.bowrain.cloud/globex/tasks",
  },
};
