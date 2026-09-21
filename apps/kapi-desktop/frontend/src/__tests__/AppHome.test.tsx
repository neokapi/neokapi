import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { AppHome } from "../components/AppHome";
import type { WorkspaceHome, WorkspaceProject } from "../types/api";

const emptyWorkspace: WorkspaceHome = {
  location: "/fakehome/.local/share/kapi/workspaces/default",
  read_only: false,
  projects: [],
};

const defaultProps = {
  workspace: emptyWorkspace,
  samplesDismissed: false,
  onOpenCheckout: vi.fn(),
  onOpenContext: vi.fn(),
  onForgetProject: vi.fn(),
  onNewProject: vi.fn(),
  onOpenProject: vi.fn(),
  onNavigate: vi.fn(),
  onCreateSampleProject: vi.fn(),
  onDismissSamples: vi.fn(),
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

describe("AppHome", () => {
  it("shows sample project cards when not dismissed", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("KapiMart")).toBeInTheDocument();
    expect(screen.getByText(/New to Kapi/)).toBeInTheDocument();
  });

  it("hides sample project cards when dismissed", () => {
    render(<AppHome {...defaultProps} samplesDismissed={true} />);
    expect(screen.queryByText("KapiMart")).not.toBeInTheDocument();
    expect(screen.queryByText("OkapiMart")).not.toBeInTheDocument();
  });

  it("calls onCreateSampleProject with 'kapimart' when clicking KapiMart", async () => {
    const onCreateSampleProject = vi.fn();
    render(<AppHome {...defaultProps} onCreateSampleProject={onCreateSampleProject} />);
    await userEvent.click(screen.getByText("KapiMart"));
    expect(onCreateSampleProject).toHaveBeenCalledWith("kapimart");
  });

  it("calls onDismissSamples when clicking dismiss button", async () => {
    const onDismissSamples = vi.fn();
    render(<AppHome {...defaultProps} onDismissSamples={onDismissSamples} />);
    await userEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismissSamples).toHaveBeenCalled();
  });

  it("renders quick action buttons", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("New Project")).toBeInTheDocument();
    expect(screen.getByText("Open a Project")).toBeInTheDocument();
    expect(screen.getByText("Design a Flow")).toBeInTheDocument();
  });

  it("offers no quick-tool run door", () => {
    render(<AppHome {...defaultProps} />);
    // Tools are a reference, not a runner, so the home page does not send
    // anyone to the toolbox expecting to execute something there.
    expect(screen.queryByText("Run a Tool")).not.toBeInTheDocument();
  });

  it("leads with a project-first Projects section", () => {
    render(<AppHome {...defaultProps} />);
    const projects = screen.getByText("Projects");
    const quickTools = screen.getByText("Quick tools");
    expect(projects).toBeInTheDocument();
    expect(quickTools).toBeInTheDocument();
    // Project section comes before the secondary quick-tools group in the DOM.
    expect(
      projects.compareDocumentPosition(quickTools) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("labels the ad-hoc tools as secondary one-off quick tools", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("Quick tools")).toBeInTheDocument();
    expect(screen.getByText(/don't need a project/i)).toBeInTheDocument();
  });

  it("calls onNewProject when clicking New Project", async () => {
    const onNewProject = vi.fn();
    render(<AppHome {...defaultProps} onNewProject={onNewProject} />);
    await userEvent.click(screen.getByText("New Project"));
    expect(onNewProject).toHaveBeenCalled();
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
      render(<AppHome {...defaultProps} workspace={withProjects(project())} samplesDismissed />);
      expect(screen.getByText("Your Projects")).toBeInTheDocument();
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
      render(<AppHome {...defaultProps} workspace={withProjects(gone)} samplesDismissed />);
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
