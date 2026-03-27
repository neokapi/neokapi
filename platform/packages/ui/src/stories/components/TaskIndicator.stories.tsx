import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TaskIndicator } from "../../components/ActivityTaskIndicators";
import { sampleTasks } from "./fixtures";

const meta: Meta<typeof TaskIndicator> = {
  title: "Pages/Activity/TaskIndicator",
  component: TaskIndicator,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TaskIndicator>;

export const WithTasks: Story = {
  args: { tasks: sampleTasks, onTaskClick: fn(), onCompleteTask: fn(), onViewAll: fn() },
};

export const Empty: Story = {
  args: { tasks: [], onTaskClick: fn() },
};
