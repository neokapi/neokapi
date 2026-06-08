import { describe, it, expect } from "vitest";
import { stepsToGraph } from "../conversion";
import {
  resolveStepLocation,
  stepAtLocation,
  updateStepAtLocation,
  removeStepAtLocation,
  type NodeStepData,
} from "../stepResolve";
import type { FlowSpec } from "../types";
import type { Node as RFNode } from "@xyflow/react";

/**
 * Identity-based step resolution (findings #10 + #11).
 *
 * The bug: the flow editor resolved the selected node by tool NAME, so the 2nd
 * of two duplicate-tool nodes edited/deleted the 1st (and config edits to a
 * parallel branch were written to the wrapper step). These tests pin the fix:
 * `stepsToGraph` threads each step's flat index (+ parallel-branch index) onto
 * `node.data`, and resolution operates on that identity.
 */

// Pull the resolvable data off a graph node by its React Flow id.
function dataFor(nodes: RFNode[], id: string): NodeStepData {
  const n = nodes.find((node) => node.id === id);
  if (!n) throw new Error(`no node ${id}`);
  return n.data as NodeStepData;
}

describe("stepsToGraph carries step identity on node data", () => {
  it("threads stepIndex onto sequential main nodes", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }, { tool: "ai-translate" }, { tool: "qa-check" }],
    };
    const { nodes } = stepsToGraph(spec);
    // Three tool nodes: tool-0, tool-1, tool-2 in declaration order.
    expect(dataFor(nodes, "tool-0").stepIndex).toBe(0);
    expect(dataFor(nodes, "tool-1").stepIndex).toBe(1);
    expect(dataFor(nodes, "tool-2").stepIndex).toBe(2);
  });

  it("threads stIndex onto source-transform nodes", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }, { tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };
    const { nodes } = stepsToGraph(spec);
    expect(dataFor(nodes, "tool-0").stage).toBe("source-transform");
    expect(dataFor(nodes, "tool-0").stIndex).toBe(0);
    expect(dataFor(nodes, "tool-1").stIndex).toBe(1);
    // The main step keeps a stepIndex relative to spec.steps, not the global id.
    expect(dataFor(nodes, "tool-2").stepIndex).toBe(0);
    expect(dataFor(nodes, "tool-2").stage).toBeUndefined();
  });

  it("emits one parallel group node carrying its branches; a branch location is its stepIndex + branchIndex", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "ai-translate" },
        { tool: "", parallel: [{ tool: "qa-a" }, { tool: "qa-b" }, { tool: "qa-c" }] },
      ],
    };
    const { nodes } = stepsToGraph(spec);
    // tool-0 = sequential; the parallel step is a single composite node (tool-1).
    expect(dataFor(nodes, "tool-0").stepIndex).toBe(0);
    const group = nodes.find((n) => n.type === "parallel")!;
    expect(group.data.stepIndex).toBe(1);
    expect((group.data.branches as unknown[]).length).toBe(3);
    // Selecting branch b resolves to that branch of step index 1.
    for (let b = 0; b < 3; b++) {
      expect(
        resolveStepLocation({ stepIndex: group.data.stepIndex as number, branchIndex: b }),
      ).toEqual({ isSourceTransform: false, index: 1, branchIndex: b });
    }
  });
});

/** Build a parallel-branch location from the composite group node (as the UI does). */
function branchLocation(nodes: RFNode[], branchIndex: number) {
  const group = nodes.find((n) => n.type === "parallel");
  if (!group) throw new Error("no parallel group node");
  return resolveStepLocation({ stepIndex: group.data.stepIndex as number, branchIndex })!;
}

describe("resolveStepLocation", () => {
  it("returns null for nodes with no resolvable index / empty data", () => {
    expect(resolveStepLocation(undefined)).toBeNull();
    expect(resolveStepLocation({ toolName: "x" })).toBeNull();
  });

  it("resolves a source-transform node by stIndex", () => {
    expect(resolveStepLocation({ stage: "source-transform", stIndex: 2 })).toEqual({
      isSourceTransform: true,
      index: 2,
    });
  });

  it("resolves a sequential main node by stepIndex", () => {
    expect(resolveStepLocation({ stepIndex: 3 })).toEqual({
      isSourceTransform: false,
      index: 3,
    });
  });

  it("resolves a parallel branch by stepIndex + branchIndex", () => {
    expect(resolveStepLocation({ stepIndex: 1, branchIndex: 2 })).toEqual({
      isSourceTransform: false,
      index: 1,
      branchIndex: 2,
    });
  });
});

describe("selecting the 2nd duplicate-tool node hits the right step (#10)", () => {
  const spec: FlowSpec = {
    steps: [
      { tool: "ai-translate", config: { targetLang: "fr" } },
      { tool: "ai-translate", config: { targetLang: "de" } },
    ],
  };

  it("stepAtLocation resolves the 2nd node to the 2nd step", () => {
    const { nodes } = stepsToGraph(spec);
    const first = resolveStepLocation(dataFor(nodes, "tool-0"))!;
    const second = resolveStepLocation(dataFor(nodes, "tool-1"))!;
    expect(stepAtLocation(spec, first)).toBe(spec.steps[0]);
    expect(stepAtLocation(spec, second)).toBe(spec.steps[1]);
    expect(stepAtLocation(spec, second)!.config).toEqual({ targetLang: "de" });
  });

  it("editing the 2nd node writes only the 2nd step", () => {
    const { nodes } = stepsToGraph(spec);
    const second = resolveStepLocation(dataFor(nodes, "tool-1"))!;
    const updated = updateStepAtLocation(spec, second, (s) => ({
      ...s,
      config: { targetLang: "es" },
    }));
    expect(updated.steps[0].config).toEqual({ targetLang: "fr" }); // 1st untouched
    expect(updated.steps[1].config).toEqual({ targetLang: "es" });
  });

  it("deleting the 2nd node removes only the 2nd step, not every same-tool step", () => {
    const { nodes } = stepsToGraph(spec);
    const second = resolveStepLocation(dataFor(nodes, "tool-1"))!;
    const updated = removeStepAtLocation(spec, second);
    expect(updated.steps).toHaveLength(1);
    expect(updated.steps[0].config).toEqual({ targetLang: "fr" });
  });

  it("deleting a parallel branch keeps sibling branches", () => {
    const parSpec: FlowSpec = {
      steps: [
        { tool: "extract" },
        {
          tool: "",
          parallel: [{ tool: "ai-translate" }, { tool: "ai-translate" }, { tool: "qa" }],
        },
      ],
    };
    const { nodes } = stepsToGraph(parSpec);
    // Remove the 2nd branch (branchIndex 1) of the parallel group.
    const branch = branchLocation(nodes, 1);
    expect(branch).toEqual({ isSourceTransform: false, index: 1, branchIndex: 1 });
    const updated = removeStepAtLocation(parSpec, branch);
    // The parallel group keeps its other two branches; the sequential step survives.
    expect(updated.steps).toHaveLength(2);
    expect(updated.steps[0].tool).toBe("extract");
    expect(updated.steps[1].parallel).toHaveLength(2);
    expect(updated.steps[1].parallel!.map((p) => p.tool)).toEqual(["ai-translate", "qa"]);
  });

  it("collapses a parallel group to a plain step when one branch remains", () => {
    const parSpec: FlowSpec = {
      steps: [{ tool: "", parallel: [{ tool: "a" }, { tool: "b" }] }],
    };
    const { nodes } = stepsToGraph(parSpec);
    const branch = branchLocation(nodes, 1); // 2nd branch
    const updated = removeStepAtLocation(parSpec, branch);
    expect(updated.steps).toHaveLength(1);
    expect(updated.steps[0].tool).toBe("a");
    expect(updated.steps[0].parallel).toBeUndefined();
  });
});

describe("config edits to a parallel branch persist to the branch (#11)", () => {
  const spec: FlowSpec = {
    steps: [
      {
        tool: "",
        parallel: [
          { tool: "ai-translate", config: { targetLang: "fr" } },
          { tool: "mt-translate", config: { targetLang: "de" } },
        ],
      },
    ],
  };

  it("writes to step.parallel[branchIndex], never the wrapper", () => {
    const { nodes } = stepsToGraph(spec);
    const branch = branchLocation(nodes, 1); // 2nd branch
    expect(branch.branchIndex).toBe(1);
    const updated = updateStepAtLocation(spec, branch, (s) => ({
      ...s,
      config: { targetLang: "es" },
    }));
    // Wrapper has no config of its own; the branch carries the edit.
    expect(updated.steps[0].config).toBeUndefined();
    expect(updated.steps[0].parallel![0].config).toEqual({ targetLang: "fr" }); // sibling untouched
    expect(updated.steps[0].parallel![1].config).toEqual({ targetLang: "es" });
  });

  it("round-trips through the graph: the panel target matches the edited branch", () => {
    const { nodes } = stepsToGraph(spec);
    // The 2nd branch resolves to the 2nd branch step, whose config the panel
    // renders — and updateStepAtLocation writes back to that same branch.
    const branch = branchLocation(nodes, 1);
    const shown = stepAtLocation(spec, branch);
    expect(shown).toBe(spec.steps[0].parallel![1]);
    expect(shown!.config).toEqual({ targetLang: "de" });
  });
});

describe("source-transform stage resolution", () => {
  it("edits the correct source-transform among duplicates", () => {
    const spec: FlowSpec = {
      sourceTransforms: [
        { tool: "redact", config: { mode: "a" } },
        { tool: "redact", config: { mode: "b" } },
      ],
      steps: [{ tool: "ai-translate" }],
    };
    const { nodes } = stepsToGraph(spec);
    const second = resolveStepLocation(dataFor(nodes, "tool-1"))!;
    expect(second).toEqual({ isSourceTransform: true, index: 1 });
    const updated = updateStepAtLocation(spec, second, (s) => ({ ...s, config: { mode: "c" } }));
    expect(updated.sourceTransforms![0].config).toEqual({ mode: "a" });
    expect(updated.sourceTransforms![1].config).toEqual({ mode: "c" });
  });

  it("removing the only source-transform drops the array", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };
    const { nodes } = stepsToGraph(spec);
    const loc = resolveStepLocation(dataFor(nodes, "tool-0"))!;
    const updated = removeStepAtLocation(spec, loc);
    expect(updated.sourceTransforms).toBeUndefined();
    expect(updated.steps).toHaveLength(1);
  });
});
