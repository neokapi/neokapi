import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { RunnerPage } from "../components/RunnerPage";
import { JobFeedProvider } from "../context/JobFeedContext";

function renderWithProviders(ui: React.ReactElement) {
  return render(<JobFeedProvider>{ui}</JobFeedProvider>);
}

describe("RunnerPage", () => {
  const defaultProps = {
    tabID: "test-tab",
    flowName: "translate",
    flow: {
      steps: [{ tool: "translate" }, { tool: "qa" }],
    },
    onClose: vi.fn(),
  };

  it("displays flow name", () => {
    renderWithProviders(<RunnerPage {...defaultProps} />);
    expect(screen.getByText("Run: translate")).toBeInTheDocument();
  });

  it("shows pipeline steps", () => {
    renderWithProviders(<RunnerPage {...defaultProps} />);
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.getByText("qa")).toBeInTheDocument();
  });

  it("shows input file selector", () => {
    renderWithProviders(<RunnerPage {...defaultProps} />);
    expect(screen.getByText("Select files...")).toBeInTheDocument();
  });

  it("shows target language input", () => {
    renderWithProviders(<RunnerPage {...defaultProps} />);
    expect(screen.getByPlaceholderText("e.g. fr-FR")).toBeInTheDocument();
  });

  it("has disabled Run button when no target lang", () => {
    renderWithProviders(<RunnerPage {...defaultProps} />);
    const runButton = screen.getByText("Run Flow");
    expect(runButton).toBeDisabled();
  });

  it("has Back button", () => {
    const onClose = vi.fn();
    renderWithProviders(<RunnerPage {...defaultProps} onClose={onClose} />);
    expect(screen.getByText("Back")).toBeInTheDocument();
  });

  // Regression: navigating back to a running flow remounts RunnerPage. autoRun
  // must fire once per run request (parent gates it via autoRun), never per
  // mount — otherwise each remount relaunches and duplicates the job.
  const autoRunProject = {
    version: "v1",
    name: "Demo",
    defaults: { target_languages: ["fr-FR"] },
  };

  it("fires onLaunched once when auto-running", () => {
    const onLaunched = vi.fn();
    renderWithProviders(
      <RunnerPage {...defaultProps} project={autoRunProject} autoRun onLaunched={onLaunched} />,
    );
    expect(onLaunched).toHaveBeenCalledTimes(1);
  });

  it("does not auto-run when autoRun is off (run already consumed by the parent)", () => {
    const onLaunched = vi.fn();
    renderWithProviders(
      <RunnerPage
        {...defaultProps}
        project={autoRunProject}
        autoRun={false}
        onLaunched={onLaunched}
      />,
    );
    expect(onLaunched).not.toHaveBeenCalled();
  });

  it("pre-populates target language from project defaults (manual path)", () => {
    const project = {
      version: "v1",
      name: "Demo",
      defaults: { target_languages: ["fr-FR", "de-DE"] },
    };
    renderWithProviders(<RunnerPage {...defaultProps} project={project} />);
    // With project targets, the free-text input is replaced by a language select
    // pre-set to the first target.
    expect(screen.queryByPlaceholderText("e.g. fr-FR")).not.toBeInTheDocument();
    const select = screen.getByLabelText("Target language");
    expect(select).toHaveTextContent("fr-FR");
  });
});
