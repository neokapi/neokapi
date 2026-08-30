import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { AppHome } from "../components/AppHome";

const defaultProps = {
  recentFiles: [] as Array<{ path: string; name: string; opened_at: string }>,
  samplesDismissed: false,
  onOpenRecent: vi.fn(),
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
      },
    ];
    render(<AppHome {...defaultProps} recentFiles={recentFiles} />);
    expect(screen.getByText("MyApp")).toBeInTheDocument();
    expect(screen.getByText("Recent Projects")).toBeInTheDocument();
  });
});
