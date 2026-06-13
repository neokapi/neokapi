import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { PilotsPanel } from "./PilotsPanel";
import { withBrandHub } from "../../stories/brandHubFixtures";
import type { ChangeSetDetail } from "../../types/brand-graph";

const now = "2026-06-13T10:00:00Z";

const base: ChangeSetDetail = {
  id: "cs-1",
  workspace_id: "ws-1",
  name: "Retire ‘utilize’",
  status: "in_review",
  created_by: "sam",
  created_at: now,
  updated_at: now,
  governed: true,
  ops: [],
  reviews: [],
  pilots: [
    {
      workspace_id: "ws-1",
      changeset_id: "cs-1",
      project_id: "Marketing Website",
      stream: "main",
      created_by: "sam",
      created_at: now,
    },
  ],
};

const pad: Decorator = (Story) => (
  <div style={{ maxWidth: 360, padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof PilotsPanel> = {
  title: "Brand Hub/Experiments/PilotsPanel",
  component: PilotsPanel,
  tags: ["autodocs"],
  decorators: [withBrandHub, pad],
};

export default meta;
type Story = StoryObj<typeof PilotsPanel>;

export const WithPilot: Story = { args: { changeset: base } };

export const Empty: Story = { args: { changeset: { ...base, pilots: [] } } };
