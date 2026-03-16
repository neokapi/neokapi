import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ActivityFeed } from "../../components/ActivityFeed";
import type { ActivityInfo } from "../../types/api";

const now = new Date();
const ago = (minutes: number) => new Date(now.getTime() - minutes * 60_000).toISOString();

const sampleActivities: ActivityInfo[] = [
  {
    id: "act-1",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "user-1",
    actor_name: "Alice Chen",
    type: "block.translated",
    entity_type: "block",
    entity_id: "blk-001",
    summary: "updated block greeting.title",
    data: { name: "Marketing Website" },
    created_at: ago(2),
  },
  {
    id: "act-2",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "user-2",
    actor_name: "Bob Martinez",
    type: "stream.created",
    entity_type: "stream",
    entity_id: "feature/translations",
    summary: "created stream feature/translations",
    created_at: ago(15),
  },
  {
    id: "act-3",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "system",
    actor_name: "System",
    type: "flow.completed",
    entity_type: "flow",
    summary: "flow completed",
    created_at: ago(30),
  },
  {
    id: "act-4",
    workspace_id: "ws-1",
    project_id: "proj-2",
    actor_id: "user-1",
    actor_name: "Alice Chen",
    type: "project.created",
    entity_type: "project",
    entity_id: "proj-2",
    summary: "created project Mobile App",
    data: { name: "Mobile App" },
    created_at: ago(120),
  },
  {
    id: "act-5",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "system",
    actor_name: "System",
    type: "gate.failed",
    entity_type: "gate",
    summary: "quality gate failed",
    created_at: ago(180),
  },
  {
    id: "act-6",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "system",
    actor_name: "System",
    type: "brand.drift",
    entity_type: "brand",
    summary: "brand voice drift detected",
    created_at: ago(300),
  },
  {
    id: "act-7",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "user-3",
    actor_name: "Carol Wang",
    type: "item.pushed",
    entity_type: "item",
    summary: "pushed locales/en.json, locales/fr.json",
    created_at: ago(500),
  },
  {
    id: "act-8",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "system",
    actor_name: "System",
    type: "extraction.done",
    entity_type: "extraction",
    summary: "extraction completed",
    created_at: ago(1440),
  },
  {
    id: "act-9",
    workspace_id: "ws-1",
    project_id: "proj-1",
    actor_id: "user-2",
    actor_name: "Bob Martinez",
    type: "stream.merged",
    entity_type: "stream",
    entity_id: "feature/translations",
    summary: "merged stream feature/translations",
    created_at: ago(2880),
  },
];

const meta: Meta<typeof ActivityFeed> = {
  title: "Pages/ActivityFeed",
  component: ActivityFeed,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ActivityFeed>;

export const Default: Story = {
  args: {
    activities: sampleActivities,
    hasMore: true,
    onLoadMore: fn(),
    onActivityClick: fn(),
  },
};

export const Empty: Story = {
  args: {
    activities: [],
    hasMore: false,
  },
};

export const Loading: Story = {
  args: {
    activities: [],
    loading: true,
  },
};

export const NoMore: Story = {
  args: {
    activities: sampleActivities.slice(0, 3),
    hasMore: false,
    onActivityClick: fn(),
  },
};

export const FailureEvents: Story = {
  args: {
    activities: sampleActivities.filter(
      (a) => a.type.includes("failed") || a.type.includes("drift"),
    ),
    hasMore: false,
    onActivityClick: fn(),
  },
};
