import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { ReviewsPanel } from "./ReviewsPanel";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ChangeSetDetail } from "../../types/brand-graph";
import type { User } from "../../types/api";

const now = "2026-06-13T10:00:00Z";

const changeset: ChangeSetDetail = {
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
  pilots: [],
};

function user(id: string): User {
  return { id, email: `${id}@demo.test`, name: id, avatar_url: "" };
}

function withUser(id: string): Decorator {
  return createProvidersDecorator(undefined, {
    ...brandHubOverrides,
    getCurrentUser: async () => user(id),
  });
}

const pad: Decorator = (Story) => (
  <div style={{ maxWidth: 360, padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof ReviewsPanel> = {
  title: "Brand Hub/Experiments/ReviewsPanel",
  component: ReviewsPanel,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof ReviewsPanel>;

/** A reviewer can approve or reject. */
export const AsReviewer: Story = {
  args: { changeset },
  decorators: [withUser("alex"), pad],
};

/** The author cannot approve their own experiment (separation of duties). */
export const AsAuthor: Story = {
  args: { changeset },
  decorators: [withUser("sam"), pad],
};
