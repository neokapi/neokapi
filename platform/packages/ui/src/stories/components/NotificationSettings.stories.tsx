import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { NotificationSettings } from "../../components/NotificationSettings";

const meta: Meta<typeof NotificationSettings> = {
  title: "Notifications/NotificationSettings",
  component: NotificationSettings,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24, maxWidth: 560 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof NotificationSettings>;

export const Default: Story = {
  args: {
    settings: {
      frequency: "daily",
      quiet_start: "22:00",
      quiet_end: "08:00",
      timezone: "America/New_York",
    },
    onChange: fn(),
  },
};

export const QuietHoursOff: Story = {
  args: {
    settings: {
      frequency: "daily",
      quiet_start: "",
      quiet_end: "",
      timezone: "UTC",
    },
    onChange: fn(),
  },
};

export const WeeklyDigest: Story = {
  args: {
    settings: {
      frequency: "weekly",
      quiet_start: "23:00",
      quiet_end: "07:00",
      timezone: "Europe/Berlin",
    },
    onChange: fn(),
  },
};

export const DigestOff: Story = {
  args: {
    settings: {
      frequency: "off",
      quiet_start: "",
      quiet_end: "",
      timezone: "Asia/Tokyo",
    },
    onChange: fn(),
  },
};

export const Saving: Story = {
  args: {
    settings: {
      frequency: "daily",
      quiet_start: "22:00",
      quiet_end: "08:00",
      timezone: "UTC",
    },
    onChange: fn(),
    saving: true,
  },
};
