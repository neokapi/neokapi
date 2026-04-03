import { render, screen } from "@testing-library/react";
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
    expect(screen.getByText("OkapiMart")).toBeInTheDocument();
    expect(screen.getByText(/New to Kapi/)).toBeInTheDocument();
  });

  it("shows sample project cards even with recent files", () => {
    const recentFiles = [
      { path: "/tmp/project.kapi", name: "Test", opened_at: "2026-03-01T00:00:00Z" },
    ];
    render(<AppHome {...defaultProps} recentFiles={recentFiles} />);
    expect(screen.getByText("KapiMart")).toBeInTheDocument();
    expect(screen.getByText("OkapiMart")).toBeInTheDocument();
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

  it("calls onCreateSampleProject with 'okapimart' when clicking OkapiMart", async () => {
    const onCreateSampleProject = vi.fn();
    render(<AppHome {...defaultProps} onCreateSampleProject={onCreateSampleProject} />);
    await userEvent.click(screen.getByText("OkapiMart"));
    expect(onCreateSampleProject).toHaveBeenCalledWith("okapimart");
  });

  it("calls onDismissSamples when clicking dismiss button", async () => {
    const onDismissSamples = vi.fn();
    render(<AppHome {...defaultProps} onDismissSamples={onDismissSamples} />);
    await userEvent.click(screen.getByTitle("Dismiss"));
    expect(onDismissSamples).toHaveBeenCalled();
  });

  it("renders quick action buttons", () => {
    render(<AppHome {...defaultProps} />);
    expect(screen.getByText("New Project")).toBeInTheDocument();
    expect(screen.getByText("Open a Project")).toBeInTheDocument();
    expect(screen.getByText("Design a Flow")).toBeInTheDocument();
    expect(screen.getByText("Run a Tool")).toBeInTheDocument();
  });

  it("renders recent projects when present", () => {
    const recentFiles = [
      {
        path: "/home/dev/KapiProjects/MyApp/project.kapi",
        name: "MyApp",
        opened_at: "2026-03-01T00:00:00Z",
      },
    ];
    render(<AppHome {...defaultProps} recentFiles={recentFiles} />);
    expect(screen.getByText("MyApp")).toBeInTheDocument();
    expect(screen.getByText("Recent Projects")).toBeInTheDocument();
  });
});
