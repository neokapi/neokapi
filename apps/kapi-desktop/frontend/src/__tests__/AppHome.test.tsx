import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { AppHome } from "../components/AppHome";
import {
  ONE_PROJECT_ROWS,
  SEVERAL_ROWS,
  WORKSPACE_RULES,
  standing,
  workspaceOf,
} from "../stories/fixtures/standings";
import type { WorkspaceHome, WorkspaceProject } from "../types/api";

const emptyWorkspace: WorkspaceHome = {
  location: "/fakehome/.local/share/kapi/workspaces/default",
  read_only: false,
  projects: [],
};

const defaultProps = {
  workspace: emptyWorkspace,
  workspaceRules: [],
  startHereDismissed: false,
  hasOpenProject: false,
  onDismissStartHere: vi.fn(),
  onConnectAssistant: vi.fn(),
  onOpenWhatsNew: vi.fn(),
  onOpenCheckout: vi.fn(),
  onOpenContext: vi.fn(),
  onForgetProject: vi.fn(),
  onNewProject: vi.fn(),
  onOpenProject: vi.fn(),
  onNavigate: vi.fn(),
  onCreateSampleProject: vi.fn(),
};

/** One project with one live checkout. */
function project(overrides: Partial<WorkspaceProject> = {}): WorkspaceProject {
  return {
    key: "prj_kapimart",
    name: "KapiMart",
    last_active: "2026-09-20T09:00:00Z",
    checkouts: [
      {
        path: "/fakehome/src/kapimart",
        recipe: "/fakehome/src/kapimart/kapi.yaml",
        missing: false,
      },
    ],
    ...overrides,
  };
}

function withProjects(...projects: WorkspaceProject[]): WorkspaceHome {
  return { ...emptyWorkspace, projects };
}

const slot = (name: string) => document.querySelector(`[data-slot='${name}']`);
const slots = (name: string) => Array.from(document.querySelectorAll(`[data-slot='${name}']`));

describe("AppHome", () => {
  it("opens on your context, with what it is and where the workspace is", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByRole("heading", { level: 1, name: "Your context" })).toBeInTheDocument();
    expect(
      screen.getByText("What your projects write by, and what your assistants noticed."),
    ).toBeInTheDocument();
    // The location is a value: its last segment, the whole path in the tooltip.
    expect(slot("workspace-location")?.textContent).toBe("default");
    expect(screen.queryByText(/workspaces\/default/)).not.toBeInTheDocument();
  });

  it("composes the sections in the order a reader meets them", () => {
    render(
      <AppHome
        {...defaultProps}
        workspace={workspaceOf(SEVERAL_ROWS)}
        workspaceRules={WORKSPACE_RULES}
      />,
    );
    const order = [
      "workspace-numbers",
      "whats-new",
      "home-projects",
      "workspace-rules",
      "start-here",
    ].map((name) => slot(name));
    expect(order.every(Boolean)).toBe(true);
    for (let i = 1; i < order.length; i++) {
      expect(
        order[i - 1]!.compareDocumentPosition(order[i]!) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
  });

  it("carries no quick tools: the Toolbox is in the sidebar", () => {
    render(<AppHome {...defaultProps} workspace={workspaceOf(SEVERAL_ROWS)} />);
    expect(screen.queryByText("Quick tools")).not.toBeInTheDocument();
    expect(screen.queryByText("Design a Flow")).not.toBeInTheDocument();
    expect(screen.queryByText("Run a Tool")).not.toBeInTheDocument();
  });

  it("draws Holds in every project only when the workspace holds a rule", () => {
    const { rerender } = render(
      <AppHome {...defaultProps} workspace={workspaceOf(SEVERAL_ROWS)} workspaceRules={[]} />,
    );
    expect(slot("workspace-rules")).toBeNull();
    expect(screen.queryByText("Holds in every project")).not.toBeInTheDocument();
    expect(screen.queryByText(/apply more widely/)).not.toBeInTheDocument();
    rerender(
      <AppHome
        {...defaultProps}
        workspace={workspaceOf(SEVERAL_ROWS)}
        workspaceRules={WORKSPACE_RULES}
      />,
    );
    expect(screen.getByText("Holds in every project")).toBeInTheDocument();
  });

  it("reads the counts off the workspace's rows", () => {
    render(<AppHome {...defaultProps} workspace={workspaceOf(ONE_PROJECT_ROWS)} />);
    const phrases = Array.from(
      slot("workspace-numbers")!.querySelectorAll("[data-slot='stat'] .sr-only"),
    ).map((s) => s.textContent);
    expect(phrases).toEqual(["1 project", "23 rules", "142 terms", "4 suggestions", "0 need you"]);
  });

  it("shows the assistant working now on its project's row, as a chip", () => {
    render(<AppHome {...defaultProps} workspace={workspaceOf(ONE_PROJECT_ROWS)} />);
    const working = slots("workspace-working");
    expect(working.map((w) => w.textContent)).toEqual(["claude-code is working"]);
    expect(working[0].closest("[data-slot='whats-new-row']")).not.toBeNull();
    expect(screen.queryByText(/is working in/)).not.toBeInTheDocument();
  });

  it("opens a project on its What's new from the row", async () => {
    const onOpenWhatsNew = vi.fn();
    render(
      <AppHome
        {...defaultProps}
        workspace={workspaceOf(ONE_PROJECT_ROWS)}
        onOpenWhatsNew={onOpenWhatsNew}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(onOpenWhatsNew).toHaveBeenCalledWith("prj_acmehelp");
  });

  it("leaves Start here to carry a first run, without counts or What's new", () => {
    render(<AppHome {...defaultProps} />);
    expect(slot("workspace-numbers")).toBeNull();
    expect(slot("whats-new")).toBeNull();
    const start = slot("start-here")!;
    expect(
      start.compareDocumentPosition(slot("home-projects")!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("says in one line that nothing is recorded in a project that holds nothing", () => {
    render(
      <AppHome
        {...defaultProps}
        workspace={withProjects(
          project({ context: standing({ project: "prj_kapimart", project_name: "KapiMart" }) }),
        )}
      />,
    );
    expect(slot("whats-new-empty")?.textContent).toMatch(/^Nothing is recorded yet\./);
  });

  it("no longer carries the raw feed", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.queryByText("Recorded")).not.toBeInTheDocument();
    expect(slot("context-feed-panel")).toBeNull();
  });

  it("opens the workspace's history from beside the title", async () => {
    const onNavigate = vi.fn();
    render(<AppHome {...defaultProps} onNavigate={onNavigate} />);
    await userEvent.click(screen.getByRole("button", { name: "History" }));
    expect(onNavigate).toHaveBeenCalledWith("history");
  });

  it("marks a workspace open for reading only in the header", () => {
    render(<AppHome {...defaultProps} workspace={workspaceOf(ONE_PROJECT_ROWS, true)} />);
    expect(slot("workspace-read-only")).toHaveTextContent("Read only");
  });

  it("shows Start here until it is dismissed", async () => {
    const onDismissStartHere = vi.fn();
    const { rerender } = render(
      <AppHome {...defaultProps} onDismissStartHere={onDismissStartHere} />,
    );
    expect(screen.getByText("Start here")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Dismiss Start here" }));
    expect(onDismissStartHere).toHaveBeenCalled();
    rerender(<AppHome {...defaultProps} startHereDismissed />);
    expect(screen.queryByText("Start here")).not.toBeInTheDocument();
  });

  it("creates the sample from Start here", async () => {
    const onCreateSampleProject = vi.fn();
    render(<AppHome {...defaultProps} onCreateSampleProject={onCreateSampleProject} />);
    await userEvent.click(screen.getByTestId("sample-kapimart"));
    expect(onCreateSampleProject).toHaveBeenCalledWith("kapimart");
  });

  it("starts and opens projects from small buttons beside the heading", async () => {
    const onNewProject = vi.fn();
    const onOpenProject = vi.fn();
    render(<AppHome {...defaultProps} onNewProject={onNewProject} onOpenProject={onOpenProject} />);
    await userEvent.click(screen.getByRole("button", { name: "New project" }));
    await userEvent.click(screen.getByRole("button", { name: "Open a folder" }));
    expect(onNewProject).toHaveBeenCalled();
    expect(onOpenProject).toHaveBeenCalled();
  });

  // The workspace list replaces the recent-files list: a project is here
  // because kapi has run in it, whichever surface ran.
  describe("the workspace list", () => {
    it("lists what the workspace holds, one row a project, without its key", () => {
      render(<AppHome {...defaultProps} workspace={workspaceOf(SEVERAL_ROWS)} />);
      expect(screen.getByText("Projects")).toBeInTheDocument();
      expect(screen.getAllByTestId("workspace-project")).toHaveLength(4);
      expect(screen.queryByText("prj_kapimart")).not.toBeInTheDocument();
    });

    it("says so when nothing has registered yet", () => {
      render(<AppHome {...defaultProps} />);
      expect(screen.getByText(/Nothing here yet/)).toBeInTheDocument();
    });

    it("opens a project with one checkout straight from its row", async () => {
      const onOpenCheckout = vi.fn();
      render(
        <AppHome
          {...defaultProps}
          workspace={withProjects(project())}
          onOpenCheckout={onOpenCheckout}
        />,
      );
      await userEvent.click(screen.getByRole("button", { name: "Open" }));
      expect(onOpenCheckout).toHaveBeenCalledWith("/fakehome/src/kapimart/kapi.yaml");
    });

    it("collapses several checkouts into one project, with the others in its menu", async () => {
      const onOpenCheckout = vi.fn();
      const worktrees = project({
        checkouts: [
          {
            path: "/fakehome/src/kapimart",
            recipe: "/fakehome/src/kapimart/kapi.yaml",
            missing: false,
          },
          {
            path: "/fakehome/src/kapimart-fix",
            recipe: "/fakehome/src/kapimart-fix/kapi.yaml",
            missing: false,
          },
        ],
      });
      render(
        <AppHome
          {...defaultProps}
          workspace={withProjects(worktrees)}
          onOpenCheckout={onOpenCheckout}
        />,
      );
      expect(screen.getAllByTestId("workspace-project")).toHaveLength(1);
      await userEvent.click(screen.getByRole("button", { name: "More for KapiMart" }));
      await userEvent.click(screen.getByTestId("checkout-open"));
      expect(onOpenCheckout).toHaveBeenCalledWith("/fakehome/src/kapimart-fix/kapi.yaml");
    });

    it("keeps a project whose checkout is gone, and opens it on its context", async () => {
      const onOpenContext = vi.fn();
      const gone = project({
        checkouts: [
          {
            path: "/fakehome/src/kapimart",
            recipe: "/fakehome/src/kapimart/kapi.yaml",
            missing: true,
          },
        ],
      });
      render(
        <AppHome {...defaultProps} workspace={withProjects(gone)} onOpenContext={onOpenContext} />,
      );
      expect(screen.getByText("KapiMart")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
      await userEvent.click(screen.getByRole("button", { name: /Open context/ }));
      expect(onOpenContext).toHaveBeenCalledWith("prj_kapimart");
    });

    it("opens the context of a project with no checkout here", async () => {
      const onOpenContext = vi.fn();
      render(
        <AppHome
          {...defaultProps}
          workspace={withProjects(project({ checkouts: [] }))}
          onOpenContext={onOpenContext}
        />,
      );
      await userEvent.click(screen.getByRole("button", { name: /Open context/ }));
      expect(onOpenContext).toHaveBeenCalledWith("prj_kapimart");
    });

    it("confirms a removal from the row's menu by naming what goes before it goes", async () => {
      const onForgetProject = vi.fn();
      render(
        <AppHome
          {...defaultProps}
          workspace={withProjects(project())}
          onForgetProject={onForgetProject}
        />,
      );
      await userEvent.click(screen.getByRole("button", { name: "More for KapiMart" }));
      await userEvent.click(screen.getByRole("menuitem", { name: "Remove" }));
      expect(onForgetProject).not.toHaveBeenCalled();
      expect(screen.getByText(/Remove KapiMart from your workspace/)).toBeInTheDocument();
      expect(screen.getByText(/its content memory and every decision/)).toBeInTheDocument();
      expect(screen.getByText(/Your files stay exactly where they are/)).toBeInTheDocument();
      expect(screen.getByText("/fakehome/src/kapimart")).toBeInTheDocument();

      await userEvent.click(screen.getByRole("button", { name: /Remove this project/ }));
      expect(onForgetProject).toHaveBeenCalledWith("prj_kapimart");
    });

    it("cancels a removal without touching anything", async () => {
      const onForgetProject = vi.fn();
      render(
        <AppHome
          {...defaultProps}
          workspace={withProjects(project())}
          onForgetProject={onForgetProject}
        />,
      );
      await userEvent.click(screen.getByRole("button", { name: "More for KapiMart" }));
      await userEvent.click(screen.getByRole("menuitem", { name: "Remove" }));
      await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(onForgetProject).not.toHaveBeenCalled();
    });

    it("reports a workspace it could not read", () => {
      render(
        <AppHome
          {...defaultProps}
          workspace={null}
          workspaceError={new Error("the workspace directory is not writable")}
        />,
      );
      expect(screen.getByText(/could not be read/)).toBeInTheDocument();
    });
  });
});
