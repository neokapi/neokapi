import { describe, it, expect } from "vitest";
import {
  traceNodeToEditorNode,
  remapEventsToEditor,
  activeEditorNodes,
  partsThroughStep,
  snapshotDelta,
  stepToolCounts,
} from "../traceSelectors";
import type { FlowTrace, PartSnapshot } from "../traceTypes";

// A trace the engine would produce for the 2-step flow [redact, ai-translate]:
// reader/writer nodes bracket the tool nodes, parts snapshot after each tool.
function sampleTrace(): FlowTrace {
  return {
    name: "lab",
    nodes: [
      { id: "reader", type: "reader", name: "read" },
      { id: "tool-1", type: "tool", name: "redact" },
      { id: "tool-2", type: "tool", name: "ai-translate" },
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

const STEPS = [{ tool: "redact" }, { tool: "ai-translate" }];

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
