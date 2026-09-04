// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it } from "vitest";
import { FlowDiagramView } from "../FlowDiagramView";
import type { ToolInfo } from "../types";
import type { FlowTrace } from "../traceTypes";

// React Flow measures its container and nodes through ResizeObserver, which
// jsdom lacks.
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
    has_schema: true,
    produces: [{ type: "target", side: "target" }],
  },
  {
    name: "qa",
    display_name: "Quality Check",
    description: "Check the result.",
    category: "quality",
    consumes: [{ type: "target", side: "target" }],
  },
  {
    name: "word-count",
    display_name: "Word Count",
    description: "Count words.",
    category: "analysis",
  },
];

const TRACE: FlowTrace = {
  name: "check",
  nodes: [
    { id: "tool-1", type: "tool", name: "translate" },
    { id: "tool-2", type: "tool", name: "qa" },
  ],
  events: [
    { ts: 1, type: "enter", nodeId: "tool-1", partId: "b1" },
    { ts: 2, type: "exit", nodeId: "tool-1", partId: "b1" },
    { ts: 3, type: "enter", nodeId: "tool-2", partId: "b1" },
    { ts: 4, type: "exit", nodeId: "tool-2", partId: "b1" },
  ],
  parts: {
    b1: {
      initial: { id: "b1", type: "Block", summary: "Hello", sourceText: "Hello" },
      afterNode: {
        "tool-1": {
          id: "b1",
          type: "Block",
          summary: "Hello",
          sourceText: "Hello",
          targetText: "Bonjour",
        },
        "tool-2": {
          id: "b1",
          type: "Block",
          summary: "Hello",
          sourceText: "Hello",
          targetText: "Bonjour",
        },
      },
    },
  },
  durationUs: 4,
};

describe("FlowDiagramView", () => {
  it("draws the steps as nodes with no authoring affordances", () => {
    render(
      <FlowDiagramView flow={{ steps: [{ tool: "translate" }, { tool: "qa" }] }} tools={TOOLS} />,
    );
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
    expect(screen.getByText("Translate")).toBeInTheDocument();
    expect(screen.getByText("Quality Check")).toBeInTheDocument();
    // No add, no remove, no run, no step-count toolbar.
    expect(screen.queryByLabelText("Add tool")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Remove tool")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Run flow")).not.toBeInTheDocument();
    expect(screen.queryByText("2 steps")).not.toBeInTheDocument();
  });

  it("renders a parallel group with every branch and no branch editing", () => {
    render(
      <FlowDiagramView
        flow={{
          steps: [
            { tool: "translate" },
            { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
          ],
        }}
        tools={TOOLS}
      />,
    );
    expect(screen.getByText("Quality Check")).toBeInTheDocument();
    expect(screen.getByText("Word Count")).toBeInTheDocument();
    expect(screen.queryByText("Add branch")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Remove tool")).not.toBeInTheDocument();
  });

  it("flags an input nothing upstream produces", () => {
    // qa consumes the target port; with no translate before it the node
    // carries the unmet-input warning.
    render(<FlowDiagramView flow={{ steps: [{ tool: "qa" }] }} tools={TOOLS} />);
    expect(screen.getByText(/needs/i)).toBeInTheDocument();
  });

  it("replays a loaded run with the playback transport", () => {
    render(
      <FlowDiagramView
        flow={{ steps: [{ tool: "translate" }, { tool: "qa" }] }}
        tools={TOOLS}
        trace={TRACE}
      />,
    );
    expect(screen.getByLabelText("Playback position")).toBeInTheDocument();
  });

  it("explains an empty flow instead of drawing an empty canvas", () => {
    render(<FlowDiagramView flow={{ steps: [] }} tools={TOOLS} />);
    expect(screen.getByText(/no steps yet/i)).toBeInTheDocument();
  });
});
