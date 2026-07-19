import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ReviewInbox } from "../../components/review/ReviewInbox";

const meta: Meta<typeof ReviewInbox> = {
  title: "Review/ReviewInbox",
  component: ReviewInbox,
  args: { onOpenReview: fn() },
};
export default meta;
type Story = StoryObj<typeof ReviewInbox>;

/** Several projects awaiting review, most-pending first. */
export const Default: Story = {
  args: {
    projects: [
      {
        projectId: "p1",
        projectName: "Marketing Website",
        stream: "main",
        pending: 12,
        governed: 3,
        aiShippable: 5,
      },
      {
        projectId: "p2",
        projectName: "Mobile App",
        stream: "main",
        pending: 4,
        governed: 8,
        aiShippable: 2,
      },
      {
        projectId: "p3",
        projectName: "Help Center",
        stream: "main",
        pending: 1,
        governed: 10,
        aiShippable: 0,
      },
    ],
  },
};

/** Nothing awaiting review across the workspace. */
export const Empty: Story = {
  args: {
    projects: [
      {
        projectId: "p1",
        projectName: "Marketing Website",
        stream: "main",
        pending: 0,
        governed: 9,
        aiShippable: 3,
      },
    ],
  },
};

export const Loading: Story = {
  args: { projects: [], loading: true },
};
