import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TooltipProvider } from "@neokapi/ui-primitives";
import { AppShell } from "../components/AppShell";
import type { SidebarContext } from "../components/AppSidebar";
import type { ProjectInfo, Workspace } from "../types/api";

/**
 * What the sidebar must never do again: change what it means depending on where
 * you are. Opening a project used to replace the workspace rail with the
 * project's own sections plus an unlabelled back arrow, which put Context,
 * Insights and Settings out of reach for as long as a project was open, and
 * offered no way to say where "back" went.
 */

const workspace: Workspace = {
  id: "ws1",
  slug: "acme",
  name: "Acme",
} as Workspace;

const project = { id: "p1", name: "Docs site", default_stream: "main" } as ProjectInfo;

function projectContext(overrides: Partial<Extract<SidebarContext, { level: "project" }>> = {}) {
  return {
    level: "project",
    project,
    activeStream: "main",
    activeProjectView: "dashboard",
    onOpenDashboard: vi.fn(),
    onOpenSource: vi.fn(),
    onOpenFile: vi.fn(),
    onStreamChange: vi.fn(),
    onOpenAutomations: vi.fn(),
    onOpenRuns: vi.fn(),
    onOpenConnectors: vi.fn(),
    ...overrides,
  } as Extract<SidebarContext, { level: "project" }>;
}

function renderShell(props: Partial<React.ComponentProps<typeof AppShell>> = {}) {
  // Both hosts wrap the router in a TooltipProvider (their root layouts), and
  // the rail's tooltips need one.
  return render(
    <TooltipProvider>
      <AppShell
        workspaces={[workspace]}
        activeWorkspace={workspace}
        onSelectWorkspace={vi.fn()}
        activeView="translate"
        onViewChange={vi.fn()}
        onSubNavChange={vi.fn()}
        user={null}
        collapsed={false}
        onCollapsedChange={vi.fn()}
        {...props}
      >
        <div>content</div>
      </AppShell>
    </TooltipProvider>,
  );
}

describe("the rail inside a project", () => {
  it("still offers every workspace destination", () => {
    renderShell({ sidebarContext: projectContext() });
    for (const id of ["translate", "context", "insights", "settings"]) {
      expect(screen.getByTestId(`nav-${id}`)).toBeInTheDocument();
    }
  });

  it("offers no bare back arrow, because the trail is the way up", () => {
    renderShell({ sidebarContext: projectContext() });
    expect(screen.queryByTestId("sidebar-home")).not.toBeInTheDocument();
  });

  it("keeps Projects lit, because a project is inside it", () => {
    renderShell({ sidebarContext: projectContext() });
    expect(screen.getByTestId("nav-translate")).toHaveAttribute("data-active", "true");
  });
});

describe("the panel beside the rail", () => {
  it("holds the project's sections, under the project's name", () => {
    renderShell({ sidebarContext: projectContext() });
    expect(screen.getByText("Docs site")).toBeInTheDocument();
    for (const id of ["dashboard", "source", "automations", "runs", "connectors"]) {
      expect(screen.getByTestId(`subnav-${id}`)).toBeInTheDocument();
    }
  });

  it("omits a section the host does not offer", () => {
    renderShell({
      sidebarContext: projectContext({ onOpenRuns: undefined, onOpenConnectors: undefined }),
    });
    expect(screen.getByTestId("subnav-source")).toBeInTheDocument();
    expect(screen.queryByTestId("subnav-runs")).not.toBeInTheDocument();
    expect(screen.queryByTestId("subnav-connectors")).not.toBeInTheDocument();
  });

  it("opens the section it names", async () => {
    const onOpenSource = vi.fn();
    renderShell({ sidebarContext: projectContext({ onOpenSource }) });
    await userEvent.click(screen.getByTestId("subnav-source"));
    expect(onOpenSource).toHaveBeenCalledOnce();
  });

  /**
   * The panel used to stay mounted at zero width so it could animate shut,
   * which left the previous project's sections in the document after leaving
   * the project — still clickable, still announced, and still matching a query
   * for the project's name.
   */
  it("is gone from the document once a project is left", () => {
    const { container, rerender } = renderShell({ sidebarContext: projectContext() });
    expect(container.querySelector("[data-secondary-panel]")).toBeInTheDocument();
    expect(screen.getByTestId("subnav-source")).toBeInTheDocument();

    rerender(
      <TooltipProvider>
        <AppShell
          workspaces={[workspace]}
          activeWorkspace={workspace}
          onSelectWorkspace={vi.fn()}
          activeView="translate"
          onViewChange={vi.fn()}
          onSubNavChange={vi.fn()}
          user={null}
          collapsed={false}
          onCollapsedChange={vi.fn()}
          sidebarContext={{ level: "workspace", activeView: "translate" }}
        >
          <div>content</div>
        </AppShell>
      </TooltipProvider>,
    );

    expect(container.querySelector("[data-secondary-panel]")).not.toBeInTheDocument();
    expect(screen.queryByTestId("subnav-source")).not.toBeInTheDocument();
    expect(screen.queryByText("Docs site")).not.toBeInTheDocument();
  });

  it("falls back to the workspace view's own sub-nav when no project is open", () => {
    renderShell({ sidebarContext: { level: "workspace", activeView: "settings" } });
    expect(screen.getByTestId("subnav-members")).toBeInTheDocument();
  });
});

describe("the trail in the header", () => {
  it("names each step and walks back to the one clicked", async () => {
    const onProjects = vi.fn();
    renderShell({
      sidebarContext: projectContext(),
      breadcrumbs: [
        { label: "Acme", onClick: vi.fn() },
        { label: "Projects", onClick: onProjects, testId: "back-to-projects" },
        { label: "Docs site" },
      ],
    });
    const trail = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(trail).toHaveTextContent("Acme");
    expect(trail).toHaveTextContent("Projects");

    await userEvent.click(screen.getByTestId("back-to-projects"));
    expect(onProjects).toHaveBeenCalledOnce();
  });

  it("marks the last step as the page you are on, and gives it no action", () => {
    renderShell({
      breadcrumbs: [{ label: "Acme", onClick: vi.fn() }, { label: "Settings" }],
    });
    expect(screen.getByText("Settings")).toHaveAttribute("aria-current", "page");
    expect(screen.getByText("Settings").tagName).not.toBe("BUTTON");
  });
});
