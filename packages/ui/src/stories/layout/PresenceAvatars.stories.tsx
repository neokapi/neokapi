import type { Meta, StoryObj } from "@storybook/react";
import { PresenceAvatars } from "../../components/PresenceAvatars";
import type { CollabUser } from "../../hooks/useCollaboration";

const users: CollabUser[] = [
  { userId: "u-1", name: "Alice", color: "#6366f1", avatarUrl: "" },
  { userId: "u-2", name: "Bob", color: "#f59e0b", avatarUrl: "" },
  { userId: "u-3", name: "Charlie", color: "#10b981", avatarUrl: "" },
  { userId: "u-4", name: "Diana", color: "#ef4444", avatarUrl: "" },
  { userId: "u-5", name: "Eve", color: "#8b5cf6", avatarUrl: "" },
  { userId: "u-6", name: "Frank", color: "#ec4899", avatarUrl: "" },
  { userId: "u-7", name: "Grace", color: "#14b8a6", avatarUrl: "" },
];

const meta: Meta<typeof PresenceAvatars> = {
  title: "Layout/PresenceAvatars",
  component: PresenceAvatars,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof PresenceAvatars>;

export const FewUsers: Story = {
  args: {
    users: users.slice(0, 3),
    currentUserId: "u-0",
  },
};

export const WithOverflow: Story = {
  args: {
    users,
    currentUserId: "u-0",
    maxVisible: 4,
  },
};

export const SingleUser: Story = {
  args: {
    users: users.slice(0, 1),
    currentUserId: "u-0",
  },
};

export const CurrentUserExcluded: Story = {
  args: {
    users: users.slice(0, 3),
    currentUserId: "u-1",
  },
};
