import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { NotificationCenter } from "../../components/NotificationCenter";
import { sampleNotifications } from "./fixtures";

const meta: Meta<typeof NotificationCenter> = {
  title: "Components/NotificationCenter",
  component: NotificationCenter,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ padding: 24 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof NotificationCenter>;

export const WithUnread: Story = {
  args: {
    notifications: sampleNotifications,
    unreadCount: 2,
    onMarkRead: fn(),
    onMarkAllRead: fn(),
    onDelete: fn(),
    onNotificationClick: fn(),
  },
};

export const AllRead: Story = {
  args: {
    notifications: sampleNotifications.map((n) => ({ ...n, read: true })),
    unreadCount: 0,
    onMarkRead: fn(),
    onMarkAllRead: fn(),
    onDelete: fn(),
  },
};

export const Empty: Story = {
  args: {
    notifications: [],
    unreadCount: 0,
    onMarkRead: fn(),
    onMarkAllRead: fn(),
    onDelete: fn(),
  },
};
