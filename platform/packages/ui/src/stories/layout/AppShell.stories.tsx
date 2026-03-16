import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AppShell } from "../../components/AppShell";
import type { SidebarContext } from "../../components/AppSidebar";
import { ThemeProvider } from "../../context/ThemeContext";
import { ApiProvider } from "../../context/ApiContext";
import { WorkspaceProvider } from "../../context/WorkspaceContext";
import { createMockAdapter } from "../mock-adapter";
import type { Workspace, ProjectInfo } from "../../types/api";

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

const sampleProject: ProjectInfo = {
  id: "proj-1",
  name: "Marketing Website",
  default_source_language: "en-US",
  target_languages: ["fr-FR", "de-DE", "ja-JP", "es-ES"],
  workspace_id: "ws-1",
  items: [
    {
      id: "itm-lnd",
      name: "landing.html",
      format: "html",
      type: "file",
      size: 12000,
      block_count: 48,
      word_count: 1320,
    },
    {
      id: "itm-abt",
      name: "about.json",
      format: "json",
      type: "file",
      size: 4200,
      block_count: 22,
      word_count: 280,
    },
    {
      id: "itm-faq",
      name: "faq.md",
      format: "md",
      type: "file",
      size: 8000,
      block_count: 35,
      word_count: 620,
    },
    {
      id: "itm-prc",
      name: "pricing.xliff",
      format: "xliff",
      type: "file",
      size: 6000,
      block_count: 18,
      word_count: 190,
    },
  ],
  streams: [
    {
      name: "main",
      parent: "",
      base_cursor: 0,
      archived: false,
      visibility: "public",
      description: "",
      created_at: "2025-12-01T10:00:00Z",
      created_by: "user-1",
    },
    {
      name: "q1-campaign",
      parent: "main",
      base_cursor: 5,
      archived: false,
      visibility: "public",
      description: "Q1 campaign",
      created_at: "2026-02-01T10:00:00Z",
      created_by: "user-1",
    },
    {
      name: "review/alice",
      parent: "main",
      base_cursor: 3,
      archived: false,
      visibility: "shared",
      description: "Alice's review",
      created_at: "2026-03-01T10:00:00Z",
      created_by: "user-2",
      shared_with: ["user-1"],
    },
  ],
  created_at: "2025-12-01T10:00:00Z",
  modified_at: "2026-03-13T09:15:00Z",
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

/** Workspace-level: default flat navigation with Projects, Brand Voice, Termbase, etc. */
export const WorkspaceLevel: Story = {
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
        sidebarContext={{ level: "workspace", activeView: view as "translate" }}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Workspace Home — Projects Dashboard
        </div>
      </AppShell>
    );
  },
};

/** Project-level: sidebar shows Dashboard (active) and Automations. */
export const ProjectLevel: Story = {
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "dashboard",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
      onCreateStream: fn(),
      onOpenAutomations: fn(),
    };
    return (
      <AppShell
        workspaces={[mockWorkspace, secondWorkspace]}
        activeWorkspace={mockWorkspace}
        onSelectWorkspace={() => {}}
        activeView="translate"
        onViewChange={() => {}}
        user={{ id: "u-1", email: "user@example.com", name: "Demo User", avatar_url: "" }}
        collapsed={collapsed}
        onCollapsedChange={setCollapsed}
        sidebarContext={ctx}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Project Detail — File list and upload area
        </div>
      </AppShell>
    );
  },
};

/** Automations page: sidebar highlights Automations, Home shows project name. */
export const AutomationsLevel: Story = {
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "automations",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
      onOpenAutomations: fn(),
    };
    return (
      <AppShell
        workspaces={[mockWorkspace, secondWorkspace]}
        activeWorkspace={mockWorkspace}
        onSelectWorkspace={() => {}}
        activeView="translate"
        onViewChange={() => {}}
        user={{ id: "u-1", email: "user@example.com", name: "Demo User", avatar_url: "" }}
        collapsed={collapsed}
        onCollapsedChange={setCollapsed}
        sidebarContext={ctx}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Project Automations
        </div>
      </AppShell>
    );
  },
};

/** Editor-level: sidebar shows Dashboard and Automations, Home shows project name. */
export const EditorLevel: Story = {
  render: () => {
    const [collapsed, setCollapsed] = useState(false);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "dashboard",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
      onOpenAutomations: fn(),
    };
    return (
      <AppShell
        workspaces={[mockWorkspace, secondWorkspace]}
        activeWorkspace={mockWorkspace}
        onSelectWorkspace={() => {}}
        activeView="translate"
        onViewChange={() => {}}
        user={{ id: "u-1", email: "user@example.com", name: "Demo User", avatar_url: "" }}
        collapsed={collapsed}
        onCollapsedChange={setCollapsed}
        sidebarContext={ctx}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Translation Editor — about.json (fr-FR)
        </div>
      </AppShell>
    );
  },
};

/** Project-level with sidebar collapsed to icon mode. */
export const ProjectCollapsed: Story = {
  render: () => {
    const [collapsed, setCollapsed] = useState(true);
    const ctx: SidebarContext = {
      level: "project",
      project: sampleProject,
      activeStream: "main",
      activeProjectView: "dashboard",
      onBack: fn(),
      onOpenDashboard: fn(),
      onOpenFile: fn(),
      onStreamChange: fn(),
    };
    return (
      <AppShell
        workspaces={[mockWorkspace]}
        activeWorkspace={mockWorkspace}
        onSelectWorkspace={() => {}}
        activeView="translate"
        onViewChange={() => {}}
        user={null}
        collapsed={collapsed}
        onCollapsedChange={setCollapsed}
        sidebarContext={ctx}
      >
        <div className="flex items-center justify-center h-full text-muted-foreground">
          Collapsed sidebar — project mode
        </div>
      </AppShell>
    );
  },
};

/** Workspace-level collapsed. */
export const WorkspaceCollapsed: Story = {
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
          Collapsed sidebar — workspace mode
        </div>
      </AppShell>
    );
  },
};
