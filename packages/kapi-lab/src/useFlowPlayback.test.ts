// @vitest-environment jsdom
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useFlowPlayback } from "./useFlowPlayback";
import type { FlowNode, PartSnapshotSet, TraceEvent } from "@neokapi/ui-primitives/preview";

const nodes: FlowNode[] = [
  { id: "reader", type: "reader", name: "json", label: "json" },
  { id: "tool-0", type: "tool", name: "pseudo-translate", label: "pseudo-translate" },
  { id: "writer", type: "writer", name: "json", label: "json" },
];

const parts: Record<string, PartSnapshotSet> = {
  b1: {
    initial: { id: "b1", type: "Block", summary: "Hello", sourceText: "Hello" },
    afterNode: { "tool-0": { id: "b1", type: "Block", summary: "Ħëłłö", targetText: "Ħëłłö" } },
  },
};

// reader exits b1 at 10, tool enters at 20, tool exits (translated) at 30,
// writer enters at 40. Distinct timestamps: 10,20,30,40 → frames [0,10,20,30,40].
const events: TraceEvent[] = [
  { ts: 10, type: "exit", nodeId: "reader", partId: "b1" },
  { ts: 20, type: "enter", nodeId: "tool-0", partId: "b1" },
  { ts: 30, type: "exit", nodeId: "tool-0", partId: "b1" },
  { ts: 40, type: "enter", nodeId: "writer", partId: "b1" },
];

describe("useFlowPlayback", () => {
  it("builds a frame per distinct timestamp plus a synthetic empty frame", () => {
    const { result } = renderHook(() => useFlowPlayback({ events, nodes, parts }));
    expect(result.current.state.frameCount).toBe(5); // [0,10,20,30,40]
    expect(result.current.state.frameIndex).toBe(0);
    expect(result.current.state.atStart).toBe(true);
    expect(result.current.delta.summary).toMatch(/nothing in the pipeline/i);
  });

  it("advances one observable transition per Next, with a narrated delta", () => {
    const { result } = renderHook(() => useFlowPlayback({ events, nodes, parts }));

    act(() => result.current.stepNext()); // → frame 1 (ts 10): reader exit
    expect(result.current.state.frameIndex).toBe(1);
    expect(result.current.state.time).toBe(10);
    expect(result.current.delta.affectedPartIds).toEqual(["b1"]);
    expect(result.current.delta.summary).toMatch(/left .*json/i);

    act(() => result.current.stepNext()); // → frame 2 (ts 20): tool enter
    expect(result.current.delta.summary).toMatch(/entered .*pseudo-translate/i);
    // b1 is now inside the tool node.
    expect(
      result.current.particles.some((p) => p.position === "node" && p.nodeId === "tool-0"),
    ).toBe(true);
  });

  it("steps backward and clamps at both ends", () => {
    const { result } = renderHook(() => useFlowPlayback({ events, nodes, parts }));
    act(() => result.current.stepPrev()); // already at start
    expect(result.current.state.frameIndex).toBe(0);

    act(() => result.current.stepTo(99)); // clamp to last
    expect(result.current.state.frameIndex).toBe(4);
    expect(result.current.state.atEnd).toBe(true);

    act(() => result.current.stepPrev());
    expect(result.current.state.frameIndex).toBe(3);
  });

  it("surfaces the translated target in the delta when a part leaves the tool", () => {
    const { result } = renderHook(() => useFlowPlayback({ events, nodes, parts }));
    act(() => result.current.stepTo(3)); // frame 3 (ts 30): tool exit with target
    expect(result.current.delta.summary).toContain("Ħëłłö");
  });
});
