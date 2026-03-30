import { describe, it, expect } from "vitest";
import { computeNodeStats } from "../traceTypes";
import type { TraceEvent } from "../traceTypes";

describe("computeNodeStats", () => {
  it("counts parts processed per node", () => {
    const events: TraceEvent[] = [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
      { ts: 100, type: "exit", nodeId: "tool-0", partId: "p1" },
      { ts: 200, type: "enter", nodeId: "tool-0", partId: "p2" },
      { ts: 300, type: "exit", nodeId: "tool-0", partId: "p2" },
    ];
    const stats = computeNodeStats(events);
    expect(stats.get("tool-0")!.partsProcessed).toBe(2);
  });

  it("computes duration from enter/exit pairs", () => {
    const events: TraceEvent[] = [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
      { ts: 500, type: "exit", nodeId: "tool-0", partId: "p1" },
      { ts: 600, type: "enter", nodeId: "tool-0", partId: "p2" },
      { ts: 900, type: "exit", nodeId: "tool-0", partId: "p2" },
    ];
    const stats = computeNodeStats(events);
    // Duration = (500-0) + (900-600) = 800
    expect(stats.get("tool-0")!.durationUs).toBe(800);
  });

  it("tracks multiple nodes independently", () => {
    const events: TraceEvent[] = [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
      { ts: 50, type: "enter", nodeId: "tool-1", partId: "p1" },
      { ts: 100, type: "exit", nodeId: "tool-0", partId: "p1" },
      { ts: 200, type: "exit", nodeId: "tool-1", partId: "p1" },
    ];
    const stats = computeNodeStats(events);
    expect(stats.get("tool-0")!.partsProcessed).toBe(1);
    expect(stats.get("tool-1")!.partsProcessed).toBe(1);
    expect(stats.get("tool-0")!.durationUs).toBe(100);
    expect(stats.get("tool-1")!.durationUs).toBe(150);
  });

  it("detects error events", () => {
    const events: TraceEvent[] = [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
      { ts: 100, type: "error", nodeId: "tool-0", partId: "p1", meta: { error: "boom" } },
    ];
    const stats = computeNodeStats(events);
    expect(stats.get("tool-0")!.hasError).toBe(true);
    expect(stats.get("tool-0")!.errorMessage).toBe("boom");
  });

  it("returns empty map for no events", () => {
    const stats = computeNodeStats([]);
    expect(stats.size).toBe(0);
  });

  it("handles enter without exit (in-progress)", () => {
    const events: TraceEvent[] = [
      { ts: 0, type: "enter", nodeId: "tool-0", partId: "p1" },
    ];
    const stats = computeNodeStats(events);
    expect(stats.get("tool-0")!.partsProcessed).toBe(0);
    expect(stats.get("tool-0")!.durationUs).toBe(0);
  });
});
