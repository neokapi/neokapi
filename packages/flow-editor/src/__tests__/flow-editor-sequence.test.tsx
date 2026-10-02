// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { FlowEditor } from "../FlowEditor";
import type { FlowSpec, ToolInfo } from "../types";

// Flow steps run in order, one after another: the runtime refuses a step that
// holds a parallel: list (core/flow CheckSequential). The canvas editor creates
// no such step, and shows a loaded one as invalid with the same refusal.

beforeAll(() => {
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
});

afterEach(cleanup);

const TOOLS: ToolInfo[] = [
  {
    name: "translate",
    display_name: "Translate",
    description: "Translate.",
    category: "translation",
  },
  { name: "qa", display_name: "Quality Check", description: "Check.", category: "validate" },
  { name: "word-count", display_name: "Word Count", description: "Count.", category: "enrich" },
];

const GROUPED: FlowSpec = {
  steps: [{ tool: "translate" }, { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] }],
};

const REFUSAL =
  "step[1] holds a parallel: list (qa, word-count), and flow steps run in order, one after another: list those tools as ordered steps";

describe("FlowEditor and parallel groups", () => {
  it("shows a loaded group as invalid and does not run it", () => {
    render(<FlowEditor flow={GROUPED} tools={TOOLS} onChange={vi.fn()} run={{ onRun: vi.fn() }} />);
    expect(screen.getByTestId("flow-sequence-error")).toHaveTextContent(REFUSAL);
    expect(screen.getByTestId("parallel-group-error")).toHaveTextContent(REFUSAL);
    expect(screen.getByLabelText("Run flow")).toBeDisabled();
  });

  it("lists the group's tools as ordered steps", () => {
    const onChange = vi.fn();
    render(<FlowEditor flow={GROUPED} tools={TOOLS} onChange={onChange} />);
    fireEvent.click(screen.getByTestId("list-in-order"));
    expect(onChange).toHaveBeenCalledWith({
      steps: [{ tool: "translate" }, { tool: "qa" }, { tool: "word-count" }],
    });
  });

  it("offers no parallel route, branch or parallelize suggestion", () => {
    const sequential: FlowSpec = { steps: [{ tool: "qa" }, { tool: "word-count" }] };
    render(
      <FlowEditor flow={sequential} tools={TOOLS} onChange={vi.fn()} run={{ onRun: vi.fn() }} />,
    );
    // Two adjacent read-only tools were where the editor suggested a group.
    expect(screen.queryByText(/can run in\s+parallel/)).not.toBeInTheDocument();
    expect(screen.queryByText("Parallelize")).not.toBeInTheDocument();
    expect(screen.queryByTestId("flow-sequence-error")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Run flow")).not.toBeDisabled();

    fireEvent.click(screen.getByLabelText("Add tool"));
    expect(screen.getByText("Protect sensitive content")).toBeInTheDocument();
    expect(screen.queryByText("Run tools in parallel")).not.toBeInTheDocument();
  });

  it("draws no add-branch control on a loaded group", () => {
    render(<FlowEditor flow={GROUPED} tools={TOOLS} onChange={vi.fn()} />);
    expect(screen.queryByText("Add branch")).not.toBeInTheDocument();
  });
});
