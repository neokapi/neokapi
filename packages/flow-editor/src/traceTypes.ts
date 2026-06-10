// Trace types matching the Go flow.TraceEvent / flow.FlowTrace structures.

export interface TraceEvent {
  ts: number; // microseconds from flow start
  type: string; // "enter", "exit", "error"
  nodeId: string;
  partId?: string;
  meta?: Record<string, unknown>;
}

/** One overlay span projected to rune offsets in the side's flattened text. */
export interface SpanSnapshot {
  start: number;
  end: number;
  /** The covered text (clipped). */
  text?: string;
  /** Compact payload summary — entity type, term concept, QA message, … */
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
  hasSkeleton?: boolean;
  overlays?: OverlaySnapshot[];
  annotations?: AnnotationSnapshot[];
}

export interface PartSnapshot {
  id: string;
  type: string; // "Block", "Data", "Media", etc.
  summary: string;
  sourceText?: string;
  targetText?: string;
  detail?: PartDetail;
}

export interface PartSnapshotSet {
  initial: PartSnapshot;
  afterNode?: Record<string, PartSnapshot>;
}

export interface FlowTrace {
  name: string;
  description?: string;
  nodes: TraceNode[];
  events: TraceEvent[];
  parts: Record<string, PartSnapshotSet>;
  durationUs: number;
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
    if (!stats.has(evt.nodeId)) {
      stats.set(evt.nodeId, {
        nodeId: evt.nodeId,
        partsProcessed: 0,
        durationUs: 0,
        hasError: false,
      });
    }
    const s = stats.get(evt.nodeId)!;

    if (evt.type === "enter") {
      enterTimes.set(`${evt.nodeId}:${evt.partId}`, evt.ts);
    } else if (evt.type === "exit") {
      s.partsProcessed++;
      const key = `${evt.nodeId}:${evt.partId}`;
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
