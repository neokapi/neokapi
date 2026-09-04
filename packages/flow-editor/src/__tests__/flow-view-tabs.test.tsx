// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { FlowViewTabs } from "../FlowViewTabs";
import type { FlowSpec, ToolInfo } from "../types";
import type { FlowTrace } from "../traceTypes";

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
    description: "Translate with an AI provider.",
    category: "translation",
    has_schema: false,
    produces: [{ type: "target", side: "target" }],
  },
  {
    name: "qa",
    display_name: "Quality Check",
    description: "Check the result.",
    category: "quality",
    has_schema: false,
    consumes: [{ type: "target", side: "target" }],
  },
];

const FLOW: FlowSpec = { steps: [{ tool: "translate" }, { tool: "qa" }] };

const TRACE: FlowTrace = {
  name: "check",
  nodes: [
    { id: "tool-1", type: "tool", name: "translate" },
    { id: "tool-2", type: "tool", name: "qa" },
  ],
  events: [
    { ts: 1, type: "enter", nodeId: "tool-1", partId: "b1" },
    { ts: 2, type: "exit", nodeId: "tool-1", partId: "b1" },
  ],
  parts: {
    b1: {
      initial: { id: "b1", type: "Block", summary: "Hello", sourceText: "Hello" },
      afterNode: {},
    },
  },
  durationUs: 2,
};

function renderTabs(props: Partial<React.ComponentProps<typeof FlowViewTabs>> = {}) {
  const onChange = vi.fn();
  const utils = render(
    <FlowViewTabs
      flowName="check"
      flow={FLOW}
      tools={TOOLS}
      onChange={onChange}
      onRun={vi.fn()}
      {...props}
    />,
  );
  return { ...utils, onChange };
}

const pressed = (testId: string) =>
  screen.getByTestId(testId).getAttribute("aria-pressed") === "true";

describe("FlowViewTabs", () => {
  it("opens on the step editor with a Steps / Diagram switch and no Run view", () => {
    renderTabs();
    expect(screen.getByTestId("linear-flow-editor")).toBeInTheDocument();
    expect(pressed("flow-view-steps")).toBe(true);
    expect(screen.getByTestId("flow-view-diagram")).toBeInTheDocument();
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
  });

  it("switches to the read-only diagram and back without touching the flow", async () => {
    const { onChange } = renderTabs();
    await userEvent.click(screen.getByTestId("flow-view-diagram"));
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
    expect(screen.queryByTestId("linear-flow-editor")).not.toBeInTheDocument();
    expect(screen.getByText("Translate")).toBeInTheDocument();
    expect(screen.queryByLabelText("Add tool")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Remove tool")).not.toBeInTheDocument();

    await userEvent.click(screen.getByTestId("flow-view-steps"));
    expect(screen.getByTestId("linear-flow-editor")).toBeInTheDocument();
    expect(screen.getAllByTestId("step-row")).toHaveLength(2);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows the host's run controls only while the Run view is active", async () => {
    renderTabs({ trace: TRACE, runControls: <span>file picker</span> });
    // A run present from the start leaves the chosen view alone.
    expect(pressed("flow-view-steps")).toBe(true);
    expect(screen.queryByTestId("flow-view-run-controls")).not.toBeInTheDocument();

    await userEvent.click(screen.getByTestId("flow-view-run"));
    expect(screen.getByTestId("flow-view-run-controls")).toHaveTextContent("file picker");

    await userEvent.click(screen.getByTestId("flow-view-diagram"));
    expect(screen.queryByTestId("flow-view-run-controls")).not.toBeInTheDocument();
  });

  it("offers the Run view only while a run is loaded, and switches to it when one arrives", () => {
    const { rerender } = renderTabs();
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();

    rerender(
      <FlowViewTabs flowName="check" flow={FLOW} tools={TOOLS} onChange={vi.fn()} trace={TRACE} />,
    );
    expect(screen.getByTestId("flow-view-run")).toBeInTheDocument();
    expect(pressed("flow-view-run")).toBe(true);
    expect(screen.getByLabelText("Playback position")).toBeInTheDocument();

    rerender(<FlowViewTabs flowName="check" flow={FLOW} tools={TOOLS} onChange={vi.fn()} />);
    // The run is gone: the view falls back to the diagram.
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
    expect(pressed("flow-view-diagram")).toBe(true);
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
  });

  it("keeps the chosen view when a run is present from the start", () => {
    renderTabs({ trace: TRACE });
    expect(screen.getByTestId("flow-view-run")).toBeInTheDocument();
    expect(pressed("flow-view-steps")).toBe(true);
    expect(screen.getByTestId("linear-flow-editor")).toBeInTheDocument();
  });

  it("follows a controlled view and reports clicks", async () => {
    const onViewChange = vi.fn();
    renderTabs({ view: "diagram", onViewChange });
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("flow-view-steps"));
    expect(onViewChange).toHaveBeenCalledWith("steps");
    // Still the diagram: the host owns the view.
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
  });

  it("passes the read-only flag through to the step editor", () => {
    renderTabs({ readOnly: true });
    expect(screen.queryByTestId("add-step")).not.toBeInTheDocument();
  });
});
