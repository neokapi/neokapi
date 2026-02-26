import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { TopBar } from "../../components/TopBar";
import { ThemeProvider } from "../../context/ThemeContext";

const meta: Meta<typeof TopBar> = {
  title: "Layout/TopBar",
  component: TopBar,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <ThemeProvider>
        <div style={{ width: "100%", maxWidth: 800 }}>
          <Story />
        </div>
      </ThemeProvider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TopBar>;

export const SignedIn: Story = {
  args: {
    user: { id: "u-1", email: "translator@example.com", name: "Jane Doe", avatar_url: "" },
    onSignOut: fn(),
    connectionState: "connected",
  },
};

export const Offline: Story = {
  args: {
    user: { id: "u-1", email: "translator@example.com", name: "Jane Doe", avatar_url: "" },
    onSignOut: fn(),
    connectionState: "offline",
    pendingChanges: 3,
  },
};

export const NoUser: Story = {
  args: {
    user: null,
  },
};
