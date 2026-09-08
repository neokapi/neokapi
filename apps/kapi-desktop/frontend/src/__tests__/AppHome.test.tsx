import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { AppHome } from "../components/AppHome";
import type { RecentFile } from "../types/api";

const defaultProps = {
  recentFiles: [] as RecentFile[],
  samplesDismissed: false,
  onOpenRecent: vi.fn(),
  onRemoveRecent: vi.fn(),
  onNewProject: vi.fn(),
  onOpenProject: vi.fn(),
  onNavigate: vi.fn(),
  onCreateSampleProject: vi.fn(),
  onDismissSamples: vi.fn(),
};

describe("AppHome", () => {
  it("shows sample project cards when not dismissed", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("KapiMart")).toBeInTheDocument();
    expect(screen.getByText(/New to Kapi/)).toBeInTheDocument();
  });

  it("shows sample project cards even with recent files", () => {
    const recentFiles = [
      {
        path: "/tmp/project/kapi.yaml",
        name: "Test",
        opened_at: "2026-03-01T00:00:00Z",
        available: true,
      },
    ];
    render(<AppHome {...defaultProps} recentFiles={recentFiles} />);
    expect(screen.getByText("KapiMart")).toBeInTheDocument();
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
    expect(screen.getByText("Projects")).toBeInTheDocument();
    // Project actions and quick tools are both present.
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

  it("renders recent projects when present", () => {
    const recentFiles = [
      {
        path: "/home/dev/KapiProjects/MyApp/kapi.yaml",
        name: "MyApp",
        opened_at: "2026-03-01T00:00:00Z",
        available: true,
      },
    ];
    render(<AppHome {...defaultProps} recentFiles={recentFiles} />);
    expect(screen.getByText("MyApp")).toBeInTheDocument();
    expect(screen.getByText("Recent Projects")).toBeInTheDocument();
  });
  // #2560: a remembered project whose recipe went away is shown for what it is
  // and never opened, so nothing downstream reads a recipe that is not there.
  describe("a project whose recipe is gone", () => {
    const gone: RecentFile = {
      path: "/home/dev/KapiProjects/KapiMart/kapi.yaml",
      name: "KapiMart Project",
      opened_at: "2026-03-01T00:00:00Z",
      available: false,
      unavailable: "moved",
    };

    it("renders it as unavailable with the folder-gone reason", () => {
      render(<AppHome {...defaultProps} recentFiles={[gone]} />);
      expect(screen.getByText("KapiMart Project")).toBeInTheDocument();
      expect(screen.getByText("Unavailable")).toBeInTheDocument();
      expect(screen.getByText(/moved or deleted/i)).toBeInTheDocument();
      expect(screen.getByText("/home/dev/KapiProjects/KapiMart")).toBeInTheDocument();
    });

    it("gives the missing-recipe reason when the folder survived", () => {
      render(<AppHome {...defaultProps} recentFiles={[{ ...gone, unavailable: "deleted" }]} />);
      expect(screen.getByText("The recipe is missing from this folder.")).toBeInTheDocument();
    });

    it("does not open it", async () => {
      const onOpenRecent = vi.fn();
      render(<AppHome {...defaultProps} recentFiles={[gone]} onOpenRecent={onOpenRecent} />);
      await userEvent.click(screen.getByTestId("recent-unavailable"));
      expect(onOpenRecent).not.toHaveBeenCalled();
    });

    it("removes it on request", async () => {
      const onRemoveRecent = vi.fn();
      const onOpenRecent = vi.fn();
      render(
        <AppHome
          {...defaultProps}
          recentFiles={[gone]}
          onOpenRecent={onOpenRecent}
          onRemoveRecent={onRemoveRecent}
        />,
      );
      await userEvent.click(screen.getByRole("button", { name: "Remove" }));
      expect(onRemoveRecent).toHaveBeenCalledWith(gone.path);
      expect(onOpenRecent).not.toHaveBeenCalled();
    });

    it("still opens the projects beside it", async () => {
      const onOpenRecent = vi.fn();
      const alive: RecentFile = {
        path: "/home/dev/KapiProjects/MyApp/kapi.yaml",
        name: "MyApp",
        opened_at: "2026-03-02T00:00:00Z",
        available: true,
      };
      render(<AppHome {...defaultProps} recentFiles={[gone, alive]} onOpenRecent={onOpenRecent} />);
      await userEvent.click(screen.getByText("MyApp"));
      expect(onOpenRecent).toHaveBeenCalledWith(alive.path);
    });
  });
});
