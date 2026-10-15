import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SettingsPage } from "../components/SettingsPage";

const meta: Meta<typeof SettingsPage> = {
  title: "Pages/SettingsPage",
  component: SettingsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof SettingsPage>;

/**
 * Default settings page with pre-loaded theme (no Wails API calls for settings).
 * Embedded CredentialsPage and PluginManager tabs will show empty state.
 */
export const Default: Story = {
  args: {
    theme: "system",
  },
};

/**
 * Dark theme pre-selected.
 */
export const DarkTheme: Story = {
  args: {
    theme: "dark",
  },
};

/** The home's Start here steps were dismissed; Settings offers them back. */
export const StartHereDismissed: Story = {
  args: {
    theme: "system",
    startHereDismissed: true,
    onRestoreStartHere: fn(),
  },
};
