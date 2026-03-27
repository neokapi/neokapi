import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TaskBoard } from "../../components/TaskBoard";
import type { TaskInfo } from "../../types/api";

const now = new Date();
const ago = (minutes: number) => new Date(now.getTime() - minutes * 60_000).toISOString();
const fromNow = (minutes: number) => new Date(now.getTime() + minutes * 60_000).toISOString();

const sampleTasks: TaskInfo[] = [
  {
    id: "task-1",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "translate",
    status: "open",
    priority: "high",
    title: "Translate marketing page to French",
    description: "The homepage and about page need French translations before the Q2 launch.",
    assignee_id: "user-2",
    created_by: "user-1",
    due_at: fromNow(1440),
    created_at: ago(120),
    updated_at: ago(60),
  },
  {
    id: "task-2",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "review",
    status: "in_progress",
    priority: "normal",
    title: "Review German translations",
    assignee_id: "user-3",
    created_by: "user-1",
    due_at: fromNow(2880),
    created_at: ago(240),
    updated_at: ago(30),
  },
  {
    id: "task-3",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "fix_quality",
    status: "open",
    priority: "urgent",
    title: "Fix QA issues in Japanese build",
    description: "3 critical QA issues found in the latest Japanese translation batch.",
    created_by: "user-1",
    due_at: ago(60), // overdue
    created_at: ago(480),
    updated_at: ago(480),
  },
  {
    id: "task-4",
    workspace_id: "ws-1",
    project_id: "proj-2",
    type: "fix_brand_voice",
    status: "open",
    priority: "normal",
    title: "Align mobile content with brand voice",
    assignee_id: "user-2",
    created_by: "user-1",
    created_at: ago(600),
    updated_at: ago(600),
  },
  {
    id: "task-5",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "translate",
    status: "completed",
    priority: "normal",
    title: "Translate onboarding flow to Spanish",
    assignee_id: "user-2",
    created_by: "user-1",
    completed_by: "user-2",
    created_at: ago(10080),
    updated_at: ago(2880),
    completed_at: ago(2880),
  },
  {
    id: "task-6",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "review_terms",
    status: "cancelled",
    priority: "low",
    title: "Review deprecated terms in glossary",
    created_by: "user-1",
    created_at: ago(20160),
    updated_at: ago(14400),
  },
  {
    id: "task-7",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "connector_setup",
    status: "in_progress",
    priority: "high",
    title: "Configure GitHub connector for docs repo",
    description: "Set up bidirectional sync with the documentation repository.",
    assignee_id: "user-1",
    created_by: "user-1",
    created_at: ago(1440),
    updated_at: ago(60),
  },
  {
    id: "task-8",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "source_review",
    status: "open",
    priority: "normal",
    title: "Review source content before translation",
    description: "Check placeholders, terminology, and DNT tags before fan-out to target languages.",
    assignee_id: "user-1",
    created_by: "system",
    data: { push_id: "push-abc", items: "en.json" },
    created_at: ago(30),
    updated_at: ago(30),
  },
];

const meta: Meta<typeof TaskBoard> = {
  title: "Pages/TaskBoard",
  component: TaskBoard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TaskBoard>;

export const Default: Story = {
  args: {
    tasks: sampleTasks,
    currentUserId: "user-1",
    onCompleteTask: fn(),
    onCancelTask: fn(),
    onTaskClick: fn(),
  },
};

export const Empty: Story = {
  args: {
    tasks: [],
  },
};

export const Loading: Story = {
  args: {
    tasks: [],
    loading: true,
  },
};

export const OpenOnly: Story = {
  args: {
    tasks: sampleTasks.filter((t) => t.status === "open"),
    onCompleteTask: fn(),
    onCancelTask: fn(),
    onTaskClick: fn(),
  },
};

export const WithOverdue: Story = {
  args: {
    tasks: sampleTasks.filter((t) => t.status === "open" || t.status === "in_progress"),
    onCompleteTask: fn(),
    onCancelTask: fn(),
    onTaskClick: fn(),
  },
};
