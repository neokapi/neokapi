import { describe, it, expect } from "vite-plus/test";
import type { GraphViz, GraphVizEdge, GraphVizNode, RelationType } from "../../types/brand-graph";
import { layoutConcepts, hierarchyPair } from "./graph-layout";

function node(id: string, extra?: Partial<GraphVizNode>): GraphVizNode {
  return { id, label: id, term_count: 0, ...extra };
}

function edge(id: string, source: string, target: string, type: RelationType): GraphVizEdge {
  return { id, source, target, type };
}

function gv(nodes: GraphVizNode[], edges: GraphVizEdge[]): GraphViz {
  return { nodes, edges, total: nodes.length, truncated: false };
}

describe("hierarchyPair", () => {
  it("resolves each hierarchy relation to parent → child", () => {
    expect(hierarchyPair(edge("e", "a", "b", "BROADER"))).toEqual({ parent: "a", child: "b" });
    expect(hierarchyPair(edge("e", "a", "b", "HAS_PART"))).toEqual({ parent: "a", child: "b" });
    expect(hierarchyPair(edge("e", "a", "b", "NARROWER"))).toEqual({ parent: "b", child: "a" });
    expect(hierarchyPair(edge("e", "a", "b", "PART_OF"))).toEqual({ parent: "b", child: "a" });
  });

  it("returns null for non-hierarchy relations", () => {
    expect(hierarchyPair(edge("e", "a", "b", "RELATED"))).toBeNull();
    expect(hierarchyPair(edge("e", "a", "b", "COMPETITOR"))).toBeNull();
  });
});

describe("layoutConcepts", () => {
  it("returns an empty layout for an empty graph", () => {
    expect(layoutConcepts(gv([], []))).toEqual({ nodes: {}, width: 0, height: 0 });
  });

  it("places a broader parent above its child", () => {
    const graph = gv([node("a"), node("b")], [edge("e1", "a", "b", "BROADER")]);
    const layout = layoutConcepts(graph);
    expect(layout.nodes.a.level).toBe(0);
    expect(layout.nodes.b.level).toBe(1);
    expect(layout.nodes.b.y).toBeGreaterThan(layout.nodes.a.y);
  });

  it("respects relation direction (NARROWER flips parent/child)", () => {
    const graph = gv([node("a"), node("b")], [edge("e1", "a", "b", "NARROWER")]);
    const layout = layoutConcepts(graph);
    // NARROWER a → b means b is broader, so b sits above a.
    expect(layout.nodes.b.level).toBe(0);
    expect(layout.nodes.a.level).toBe(1);
  });

  it("falls back to a deterministic grid when there is no hierarchy", () => {
    const graph = gv([node("a"), node("b"), node("c")], [edge("e1", "a", "b", "RELATED")]);
    const layout = layoutConcepts(graph);
    // No hierarchy edge → all on level 0 (grid columns default to 4).
    expect(layout.nodes.a.level).toBe(0);
    expect(layout.nodes.b.level).toBe(0);
    expect(layout.nodes.c.level).toBe(0);
    // Distinct x positions in input order.
    expect(layout.nodes.a.x).toBeLessThan(layout.nodes.b.x);
    expect(layout.nodes.b.x).toBeLessThan(layout.nodes.c.x);
  });

  it("levels a three-deep chain", () => {
    const graph = gv(
      [node("a"), node("b"), node("c")],
      [edge("e1", "a", "b", "BROADER"), edge("e2", "b", "c", "BROADER")],
    );
    const layout = layoutConcepts(graph);
    expect(layout.nodes.a.level).toBe(0);
    expect(layout.nodes.b.level).toBe(1);
    expect(layout.nodes.c.level).toBe(2);
    expect(layout.height).toBeGreaterThan(0);
  });

  it("is deterministic — equal input yields equal output", () => {
    const graph = gv(
      [node("a"), node("b"), node("c")],
      [edge("e1", "a", "b", "BROADER"), edge("e2", "a", "c", "BROADER")],
    );
    expect(layoutConcepts(graph)).toEqual(layoutConcepts(graph));
  });

  it("ignores hierarchy edges that reference unknown nodes", () => {
    const graph = gv([node("a")], [edge("e1", "a", "ghost", "BROADER")]);
    const layout = layoutConcepts(graph);
    expect(layout.nodes.a).toBeDefined();
    expect(Object.keys(layout.nodes)).toEqual(["a"]);
  });
});
