import { describe, it, expect } from "vitest";
import {
  traceNodeToEditorNode,
  remapEventsToEditor,
  activeEditorNodes,
  partsThroughStep,
  snapshotDelta,
  stepToolCounts,
  nodeSpans,
  edgeTransits,
  formatUs,
} from "../traceSelectors";
import type { FlowTrace, PartSnapshot } from "../traceTypes";

// A trace the engine would produce for the 2-step flow [redact, translate]:
// reader/writer nodes bracket the tool nodes, parts snapshot after each tool.
function sampleTrace(): FlowTrace {
  return {
    name: "lab",
    nodes: [
      { id: "reader", type: "reader", name: "read" },
      { id: "tool-1", type: "tool", name: "redact" },
      { id: "tool-2", type: "tool", name: "translate" },
      { id: "writer", type: "writer", name: "write" },
    ],
    events: [
      { ts: 1, type: "enter", nodeId: "tool-1", partId: "b1" },
      { ts: 2, type: "exit", nodeId: "tool-1", partId: "b1" },
      { ts: 3, type: "enter", nodeId: "tool-2", partId: "b1" },
      { ts: 5, type: "exit", nodeId: "tool-2", partId: "b1" },
      { ts: 6, type: "enter", nodeId: "writer", partId: "b1" },
    ],
    parts: {
      b1: {
        initial: {
          id: "b1",
          type: "Block",
          summary: "Call Jane Doe",
          sourceText: "Call Jane Doe",
          detail: {
            overlays: [
              {
                type: "entity",
                side: "source",
                spans: [{ start: 5, end: 13, text: "Jane Doe", note: "entity:person" }],
              },
            ],
          },
        },
        afterNode: {
          "tool-1": {
            id: "b1",
            type: "Block",
            summary: "Call Jane Doe",
            sourceText: "Call [REDACTED:Person]",
            detail: {
              annotations: [{ key: "redaction.secret", summary: "redaction.secret" }],
            },
          },
          "tool-2": {
            id: "b1",
            type: "Block",
            summary: "Call Jane Doe",
            sourceText: "Call [REDACTED:Person]",
            targetText: "Appeler [REDACTED:Person]",
            detail: {
              annotations: [{ key: "redaction.secret", summary: "redaction.secret" }],
            },
          },
        },
      },
    },
    durationUs: 10,
  };
}

const STEPS = [{ tool: "redact" }, { tool: "translate" }];

describe("stepToolCounts", () => {
  it("counts 1 per plain step and the branch count for parallel groups", () => {
    expect(stepToolCounts(STEPS)).toEqual([1, 1]);
    expect(stepToolCounts([{ tool: "a" }, { tool: "", parallel: [{}, {}, {}] }])).toEqual([1, 3]);
  });
});

describe("traceNodeToEditorNode", () => {
  it("maps trace tool nodes onto editor step ids by order", () => {
    const m = traceNodeToEditorNode(sampleTrace(), stepToolCounts(STEPS));
    expect(m.get("tool-1")).toBe("tool-0");
    expect(m.get("tool-2")).toBe("tool-1");
    expect(m.has("reader")).toBe(false);
  });

  it("maps a parallel group's trace nodes onto the single group node", () => {
    const trace = sampleTrace();
    const m = traceNodeToEditorNode(trace, [2]); // one parallel step with 2 branches
    expect(m.get("tool-1")).toBe("tool-0");
    expect(m.get("tool-2")).toBe("tool-0");
  });
});

describe("remapEventsToEditor", () => {
  it("rewrites node ids and drops reader/writer events", () => {
    const events = remapEventsToEditor(sampleTrace(), stepToolCounts(STEPS));
    expect(events).toHaveLength(4);
    expect(events.map((e) => e.nodeId)).toEqual(["tool-0", "tool-0", "tool-1", "tool-1"]);
  });
});

describe("activeEditorNodes", () => {
  it("reports nodes with parts entered but not exited at the cursor", () => {
    const events = remapEventsToEditor(sampleTrace(), stepToolCounts(STEPS));
    expect([...activeEditorNodes(events, 1)]).toEqual(["tool-0"]);
    expect(activeEditorNodes(events, 2).size).toBe(0);
    expect([...activeEditorNodes(events, 3)]).toEqual(["tool-1"]);
    expect(activeEditorNodes(events, 4).size).toBe(0);
  });
});

describe("partsThroughStep", () => {
  it("pairs each part's entering and leaving state for a step", () => {
    const trace = sampleTrace();
    const counts = stepToolCounts(STEPS);

    const redact = partsThroughStep(trace, counts, 0);
    expect(redact).toHaveLength(1);
    expect(redact[0].before.sourceText).toBe("Call Jane Doe");
    expect(redact[0].after.sourceText).toBe("Call [REDACTED:Person]");

    const translate = partsThroughStep(trace, counts, 1);
    expect(translate).toHaveLength(1);
    expect(translate[0].before.sourceText).toBe("Call [REDACTED:Person]");
    expect(translate[0].after.targetText).toBe("Appeler [REDACTED:Person]");
  });
});

describe("snapshotDelta", () => {
  it("reports the overlay/annotation delta a step produced", () => {
    const trace = sampleTrace();
    const [t] = partsThroughStep(trace, stepToolCounts(STEPS), 0);
    const delta = snapshotDelta(t.before, t.after);
    expect(delta.sourceChanged).toBe(true);
    expect(delta.targetChanged).toBe(false);
    // redact consumed the entity overlay and vaulted the secret annotation.
    expect(delta.removedOverlays).toEqual([{ type: "entity", side: "source", spans: 1 }]);
    expect(delta.addedAnnotations).toEqual(["redaction.secret"]);
  });

  it("reports added overlays with span counts", () => {
    const before: PartSnapshot = { id: "b", type: "Block", summary: "s", detail: {} };
    const after: PartSnapshot = {
      id: "b",
      type: "Block",
      summary: "s",
      detail: {
        overlays: [
          {
            type: "segmentation",
            side: "source",
            spans: [
              { start: 0, end: 5 },
              { start: 6, end: 9 },
            ],
          },
        ],
      },
    };
    const delta = snapshotDelta(before, after);
    expect(delta.addedOverlays).toEqual([{ type: "segmentation", side: "source", spans: 2 }]);
    expect(delta.removedOverlays).toEqual([]);
  });
});

describe("nodeSpans", () => {
  it("reports each node's wall-clock window (first enter → last exit) at the cursor", () => {
    const events = remapEventsToEditor(sampleTrace(), stepToolCounts(STEPS));
    // Full window: tool-0 spans ts 1→2, tool-1 spans ts 3→5.
    const full = nodeSpans(events, events.length);
    expect(full.get("tool-0")).toBe(1);
    expect(full.get("tool-1")).toBe(2);
    // Mid-playback: tool-1 has entered but not exited — no span yet.
    const mid = nodeSpans(events, 3);
    expect(mid.get("tool-0")).toBe(1);
    expect(mid.has("tool-1")).toBe(false);
  });
});

describe("formatUs", () => {
  it("formats µs/ms/s at readable precision", () => {
    expect(formatUs(300)).toBe("300µs");
    expect(formatUs(1_600)).toBe("1.6ms");
    expect(formatUs(2_100_000)).toBe("2.1s");
  });
});

describe("edgeTransits", () => {
  it("reports a part as on the edge between its exit and next enter", () => {
    const events = remapEventsToEditor(sampleTrace(), stepToolCounts(STEPS));
    // cursor 2 = after exit tool-0, before enter tool-1: b1 is on tool-0→tool-1.
    expect(edgeTransits(events, 2).get("tool-0→tool-1")).toBe(1);
    // cursor 1/3 = inside a node: no edge transit.
    expect(edgeTransits(events, 1).size).toBe(0);
    expect(edgeTransits(events, 3).size).toBe(0);
    // cursor 4 = after the final exit: b1 is on the way to the sink.
    expect(edgeTransits(events, 4).get("tool-1→endpoint-sink")).toBe(1);
  });
});
