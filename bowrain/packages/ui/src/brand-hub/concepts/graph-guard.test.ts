import { describe, it, expect } from "vite-plus/test";
import { shouldGuardGraph, GRAPH_READABLE_NODE_LIMIT } from "./graph-guard";

describe("shouldGuardGraph", () => {
  it("does not guard a small, wide-open graph", () => {
    expect(
      shouldGuardGraph({ truncated: false, nodeCount: 12, hasFocus: false, hasFilter: false }),
    ).toBe(false);
  });

  it("guards a truncated wide-open graph", () => {
    expect(
      shouldGuardGraph({ truncated: true, nodeCount: 60, hasFocus: false, hasFilter: false }),
    ).toBe(true);
  });

  it("guards when the node count exceeds the readable limit even if not flagged truncated", () => {
    expect(
      shouldGuardGraph({
        truncated: false,
        nodeCount: GRAPH_READABLE_NODE_LIMIT + 1,
        hasFocus: false,
        hasFilter: false,
      }),
    ).toBe(true);
  });

  it("does not guard at exactly the readable limit", () => {
    expect(
      shouldGuardGraph({
        truncated: false,
        nodeCount: GRAPH_READABLE_NODE_LIMIT,
        hasFocus: false,
        hasFilter: false,
      }),
    ).toBe(false);
  });

  it("never guards once a concept is focused, however large the graph", () => {
    expect(
      shouldGuardGraph({ truncated: true, nodeCount: 500, hasFocus: true, hasFilter: false }),
    ).toBe(false);
  });

  it("never guards once a scope filter is active", () => {
    expect(
      shouldGuardGraph({ truncated: true, nodeCount: 500, hasFocus: false, hasFilter: true }),
    ).toBe(false);
  });
});
