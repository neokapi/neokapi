import type { Meta, StoryObj } from "@storybook/react-vite";
import { GovernanceSettings } from "../../components/GovernanceSettings";
import { createProvidersDecorator } from "../decorators";
import type { DenyRule, Group } from "../../types/api";

const groups: Group[] = [
  {
    id: "g1",
    workspace_id: "ws-1",
    name: "Reviewers",
    description: "",
    created_at: "2026-01-01T00:00:00Z",
    member_count: 3,
  },
  {
    id: "g2",
    workspace_id: "ws-1",
    name: "Nordic marketing",
    description: "",
    created_at: "2026-01-01T00:00:00Z",
    member_count: 5,
  },
];

const denyRules: DenyRule[] = [
  {
    id: "d1",
    workspace_id: "ws-1",
    subject_type: "role",
    subject_id: "viewer",
    project_id: "",
    denied_perms: 4,
    reason: "",
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "d2",
    workspace_id: "ws-1",
    subject_type: "group",
    subject_id: "g2",
    project_id: "proj-1",
    denied_perms: 32,
    reason: "",
    created_at: "2026-01-01T00:00:00Z",
  },
];

const populated = createProvidersDecorator(undefined, {
  getSoDMode: async () => ({ mode: "block" as const }),
  listGroups: async () => groups,
  listDenyRules: async () => denyRules,
  listRoleOverrides: async () => ({ member: ["review", "translate"], viewer: ["read"] }),
});

const meta: Meta<typeof GovernanceSettings> = {
  title: "Pages/GovernanceSettings",
  component: GovernanceSettings,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "The workspace's governance: who may decide (separation of duties, role overrides, deny rules) and the teams decisions are granted to. Where content sits and what governs it there are project settings.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof GovernanceSettings>;

export const Populated: Story = {
  decorators: [populated],
};

export const Empty: Story = {
  name: "Nothing set (defaults)",
  decorators: [createProvidersDecorator()],
};

export const Dark: Story = {
  globals: { theme: "dark" },
  decorators: [populated],
};
