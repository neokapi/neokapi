import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { AppShell } from "../../components/AppShell";
import { ThemeProvider } from "../../context/ThemeContext";
import { ApiProvider } from "../../context/ApiContext";
import { WorkspaceProvider } from "../../context/WorkspaceContext";
import { createMockAdapter } from "../mock-adapter";
import type { Workspace } from "../../types/api";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const secondWorkspace: Workspace = {
  id: "ws-2",
  name: "Acme Corp",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "editor",
};

const meta: Meta<typeof AppShell> = {
  title: "Layout/AppShell",
  component: AppShell,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <ThemeProvider>
        <ApiProvider adapter={createMockAdapter()}>
          <WorkspaceProvider initialWorkspace={mockWorkspace}>
            <div style={{ width: "100vw", height: "100vh", overflow: "hidden" }}>
              <Story />
            </div>
          </WorkspaceProvider>
        </ApiProvider>
      </ThemeProvider>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof AppShell>;

export const Default: Story = {
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const [view, setView] = useState("translate");
    return (
      <AppShell
        workspaces={[mockWorkspace, secondWorkspace]}
        activeWorkspace={mockWorkspace}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={() => {}}
        activeView={view}
        onViewChange={setView}
        user={{ id: "u-1", email: "user@example.com", name: "Demo User", avatar_url: "" }}
        onSignOut={() => {}}
        collapsed={collapsed}
        onCollapsedChange={setCollapsed}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Content Area
        </div>
      </AppShell>
    );
  },
};

export const Collapsed: Story = {
  render: () => {
    const [collapsed, setCollapsed] = useState(true);
    const [view, setView] = useState("translate");
    return (
      <AppShell
        workspaces={[mockWorkspace]}
        activeWorkspace={mockWorkspace}
        onSelectWorkspace={() => {}}
        activeView={view}
        onViewChange={setView}
        user={null}
        collapsed={collapsed}
        onCollapsedChange={setCollapsed}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Collapsed sidebar
        </div>
      </AppShell>
    );
  },
};
