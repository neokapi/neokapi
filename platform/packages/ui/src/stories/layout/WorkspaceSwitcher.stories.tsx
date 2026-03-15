import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceSwitcher } from "../../components/WorkspaceSwitcher";
import { SidebarProvider } from "../../components/ui/sidebar";
import type { Workspace } from "../../types/api";

const workspaces: Workspace[] = [
  {
    id: "ws-1",
    name: "Personal",
    slug: "personal",
    description: "",
    logo_url: "",
    type: "personal",
    role: "owner",
  },
  {
    id: "ws-2",
    name: "Acme Corp",
    slug: "acme",
    description: "",
    logo_url: "",
    type: "team",
    role: "editor",
  },
  {
    id: "ws-3",
    name: "Globex Inc",
    slug: "globex",
    description: "",
    logo_url: "",
    type: "team",
    role: "owner",
  },
];

const meta: Meta<typeof WorkspaceSwitcher> = {
  title: "Layout/WorkspaceSwitcher",
  component: WorkspaceSwitcher,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <SidebarProvider open>
        <div style={{ width: 220, padding: 8 }}>
          <Story />
        </div>
      </SidebarProvider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceSwitcher>;

export const Default: Story = {
  args: {
    workspaces,
    activeWorkspace: workspaces[0],
    onSelectWorkspace: fn(),
    onCreateWorkspace: fn(),
  },
};

export const SingleWorkspace: Story = {
  args: {
    workspaces: [workspaces[0]],
    activeWorkspace: workspaces[0],
    onSelectWorkspace: fn(),
  },
};
