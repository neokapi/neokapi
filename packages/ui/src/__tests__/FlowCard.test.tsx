// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { FlowCardItem } from "../components/flows";
import { FlowCard, FlowsEmptyState } from "../components/flows";

const shipFlow: FlowCardItem = {
  id: "ship",
  name: "Ship translations",
  description: "Translate with guardrails.",
  steps: ["Memory Reuse", "Translate", "Quality Check"],
  stepCount: 3,
  source: "project",
  isDefault: true,
};

afterEach(cleanup);

describe("FlowCard", () => {
  it("leads with the outcome and shows the steps as chips, not a step count", () => {
    render(<FlowCard item={shipFlow} onClick={() => {}} />);
    expect(screen.getByText("Ship translations")).toBeTruthy();
    expect(screen.getByText("Translate with guardrails.")).toBeTruthy();

    const chips = screen.getByTestId("flow-steps");
    expect(within(chips).getByText("Memory Reuse")).toBeTruthy();
    expect(within(chips).getByText("Translate")).toBeTruthy();
    expect(within(chips).getByText("Quality Check")).toBeTruthy();

    // The bare "{n} step(s)" identity is gone.
    expect(screen.queryByText(/step\(s\)/)).toBeNull();
    expect(screen.queryByText(/3 steps/)).toBeNull();
  });

  it("marks the project's default flow", () => {
    render(<FlowCard item={shipFlow} onClick={() => {}} />);
    expect(screen.getByTestId("flow-default").textContent).toContain("Default");
  });

  it("does not mark a non-default flow", () => {
    render(<FlowCard item={{ ...shipFlow, isDefault: false }} onClick={() => {}} />);
    expect(screen.queryByTestId("flow-default")).toBeNull();
  });

  it("says so when a flow has no steps yet", () => {
    render(
      <FlowCard
        item={{ id: "e", name: "empty", steps: [], stepCount: 0, source: "user" }}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("No steps yet")).toBeTruthy();
    expect(screen.queryByTestId("flow-steps")).toBeNull();
  });

  it("shows a built-in badge for a built-in flow", () => {
    render(
      <FlowCard
        item={{ id: "b", name: "convert", steps: ["Word count"], stepCount: 1, source: "built-in" }}
        onClick={() => {}}
      />,
    );
    expect(screen.getByText("built-in")).toBeTruthy();
  });
});

describe("FlowsEmptyState", () => {
  it("points a project with no flows at the default flow it already runs", () => {
    render(<FlowsEmptyState projectMode onCreate={() => {}} />);
    expect(screen.getByText("This project runs the default flow")).toBeTruthy();
    expect(screen.getByRole("button", { name: /Create Flow/ })).toBeTruthy();
  });

  it("invites a first flow in ad-hoc mode", () => {
    render(<FlowsEmptyState projectMode={false} onCreate={() => {}} />);
    expect(screen.getByText("No flows yet")).toBeTruthy();
  });
});
