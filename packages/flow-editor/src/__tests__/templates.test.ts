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

  it("templates with hasParallel=true contain parallel steps", () => {
    const parallel = FLOW_TEMPLATES.filter((t) => t.hasParallel);
    expect(parallel.length).toBeGreaterThan(0);
    for (const t of parallel) {
      const hasParallelStep = t.spec.steps.some((s) => s.parallel && s.parallel.length > 0);
      expect(hasParallelStep).toBe(true);
    }
  });

  it("templates without hasParallel have only sequential steps", () => {
    const sequential = FLOW_TEMPLATES.filter((t) => !t.hasParallel);
    for (const t of sequential) {
      for (const s of t.spec.steps) {
        expect(s.parallel).toBeUndefined();
      }
    }
  });

  it("all templates produce valid graphs", () => {
    for (const t of FLOW_TEMPLATES) {
      const { nodes, edges } = stepsToGraph(t.spec);
      // Tool nodes only (a flow owns no I/O); every template has at least one step.
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
      let count = 0;
      for (const s of t.spec.steps) {
        if (s.parallel && s.parallel.length > 0) {
          count += s.parallel.length;
        } else {
          count++;
        }
      }
      expect(count).toBe(t.stepCount);
    }
  });

  it("has unique IDs", () => {
    const ids = FLOW_TEMPLATES.map((t) => t.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});
