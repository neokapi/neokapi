import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { PipelineProgress, StepBadge, deriveStepState } from "../components/PipelineProgress";
import type { StepSnapshot } from "../context/JobFeedContext";

// ---------------------------------------------------------------------------
// deriveStepState
// ---------------------------------------------------------------------------

describe("deriveStepState", () => {
  it("returns pending when idle", () => {
    expect(deriveStepState(undefined, "idle")).toBe("pending");
    expect(deriveStepState({ name: "x", parts_in: 50, parts_out: 50 }, "idle")).toBe("pending");
  });

  it("returns done when complete/error/canceled", () => {
    const snap: StepSnapshot = { name: "x", parts_in: 10, parts_out: 5 };
    expect(deriveStepState(snap, "complete")).toBe("done");
    expect(deriveStepState(snap, "error")).toBe("done");
    expect(deriveStepState(snap, "canceled")).toBe("done");
  });

  it("returns pending when running but no snapshot", () => {
    expect(deriveStepState(undefined, "running")).toBe("pending");
  });

  it("returns pending when running with zero parts_in", () => {
    expect(deriveStepState({ name: "x", parts_in: 0, parts_out: 0 }, "running")).toBe("pending");
  });

  it("returns active when parts_in > parts_out", () => {
    expect(deriveStepState({ name: "x", parts_in: 50, parts_out: 30 }, "running")).toBe("active");
  });

  it("returns done when parts_in === parts_out and both > 0", () => {
    expect(deriveStepState({ name: "x", parts_in: 50, parts_out: 50 }, "running")).toBe("done");
  });
});

// ---------------------------------------------------------------------------
// StepBadge
// ---------------------------------------------------------------------------

describe("StepBadge", () => {
  it("renders the tool name", () => {
    render(<StepBadge name="translate" state="pending" />);
    expect(screen.getByText("translate")).toBeInTheDocument();
  });

  it("shows spinner when active", () => {
    const { container } = render(<StepBadge name="translate" state="active" />);
    expect(container.querySelector(".animate-spin")).not.toBeNull();
  });

  it("does not show spinner when pending", () => {
    const { container } = render(<StepBadge name="translate" state="pending" />);
    expect(container.querySelector(".animate-spin")).toBeNull();
  });

  it("does not show spinner when done", () => {
    const { container } = render(<StepBadge name="translate" state="done" />);
    expect(container.querySelector(".animate-spin")).toBeNull();
  });

  it("shows part counts when snapshot has parts_in > 0", () => {
    render(
      <StepBadge
        name="qa"
        state="active"
        snapshot={{ name: "qa", parts_in: 120, parts_out: 87 }}
      />,
    );
    expect(screen.getByText("87/120")).toBeInTheDocument();
  });

  it("does not show counts when parts_in is 0", () => {
    render(
      <StepBadge name="qa" state="pending" snapshot={{ name: "qa", parts_in: 0, parts_out: 0 }} />,
    );
    expect(screen.queryByText("0/0")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// PipelineProgress
// ---------------------------------------------------------------------------

describe("PipelineProgress", () => {
  const steps = [{ tool: "translate" }, { tool: "qa" }, { tool: "term-enforce" }];

  it("renders all step names", () => {
    render(<PipelineProgress steps={steps} />);
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.getByText("qa")).toBeInTheDocument();
    expect(screen.getByText("term-enforce")).toBeInTheDocument();
  });

  it("renders arrow separators between steps", () => {
    render(<PipelineProgress steps={steps} />);
    const arrows = screen.getAllByText("→");
    expect(arrows).toHaveLength(2);
  });

  it("shows no spinners when idle", () => {
    const { container } = render(<PipelineProgress steps={steps} runState="idle" />);
    expect(container.querySelector(".animate-spin")).toBeNull();
  });

  it("shows spinner on active step during running", () => {
    const { container } = render(
      <PipelineProgress
        steps={steps}
        runState="running"
        snapshots={[
          { name: "translate", parts_in: 50, parts_out: 30 },
          { name: "qa", parts_in: 0, parts_out: 0 },
          { name: "term-enforce", parts_in: 0, parts_out: 0 },
        ]}
      />,
    );
    const spinners = container.querySelectorAll(".animate-spin");
    expect(spinners).toHaveLength(1);
  });

  it("shows no spinners when complete", () => {
    const { container } = render(
      <PipelineProgress
        steps={steps}
        runState="complete"
        snapshots={[
          { name: "translate", parts_in: 100, parts_out: 100 },
          { name: "qa", parts_in: 100, parts_out: 100 },
          { name: "term-enforce", parts_in: 100, parts_out: 100 },
        ]}
      />,
    );
    expect(container.querySelector(".animate-spin")).toBeNull();
  });

  it("renders single step without separator", () => {
    render(<PipelineProgress steps={[{ tool: "pseudo" }]} />);
    expect(screen.getByText("pseudo")).toBeInTheDocument();
    expect(screen.queryByText("→")).not.toBeInTheDocument();
  });
});
