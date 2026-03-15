import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AuditLogView } from "../../components/AuditLogView";
import { withProviders } from "../decorators";
import type { AuditEntry, ProjectInfo } from "../../types/api";

const now = new Date();
const ago = (minutes: number) => new Date(now.getTime() - minutes * 60_000).toISOString();

const sampleProjects: ProjectInfo[] = [
  {
    id: "proj-1",
    name: "Marketing Website",
    source_locale: "en-US",
    target_locales: ["fr-FR", "de-DE"],
    created_at: ago(10000),
    modified_at: ago(5),
  },
  {
    id: "proj-2",
    name: "Mobile App",
    source_locale: "en",
    target_locales: ["ja", "ko"],
    created_at: ago(20000),
    modified_at: ago(120),
  },
];

const sampleEntries: AuditEntry[] = [
  {
    id: 1, project_id: "proj-1", event_type: "project.created",
    actor: "alice@example.com", source: "web",
    data: JSON.stringify({ name: "Marketing Website" }), created_at: ago(5),
  },
  {
    id: 2, project_id: "proj-1", event_type: "item.created",
    actor: "alice@example.com", source: "web",
    data: JSON.stringify({ item_name: "locales/en.json", stream: "main", format: "json" }), created_at: ago(4),
  },
  {
    id: 3, project_id: "proj-1", event_type: "block.updated",
    actor: "bob@example.com", source: "editor",
    data: JSON.stringify({ block_id: "abc12345-def6-7890", item_name: "locales/en.json" }), created_at: ago(3),
  },
  {
    id: 4, project_id: "proj-1", event_type: "stream.created",
    actor: "alice@example.com", source: "web",
    data: JSON.stringify({ stream: "feature/translations", parent: "main" }), created_at: ago(2),
  },
  {
    id: 5, project_id: "proj-2", event_type: "connector.push.completed",
    actor: "ci-bot", source: "sync",
    data: JSON.stringify({ items: "src/i18n/en.json,src/i18n/common.json", push_id: "push-001" }), created_at: ago(90),
  },
];

const meta: Meta<typeof AuditLogView> = {
  title: "Pages/AuditLogView",
  component: AuditLogView,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AuditLogView>;

export const Default: Story = {
  args: {
    entries: sampleEntries,
    projects: sampleProjects,
    hasMore: true,
    onLoadMore: fn(),
    onFiltersChange: fn(),
    onSearchChange: fn(),
    activeFilters: [],
    activeSearch: "",
  },
};

export const Empty: Story = {
  args: {
    entries: [],
    projects: sampleProjects,
    hasMore: false,
    onFiltersChange: fn(),
    onSearchChange: fn(),
  },
};

export const WithFilters: Story = {
  args: {
    entries: sampleEntries.filter((e) => e.event_type.startsWith("project")),
    projects: sampleProjects,
    hasMore: false,
    activeFilters: [{ key: "type", value: "project" }],
    activeSearch: "",
    onFiltersChange: fn(),
    onSearchChange: fn(),
  },
};
