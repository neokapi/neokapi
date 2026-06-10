// Pure selectors over a FlowTrace for in-canvas playback (no React). The
// editor renders the designed flow; a trace from running THAT flow plays back
// on the same nodes — these helpers map trace node ids onto editor node ids,
// window the events to a playback cursor, and compute the per-node part
// states (with overlay/annotation deltas) the run inspector shows.

import type { FlowTrace, TraceEvent, TraceNode, PartSnapshot, OverlaySnapshot } from "./traceTypes";

/** Trace nodes that correspond to editor steps (tools), in trace order. */
export function traceToolNodes(trace: FlowTrace): TraceNode[] {
  return trace.nodes.filter((n) => n.type === "tool" || n.type === "bridge-tool");
}

/**
 * Map trace node ids → editor node ids by tool order: the editor's nodes are
 * `tool-<stepIndex>` in step order, and the engine compiled the same ordered
 * steps, so the i-th trace tool node is the i-th editor step. (A parallel
 * group occupies one editor node but N trace nodes; those all map onto the
 * group node.)
 */
export function traceNodeToEditorNode(
  trace: FlowTrace,
  stepToolCounts: number[],
): Map<string, string> {
  const m = new Map<string, string>();
  const tools = traceToolNodes(trace);
  let t = 0;
  stepToolCounts.forEach((count, stepIndex) => {
    for (let k = 0; k < count && t < tools.length; k++, t++) {
      m.set(tools[t].id, `tool-${stepIndex}`);
    }
  });
  return m;
}

/** Remap a trace's events onto editor node ids (events on reader/writer nodes drop). */
export function remapEventsToEditor(trace: FlowTrace, stepToolCounts: number[]): TraceEvent[] {
  const map = traceNodeToEditorNode(trace, stepToolCounts);
  const out: TraceEvent[] = [];
  for (const e of trace.events) {
    const nodeId = map.get(e.nodeId);
    if (nodeId) out.push({ ...e, nodeId });
  }
  return out;
}

/** Editor node ids with parts entered but not yet exited at the cursor. */
export function activeEditorNodes(events: TraceEvent[], cursor: number): Set<string> {
  const inFlight = new Map<string, number>();
  for (const e of events.slice(0, cursor)) {
    const key = `${e.nodeId}:${e.partId ?? ""}`;
    if (e.type === "enter") inFlight.set(key, (inFlight.get(key) ?? 0) + 1);
    else if (e.type === "exit") {
      const n = (inFlight.get(key) ?? 0) - 1;
      if (n <= 0) inFlight.delete(key);
      else inFlight.set(key, n);
    }
  }
  const active = new Set<string>();
  for (const key of inFlight.keys()) active.add(key.slice(0, key.indexOf(":")));
  return active;
}

/**
 * Per-node wall-clock span at the cursor: last exit − first enter, in µs.
 * This is the node's active window in the run — bounded by the total duration
 * and intuitive on a node badge, unlike the per-part busy sum (pipelined parts
 * overlap, so the sum can exceed the wall total).
 */
export function nodeSpans(events: TraceEvent[], cursor: number): Map<string, number> {
  const first = new Map<string, number>();
  const last = new Map<string, number>();
  for (const e of events.slice(0, cursor)) {
    if (e.type === "enter" && !first.has(e.nodeId)) first.set(e.nodeId, e.ts);
    if (e.type === "exit") last.set(e.nodeId, e.ts);
  }
  const spans = new Map<string, number>();
  for (const [nodeId, start] of first) {
    const end = last.get(nodeId);
    if (end !== undefined && end >= start) spans.set(nodeId, end - start);
  }
  return spans;
}

/**
 * Parts in transit per edge at the cursor — a part is "on" the edge X→Y
 * between its exit from X and its enter into Y (or on the X→sink edge after
 * its final exit). Keys are `${sourceNodeId}→${targetNodeId}` in editor node
 * ids. This is what makes the playback dots literal: an edge shows movement
 * exactly when a part is mid-hop at the cursor, not as a decorative loop.
 */
export function edgeTransits(
  events: TraceEvent[],
  cursor: number,
  sinkId = "endpoint-sink",
): Map<string, number> {
  // Last applied event per part at the cursor.
  const lastApplied = new Map<string, { idx: number; type: string; nodeId: string }>();
  const upTo = Math.min(cursor, events.length);
  for (let i = 0; i < upTo; i++) {
    const e = events[i];
    if (!e.partId) continue;
    lastApplied.set(e.partId, { idx: i, type: e.type, nodeId: e.nodeId });
  }

  const transits = new Map<string, number>();
  for (const [partId, last] of lastApplied) {
    if (last.type !== "exit") continue; // inside a node (or errored) — not on an edge
    // The part left last.nodeId; its next enter names the edge it is crossing.
    let target = sinkId;
    for (let i = last.idx + 1; i < events.length; i++) {
      const e = events[i];
      if (e.partId === partId && e.type === "enter") {
        target = e.nodeId;
        break;
      }
    }
    const key = `${last.nodeId}→${target}`;
    transits.set(key, (transits.get(key) ?? 0) + 1);
  }
  return transits;
}

/** Human label for a µs duration: 300µs, 1.6ms, 2.1s. */
export function formatUs(us: number): string {
  if (us >= 1_000_000) return `${(us / 1_000_000).toFixed(1)}s`;
  if (us >= 1_000) return `${(us / 1_000).toFixed(1)}ms`;
  return `${us}µs`;
}

/** One part's before/after states at a node, for the run inspector. */
export interface PartTransition {
  partId: string;
  before: PartSnapshot;
  after: PartSnapshot;
}

/**
 * The parts that passed through an editor step, each with its state entering
 * (the previous tool's snapshot, or the initial state) and leaving the step.
 */
export function partsThroughStep(
  trace: FlowTrace,
  stepToolCounts: number[],
  stepIndex: number,
): PartTransition[] {
  const map = traceNodeToEditorNode(trace, stepToolCounts);
  const editorId = `tool-${stepIndex}`;
  // Trace tool node ids for this step, and the ordered list of all tool ids.
  const tools = traceToolNodes(trace);
  const stepNodeIds = tools.filter((n) => map.get(n.id) === editorId).map((n) => n.id);
  if (stepNodeIds.length === 0) return [];

  const out: PartTransition[] = [];
  for (const [partId, set] of Object.entries(trace.parts)) {
    // After: the snapshot from this step's (last) trace node.
    let after: PartSnapshot | undefined;
    for (const id of stepNodeIds) {
      after = set.afterNode?.[id] ?? after;
    }
    if (!after) continue;
    // Before: the nearest earlier tool node with a snapshot, else initial.
    let before: PartSnapshot = set.initial;
    for (const n of tools) {
      if (stepNodeIds.includes(n.id)) break;
      const snap = set.afterNode?.[n.id];
      if (snap) before = snap;
    }
    out.push({ partId, before, after });
  }
  return out;
}

/** Key an overlay for delta comparison. */
const overlayKey = (o: OverlaySnapshot) => `${o.type}@${o.side}${o.layer ? `#${o.layer}` : ""}`;

/** What a step changed on a part — the content-model teaching surface. */
export interface SnapshotDelta {
  sourceChanged: boolean;
  targetChanged: boolean;
  /** Overlays added (or grown) by this step, with the span count added. */
  addedOverlays: { type: string; side: string; spans: number }[];
  /** Overlays removed (or shrunk) by this step. */
  removedOverlays: { type: string; side: string; spans: number }[];
  /** Block annotations added by this step. */
  addedAnnotations: string[];
  /** Block annotations removed by this step (e.g. unredact clearing secrets). */
  removedAnnotations: string[];
}

export function snapshotDelta(before: PartSnapshot, after: PartSnapshot): SnapshotDelta {
  const beforeOv = new Map((before.detail?.overlays ?? []).map((o) => [overlayKey(o), o]));
  const afterOv = new Map((after.detail?.overlays ?? []).map((o) => [overlayKey(o), o]));

  const addedOverlays: SnapshotDelta["addedOverlays"] = [];
  const removedOverlays: SnapshotDelta["removedOverlays"] = [];
  for (const [key, o] of afterOv) {
    const prev = beforeOv.get(key);
    const grewBy = (o.spans?.length ?? 0) - (prev?.spans?.length ?? 0);
    if (!prev || grewBy > 0) {
      addedOverlays.push({
        type: o.type,
        side: o.side,
        spans: prev ? grewBy : (o.spans?.length ?? 0),
      });
    } else if (grewBy < 0) {
      removedOverlays.push({ type: o.type, side: o.side, spans: -grewBy });
    }
  }
  for (const [key, o] of beforeOv) {
    if (!afterOv.has(key)) {
      removedOverlays.push({ type: o.type, side: o.side, spans: o.spans?.length ?? 0 });
    }
  }

  const beforeAnno = new Set((before.detail?.annotations ?? []).map((a) => a.key));
  const afterAnno = new Set((after.detail?.annotations ?? []).map((a) => a.key));
  const addedAnnotations = [...afterAnno].filter((k) => !beforeAnno.has(k));
  const removedAnnotations = [...beforeAnno].filter((k) => !afterAnno.has(k));

  return {
    sourceChanged: before.sourceText !== after.sourceText,
    targetChanged: before.targetText !== after.targetText,
    addedOverlays,
    removedOverlays,
    addedAnnotations,
    removedAnnotations,
  };
}

/**
 * Tool-node count per step for the editor↔trace mapping: 1 for a plain step,
 * branch count for a parallel group.
 */
export function stepToolCounts(steps: { parallel?: unknown[] }[]): number[] {
  return steps.map((s) => (s.parallel && s.parallel.length > 0 ? s.parallel.length : 1));
}
