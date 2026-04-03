import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ActivityFeed } from "../components/ActivityFeed";
import type { ActivityInfo } from "../types/api";

const meta: Meta<typeof ActivityFeed> = {
  title: "Pages/Activity/ActivityFeedAgent",
  component: ActivityFeed,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ActivityFeed>;

const now = Date.now();
const agentActivities: ActivityInfo[] = [
  {
    id: "1",
    workspace_id: "ws-1",
    actor_id: "u1",
    actor_name: "Alice",
    type: "agent.conversation.created",
    summary: "",
    data: { title: "Translate French files" },
    created_at: new Date(now - 60000).toISOString(),
  },
  {
    id: "2",
    workspace_id: "ws-1",
    actor_id: "u1",
    actor_name: "Alice",
    type: "agent.message.sent",
    summary: "",
    data: { blocks_count: "45" },
    created_at: new Date(now - 120000).toISOString(),
  },
  {
    id: "3",
    workspace_id: "ws-1",
    actor_id: "bravo",
    actor_name: "@bravo",
    type: "agent.tool.executed",
    summary: "",
    data: { tool: "run_flow", duration: "2.3s" },
    created_at: new Date(now - 180000).toISOString(),
  },
  {
    id: "4",
    workspace_id: "ws-1",
    actor_id: "u1",
    actor_name: "Alice",
    type: "agent.tool.approved",
    summary: "",
    data: { tool: "connector_push" },
    created_at: new Date(now - 240000).toISOString(),
  },
  {
    id: "5",
    workspace_id: "ws-1",
    actor_id: "u1",
    actor_name: "Bob",
    type: "agent.tool.denied",
    summary: "",
    data: { tool: "execute_script" },
    created_at: new Date(now - 300000).toISOString(),
  },
  {
    id: "6",
    workspace_id: "ws-1",
    actor_id: "bravo",
    actor_name: "@bravo",
    type: "agent.code.executed",
    summary: "",
    data: { language: "python", exit_code: "0" },
    created_at: new Date(now - 360000).toISOString(),
  },
  {
    id: "7",
    workspace_id: "ws-1",
    actor_id: "bravo",
    actor_name: "@bravo",
    type: "agent.code.executed",
    summary: "",
    data: { language: "bash", exit_code: "1" },
    created_at: new Date(now - 420000).toISOString(),
  },
];

const mixedActivities: ActivityInfo[] = [
  {
    id: "m1",
    workspace_id: "ws-1",
    actor_id: "u1",
    actor_name: "Alice",
    type: "project.created",
    summary: "created project Mobile App",
    data: { name: "Mobile App" },
    project_id: "p1",
    created_at: new Date(now - 60000).toISOString(),
  },
  ...agentActivities.slice(0, 3),
  {
    id: "m2",
    workspace_id: "ws-1",
    actor_id: "u2",
    actor_name: "Bob",
    type: "flow.completed",
    summary: "completed pseudo-translate flow",
    created_at: new Date(now - 500000).toISOString(),
  },
];

export const AgentEventsOnly: Story = {
  args: {
    activities: agentActivities,
    onActivityClick: fn(),
  },
};

export const MixedWithAgentEvents: Story = {
  args: {
    activities: mixedActivities,
    hasMore: true,
    onLoadMore: fn(),
    onActivityClick: fn(),
  },
};

export const Empty: Story = {
  args: {
    activities: [],
  },
};
