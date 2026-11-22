// Trace types matching the Go flow.TraceEvent / flow.FlowTrace structures.

export interface TraceEvent {
  ts: number; // microseconds from flow start
  type: string; // "enter", "exit", "error"
  node_id: string;
  part_id?: string;
  meta?: Record<string, unknown>;
}

/** One overlay span projected to rune offsets in the side's flattened text. */
export interface SpanSnapshot {
  start: number;
  end: number;
  /** The covered text (clipped). */
  text?: string;
  /** Compact payload summary — entity type, term concept, check message, … */
  note?: string;
}

/** One stand-off overlay (AD-002) summarized at snapshot time. */
export interface OverlaySnapshot {
  type: string; // "segmentation", "term", "entity", "qa", …
  side: string; // "source" or a target variant key
  layer?: string;
  spans?: SpanSnapshot[];
}

/** One block-scoped annotation (key + compact summary). */
export interface AnnotationSnapshot {
  key: string;
  summary?: string;
}

/** Full block detail at a point in time (run-native; overlays + annotations). */
export interface PartDetail {
  name?: string;
  translatable?: boolean;
  properties?: Record<string, string>;
  has_skeleton?: boolean;
  overlays?: OverlaySnapshot[];
  annotations?: AnnotationSnapshot[];
}

export interface PartSnapshot {
  id: string;
  type: string; // "Block", "Data", "Media", etc.
  summary: string;
  source_text?: string;
  target_text?: string;
  detail?: PartDetail;
}

export interface PartSnapshotSet {
  initial: PartSnapshot;
  after_node?: Record<string, PartSnapshot>;
}

export interface FlowTrace {
  name: string;
  description?: string;
  nodes: TraceNode[];
  events: TraceEvent[];
  parts: Record<string, PartSnapshotSet>;
  duration_us: number;
  /** Set when a recording budget dropped parts: the trace is the first parts of the run. */
  truncated?: boolean;
}

export interface TraceNode {
  id: string;
  type: string;
  name: string;
  label?: string;
}

/** Per-node aggregated stats computed from trace events. */
export interface NodeTraceStats {
  nodeId: string;
  partsProcessed: number;
  durationUs: number;
  hasError: boolean;
  errorMessage?: string;
}

/** Compute per-node stats from trace events. */
export function computeNodeStats(events: TraceEvent[]): Map<string, NodeTraceStats> {
  const stats = new Map<string, NodeTraceStats>();

  // Track enter timestamps per partId per nodeId for duration calculation.
  const enterTimes = new Map<string, number>(); // key: `${nodeId}:${partId}`

  for (const evt of events) {
    if (!stats.has(evt.node_id)) {
      stats.set(evt.node_id, {
        nodeId: evt.node_id,
        partsProcessed: 0,
        durationUs: 0,
        hasError: false,
      });
    }
    const s = stats.get(evt.node_id)!;

    if (evt.type === "enter") {
      enterTimes.set(`${evt.node_id}:${evt.part_id}`, evt.ts);
    } else if (evt.type === "exit") {
      s.partsProcessed++;
      const key = `${evt.node_id}:${evt.part_id}`;
      const enterTs = enterTimes.get(key);
      if (enterTs !== undefined) {
        s.durationUs += evt.ts - enterTs;
        enterTimes.delete(key);
      }
    } else if (evt.type === "error") {
      s.hasError = true;
      s.errorMessage = evt.meta?.error as string;
    }
  }

  return stats;
}
