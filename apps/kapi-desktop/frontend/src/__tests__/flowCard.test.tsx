import { render, screen, within } from "./testUtils";
import { describe, it, expect } from "vitest";
import type { FlowCardItem } from "../components/flows/FlowCard";
import { FlowCard } from "../components/flows/FlowCard";
import { FlowsEmptyState } from "../components/flows/FlowsEmptyState";

const shipFlow: FlowCardItem = {
  id: "ship",
  name: "Ship translations",
  description: "Translate with guardrails.",
  steps: ["Memory Reuse", "Translate", "Quality Check"],
  stepCount: 3,
  source: "project",
  isDefault: true,
};

describe("FlowCard", () => {
  it("leads with the outcome and shows the steps as chips, not a step count", () => {
    render(<FlowCard item={shipFlow} onClick={() => {}} />);
    expect(screen.getByText("Ship translations")).toBeInTheDocument();
    expect(screen.getByText("Translate with guardrails.")).toBeInTheDocument();

    const chips = screen.getByTestId("flow-steps");
    expect(within(chips).getByText("Memory Reuse")).toBeInTheDocument();
    expect(within(chips).getByText("Translate")).toBeInTheDocument();
    expect(within(chips).getByText("Quality Check")).toBeInTheDocument();

    // The bare "{n} step(s)" identity is gone.
    expect(screen.queryByText(/step\(s\)/)).not.toBeInTheDocument();
    expect(screen.queryByText(/3 steps/)).not.toBeInTheDocument();
  });

  it("marks the project's default flow", () => {
    render(<FlowCard item={shipFlow} onClick={() => {}} />);
    expect(screen.getByTestId("flow-default")).toHaveTextContent("Default");
  });

  it("does not mark a non-default flow", () => {
    render(<FlowCard item={{ ...shipFlow, isDefault: false }} onClick={() => {}} />);
    expect(screen.queryByTestId("flow-default")).not.toBeInTheDocument();
  });

  it("says so when a flow has no steps yet", () => {
    render(
      <FlowCard
        item={{ id: "e", name: "empty", steps: [], stepCount: 0, source: "user" }}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("No steps yet")).toBeInTheDocument();
    expect(screen.queryByTestId("flow-steps")).not.toBeInTheDocument();
  });
});

describe("FlowsEmptyState", () => {
  it("points a project with no flows at the default flow it already runs", () => {
    render(<FlowsEmptyState projectMode onCreate={() => {}} />);
    expect(screen.getByText("This project runs the default flow")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Create Flow/ })).toBeInTheDocument();
  });

  it("invites a first flow in ad-hoc mode", () => {
    render(<FlowsEmptyState projectMode={false} onCreate={() => {}} />);
    expect(screen.getByText("No flows yet")).toBeInTheDocument();
  });
});
