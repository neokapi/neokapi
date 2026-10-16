import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { AppHome } from "../components/AppHome";
import {
  ONE_PROJECT_ROWS,
  SEVERAL_ROWS,
  WORKSPACE_RULES,
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

describe("AppHome", () => {
  it("opens on your context, with what it is", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByRole("heading", { level: 1, name: "Your context" })).toBeInTheDocument();
    expect(
      screen.getByText("What your projects write by, and what your assistants noticed."),
    ).toBeInTheDocument();
    expect(screen.getByText(/workspaces\/default/)).toBeInTheDocument();
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
    const quickTools = screen.getByText("Quick tools");
    expect(
      slot("start-here")!.compareDocumentPosition(quickTools) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("reads the standings off the workspace's rows", () => {
    render(<AppHome {...defaultProps} workspace={workspaceOf(ONE_PROJECT_ROWS)} />);
    expect(
      screen.getByText("kapi knows 23 rules across 1 project. 4 suggestions are advising."),
    ).toBeInTheDocument();
    expect(screen.getByText("claude-code is working in Acme Help Center now.")).toBeInTheDocument();
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

  it("says nothing is recorded on a first run", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("Nothing is recorded yet.")).toBeInTheDocument();
    expect(slot("whats-new")).toBeNull();
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

  it("offers no quick-tool run door", () => {
    render(<AppHome {...defaultProps} />);
    // Tools are a reference, not a runner, so the home page does not send
    // anyone to the toolbox expecting to execute something there.
    expect(screen.queryByText("Run a Tool")).not.toBeInTheDocument();
  });

  it("labels the ad-hoc tools as secondary one-off quick tools", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("Quick tools")).toBeInTheDocument();
    expect(screen.getByText(/don't need a project/i)).toBeInTheDocument();
  });

  it("navigates to flows from quick tools", async () => {
    const onNavigate = vi.fn();
    render(<AppHome {...defaultProps} onNavigate={onNavigate} />);
    await userEvent.click(screen.getByText("Design a Flow"));
    expect(onNavigate).toHaveBeenCalledWith("flows");
  });

  // The workspace list replaces the recent-files list: a project is here
  // because kapi has run in it, whichever surface ran.
  describe("the workspace list", () => {
    it("lists what the workspace holds, and where the workspace is", () => {
      render(<AppHome {...defaultProps} workspace={withProjects(project())} />);
      expect(screen.getByText("Projects")).toBeInTheDocument();
      expect(screen.getByText("KapiMart")).toBeInTheDocument();
      expect(screen.getByText("prj_kapimart")).toBeInTheDocument();
      expect(screen.getByText(/workspaces\/default/)).toBeInTheDocument();
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

    it("collapses several checkouts into one project and asks which", async () => {
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
      // No single "Open": the reader picks which checkout.
      expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
      const choices = screen.getAllByTestId("checkout-open");
      expect(choices).toHaveLength(2);
      await userEvent.click(choices[1]);
      expect(onOpenCheckout).toHaveBeenCalledWith("/fakehome/src/kapimart-fix/kapi.yaml");
    });

    it("shows a checkout that is gone as missing, keeping the project", () => {
      const gone = project({
        checkouts: [
          {
            path: "/fakehome/src/kapimart",
            recipe: "/fakehome/src/kapimart/kapi.yaml",
            missing: true,
          },
        ],
      });
      render(<AppHome {...defaultProps} workspace={withProjects(gone)} />);
      expect(screen.getByText("KapiMart")).toBeInTheDocument();
      expect(screen.getByTestId("checkout-missing")).toBeInTheDocument();
      expect(screen.getByText("Missing")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
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

    it("confirms a removal by naming what goes before it goes", async () => {
      const onForgetProject = vi.fn();
      render(
        <AppHome
          {...defaultProps}
          workspace={withProjects(project())}
          onForgetProject={onForgetProject}
        />,
      );
      await userEvent.click(screen.getByRole("button", { name: /Remove KapiMart/ }));
      expect(onForgetProject).not.toHaveBeenCalled();
      expect(screen.getByText(/Remove KapiMart from your workspace/)).toBeInTheDocument();
      expect(screen.getByText(/its content memory and every decision/)).toBeInTheDocument();
      expect(screen.getByText(/Your files stay exactly where they are/)).toBeInTheDocument();
      expect(screen.getAllByText("/fakehome/src/kapimart").length).toBeGreaterThan(1);

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
      await userEvent.click(screen.getByRole("button", { name: /Remove KapiMart/ }));
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
