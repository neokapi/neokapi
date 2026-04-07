import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { RunnerPage } from "../components/RunnerPage";

describe("RunnerPage", () => {
  const defaultProps = {
    tabID: "test-tab",
    flowName: "translate",
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    onClose: vi.fn(),
  };

  it("displays flow name", () => {
    render(<RunnerPage {...defaultProps} />);
    expect(screen.getByText("Run: translate")).toBeInTheDocument();
  });

  it("shows pipeline steps", () => {
    render(<RunnerPage {...defaultProps} />);
    expect(screen.getByText("ai-translate")).toBeInTheDocument();
    expect(screen.getByText("qa-check")).toBeInTheDocument();
  });

  it("shows input file selector", () => {
    render(<RunnerPage {...defaultProps} />);
    expect(screen.getByText("Select files...")).toBeInTheDocument();
  });

  it("shows target language input", () => {
    render(<RunnerPage {...defaultProps} />);
    expect(screen.getByPlaceholderText("e.g. fr-FR")).toBeInTheDocument();
  });

  it("has disabled Run button when no target lang", () => {
    render(<RunnerPage {...defaultProps} />);
    const runButton = screen.getByText("Run Flow");
    expect(runButton).toBeDisabled();
  });

  it("has Back button", () => {
    const onClose = vi.fn();
    render(<RunnerPage {...defaultProps} onClose={onClose} />);
    expect(screen.getByText("Back")).toBeInTheDocument();
  });
});
