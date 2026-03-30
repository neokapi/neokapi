import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AppHome } from "../components/AppHome";

const meta: Meta<typeof AppHome> = {
  title: "Components/AppHome",
  component: AppHome,
  tags: ["autodocs"],
  args: {
    onOpenRecent: fn(),
    onNewProject: fn(),
    onOpenProject: fn(),
    onNavigate: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof AppHome>;

export const WithRecentProjects: Story = {
  args: {
    recentFiles: [
      {
        path: "/Users/dev/projects/acme-app/project.kapi",
        name: "Acme App Localization",
        opened_at: "2026-03-29T14:30:00Z",
      },
      {
        path: "/Users/dev/projects/website-i18n/project.kapi",
        name: "Website i18n",
        opened_at: "2026-03-28T09:15:00Z",
      },
      {
        path: "/Users/dev/projects/mobile-strings/project.kapi",
        name: "Mobile Strings",
        opened_at: "2026-03-25T16:45:00Z",
      },
    ],
  },
};

export const Empty: Story = {
  args: {
    recentFiles: [],
  },
};
