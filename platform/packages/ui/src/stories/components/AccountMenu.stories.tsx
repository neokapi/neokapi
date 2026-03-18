import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AccountMenu } from "../../components/AccountMenu";
import { sampleUser } from "./fixtures";

const meta: Meta<typeof AccountMenu> = {
  title: "Components/AccountMenu",
  component: AccountMenu,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AccountMenu>;

export const Default: Story = {
  args: { user: sampleUser, onSignOut: fn(), onSettings: fn() },
};

export const IconVariant: Story = {
  args: { user: sampleUser, onSignOut: fn(), variant: "icon", status: "online" },
};

export const IconOffline: Story = {
  args: { user: sampleUser, onSignOut: fn(), variant: "icon", status: "offline" },
};

export const SidebarVariant: Story = {
  args: { user: sampleUser, onSignOut: fn(), variant: "sidebar", onSettings: fn() },
};

export const SidebarCollapsed: Story = {
  args: { user: sampleUser, onSignOut: fn(), variant: "sidebar", collapsed: true },
};
