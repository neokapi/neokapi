import { describe, it, expect } from "vite-plus/test";
import type { GraphViz, GraphVizEdge, GraphVizNode, RelationType } from "../../types/brand-graph";
import {
  statusTone,
  statusColorVar,
  isRetiredStatus,
  relationFamily,
  relationEdgeStyle,
  orientEdge,
  neighbourhood,
  relationNeighbours,
} from "./graph-style";

function node(id: string, extra?: Partial<GraphVizNode>): GraphVizNode {
  return { id, label: id, term_count: 0, ...extra };
}
function edge(id: string, source: string, target: string, type: RelationType): GraphVizEdge {
  return { id, source, target, type };
}
function gv(nodes: GraphVizNode[], edges: GraphVizEdge[]): GraphViz {
  return { nodes, edges, total: nodes.length, truncated: false };
}

describe("statusTone / statusColorVar", () => {
  it("buckets statuses into tones", () => {
    expect(statusTone("preferred")).toBe("preferred");
    expect(statusTone("approved")).toBe("approved");
    expect(statusTone("admitted")).toBe("admitted");
    expect(statusTone("forbidden")).toBe("forbidden");
    expect(statusTone("proposed")).toBe("neutral");
    expect(statusTone("deprecated")).toBe("neutral");
    expect(statusTone("")).toBe("neutral");
    expect(statusTone(undefined)).toBe("neutral");
  });

  it("maps tones to distinct theme colours", () => {
    expect(statusColorVar("preferred")).toContain("success");
    expect(statusColorVar("approved")).toContain("primary");
    expect(statusColorVar("admitted")).toContain("warning");
    expect(statusColorVar("forbidden")).toContain("destructive");
    expect(statusColorVar("proposed")).toContain("muted-foreground");
    expect(statusColorVar(undefined)).toContain("muted-foreground");
  });
});

describe("isRetiredStatus", () => {
  it("flags deprecated and forbidden", () => {
    expect(isRetiredStatus("deprecated")).toBe(true);
    expect(isRetiredStatus("forbidden")).toBe(true);
    expect(isRetiredStatus("preferred")).toBe(false);
    expect(isRetiredStatus("")).toBe(false);
  });
});

describe("relationFamily / relationEdgeStyle", () => {
  it("assigns each relation type to a family", () => {
    expect(relationFamily("BROADER")).toBe("hierarchy");
    expect(relationFamily("REPLACED_BY")).toBe("succession");
    expect(relationFamily("USE_INSTEAD")).toBe("guidance");
    expect(relationFamily("EXACT_MATCH")).toBe("equivalence");
    expect(relationFamily("CLOSE_MATCH")).toBe("equivalence");
    expect(relationFamily("COMPETITOR")).toBe("competitor");
    expect(relationFamily("RELATED")).toBe("related");
  });

  it("only governed succession animates", () => {
    expect(relationEdgeStyle("REPLACED_BY").animated).toBe(true);
    expect(relationEdgeStyle("BROADER").animated).toBe(false);
    expect(relationEdgeStyle("RELATED").animated).toBe(false);
  });

  it("symmetric relations are undirected", () => {
    expect(relationEdgeStyle("RELATED").directed).toBe(false);
    expect(relationEdgeStyle("EXACT_MATCH").directed).toBe(false);
    expect(relationEdgeStyle("BROADER").directed).toBe(true);
    expect(relationEdgeStyle("REPLACED_BY").directed).toBe(true);
    expect(relationEdgeStyle("COMPETITOR").directed).toBe(true);
  });

  it("dashes the non-hierarchy families", () => {
    expect(relationEdgeStyle("BROADER").dash).toBeUndefined();
    expect(relationEdgeStyle("REPLACED_BY").dash).toBeUndefined();
    expect(relationEdgeStyle("USE_INSTEAD").dash).toBeTruthy();
    expect(relationEdgeStyle("COMPETITOR").dash).toBeTruthy();
    expect(relationEdgeStyle("RELATED").dash).toBeTruthy();
  });
});

describe("orientEdge", () => {
  it("orients hierarchy edges parent → child", () => {
    expect(orientEdge(edge("e", "a", "b", "BROADER"))).toEqual({ source: "a", target: "b" });
    expect(orientEdge(edge("e", "a", "b", "HAS_PART"))).toEqual({ source: "a", target: "b" });
    expect(orientEdge(edge("e", "a", "b", "NARROWER"))).toEqual({ source: "b", target: "a" });
    expect(orientEdge(edge("e", "a", "b", "PART_OF"))).toEqual({ source: "b", target: "a" });
  });

  it("keeps non-hierarchy edges in stored direction", () => {
    expect(orientEdge(edge("e", "a", "b", "RELATED"))).toEqual({ source: "a", target: "b" });
    expect(orientEdge(edge("e", "a", "b", "COMPETITOR"))).toEqual({ source: "a", target: "b" });
  });
});

describe("neighbourhood", () => {
  const graph = gv(
    [node("a"), node("b"), node("c"), node("d")],
    [
      edge("e1", "a", "b", "RELATED"),
      edge("e2", "c", "a", "COMPETITOR"),
      edge("e3", "c", "d", "RELATED"),
    ],
  );

  it("collects the focus, its neighbours, and touching edges", () => {
    const nbh = neighbourhood(graph, "a");
    expect([...nbh.nodeIds].sort()).toEqual(["a", "b", "c"]);
    expect([...nbh.edgeIds].sort()).toEqual(["e1", "e2"]);
    expect(nbh.nodeIds.has("d")).toBe(false);
  });

  it("returns empty sets for an unknown or missing focus", () => {
    expect(neighbourhood(graph, "ghost").nodeIds.size).toBe(0);
    expect(neighbourhood(graph, undefined).nodeIds.size).toBe(0);
  });
});

describe("relationNeighbours", () => {
  const graph = gv(
    [node("a"), node("b"), node("c")],
    [edge("e1", "a", "b", "RELATED"), edge("e2", "c", "a", "COMPETITOR")],
  );

  it("returns neighbours with the correct direction", () => {
    const result = relationNeighbours(graph, "a");
    expect(result).toEqual([
      { edgeId: "e1", otherId: "b", type: "RELATED", outgoing: true },
      { edgeId: "e2", otherId: "c", type: "COMPETITOR", outgoing: false },
    ]);
  });
});
