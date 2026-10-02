import { describe, it, expect } from "vitest";
import { FLOW_TEMPLATES } from "../templates";
import { stepsToGraph } from "../conversion";

describe("FLOW_TEMPLATES", () => {
  it("has at least 5 templates", () => {
    expect(FLOW_TEMPLATES.length).toBeGreaterThanOrEqual(5);
  });

  it("each template has required fields", () => {
    for (const t of FLOW_TEMPLATES) {
      expect(t.id).toBeTruthy();
      expect(t.name).toBeTruthy();
      expect(t.description).toBeTruthy();
      expect(t.category).toBeTruthy();
      expect(t.spec.steps).toBeDefined();
    }
  });

  // Flow steps run in order; the runtime refuses a parallel: list, so no
  // template offers one.
  it("every template's steps run in order", () => {
    for (const t of FLOW_TEMPLATES) {
      for (const s of t.spec.steps) {
        expect(s.parallel).toBeUndefined();
        expect(s.tool).toBeTruthy();
      }
    }
  });

  it("all templates produce valid graphs", () => {
    for (const t of FLOW_TEMPLATES) {
      const { nodes, edges } = stepsToGraph(t.spec);
      // Step nodes only (a flow owns no I/O); every template has at least one
      // step, and every step node is a single tool.
      expect(nodes.length).toBeGreaterThanOrEqual(1);
      expect(nodes.every((n) => n.type === "tool")).toBe(true);
      // All edges reference existing node IDs
      const nodeIds = new Set(nodes.map((n) => n.id));
      for (const e of edges) {
        expect(nodeIds.has(e.source)).toBe(true);
        expect(nodeIds.has(e.target)).toBe(true);
      }
    }
  });

  it("stepCount matches actual step count", () => {
    for (const t of FLOW_TEMPLATES) {
      expect(t.spec.steps.length).toBe(t.stepCount);
    }
  });

  it("has unique IDs", () => {
    const ids = FLOW_TEMPLATES.map((t) => t.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});
