// Canonical lab data contracts, mirroring the Go wire types:
//   - FlowTrace  ← core/flow/trace.go        (the --trace / runWithTrace output)
//   - ContentTree ← core/editor/anatomy.go    (the labInspect output)
//
// These are the single source of truth the lab explorers consume; the
// flow-visualization page and any other host import them from here.

// ── FlowTrace (pipeline tracing) ─────────────────────────────────────────────

export interface FlowNode {
  id: string;
  type: "reader" | "tool" | "writer" | "bridge-reader" | "bridge-writer";
  name: string;
  label: string;
  /** Parallel worker count (e.g. 3 for --parallel-blocks 3). */
  concurrency?: number;
  bridge?: {
    filterClass: string;
    protocol: "grpc" | "netrpc";
    subprocess?: string;
  };
}

export type TraceEventType =
  | "enter"
  | "exit"
  | "bridge-serialize"
  | "bridge-deserialize"
  | "bridge-send"
  | "bridge-receive"
  | "pool-acquire"
  | "pool-release"
  | "jvm-start"
  | "jvm-ready"
  | "grpc-open"
  | "grpc-read-start"
  | "grpc-read-end"
  | "grpc-write-start"
  | "grpc-write-end";

export interface TraceEvent {
  ts: number;
  type: TraceEventType;
  nodeId: string;
  partId?: string;
  meta?: Record<string, unknown>;
}

export interface PartSnapshot {
  id: string;
  type: "LayerStart" | "LayerEnd" | "Block" | "Data" | "Media" | "GroupStart" | "GroupEnd";
  summary: string;
  sourceText?: string;
  targetText?: string;
  /** Full part structure at this point — run sequences, every locale, properties. */
  detail?: PartDetail;
}

/** The run-native, full view of a Block at a point in time (mirrors Go PartDetail). */
export interface PartDetail {
  name?: string;
  translatable?: boolean;
  source?: Run[];
  targets?: Record<string, Run[]>;
  properties?: Record<string, string>;
  hasSkeleton?: boolean;
}

export interface PartSnapshotSet {
  initial: PartSnapshot;
  afterNode?: Record<string, PartSnapshot>;
}

export interface TraceFile {
  name: string;
  format?: string;
  preview: string;
}

export interface FlowTrace {
  name: string;
  description: string;
  nodes: FlowNode[];
  channelSize: number;
  events: TraceEvent[];
  parts: Record<string, PartSnapshotSet>;
  inputFile: TraceFile;
  outputFile: TraceFile;
  durationUs: number;
}

// ── Particle (player animation) ──────────────────────────────────────────────

export interface Particle {
  partId: string;
  partType: string;
  position: "edge" | "node";
  nodeId?: string;
  edgeIndex?: number;
  progress?: number;
  summary: string;
  /** Worker lane (0-indexed) for concurrent nodes. */
  worker?: number;
}

// ── ContentTree (anatomy) ────────────────────────────────────────────────────
//
// Mirrors core/editor.ContentTree. The Block atom is a flat run sequence
// (Source/Targets); Segments is a secondary run-index overlay.

export type RunKind = "text" | "ph" | "pcOpen" | "pcClose" | "sub" | "plural" | "select";

export interface CodeRun {
  id: string;
  type?: string;
  subType?: string;
  data?: string;
  equiv?: string;
  disp?: string;
}
export interface SubRun {
  id: string;
  ref: string;
  equiv?: string;
}
export interface PluralRun {
  pivot: string;
  forms: Record<string, Run[]>;
}
export interface SelectRun {
  pivot: string;
  cases: Record<string, Run[]>;
}

/**
 * A run is an object with exactly one discriminator key (RFC 0001). Text runs
 * serialize flat — `{"text":"literal"}` — per Framework AD-002, so `text` is a
 * plain string; every other kind nests its struct under its discriminator key.
 */
export interface Run {
  text?: string;
  ph?: CodeRun;
  pcOpen?: CodeRun;
  pcClose?: CodeRun;
  sub?: SubRun;
  plural?: PluralRun;
  select?: SelectRun;
}

export interface SegmentSpan {
  id: string;
  start: number;
  end: number;
}

export type ContentNodeKind = "layer" | "group" | "block" | "data" | "media";

export interface ContentNode {
  kind: ContentNodeKind;
  id: string;
  name?: string;
  properties?: Record<string, string>;
  // layer
  format?: string;
  locale?: string;
  parentId?: string;
  // block
  translatable?: boolean;
  source?: Run[];
  targets?: Record<string, Run[]>;
  segments?: SegmentSpan[];
  hasSkeleton?: boolean;
  // data / media
  mimeType?: string;
  summary?: string;
  // containers
  children?: ContentNode[];
}

export interface ContentStats {
  layers: number;
  groups: number;
  blocks: number;
  data: number;
  media: number;
  runs: number;
}

export interface ContentTree {
  format: string;
  root: ContentNode[];
  stats: ContentStats;
}

/** The discriminator key of a run, or "text" as a safe default. */
export function runKind(run: Run): RunKind {
  if (run.ph) return "ph";
  if (run.pcOpen) return "pcOpen";
  if (run.pcClose) return "pcClose";
  if (run.sub) return "sub";
  if (run.plural) return "plural";
  if (run.select) return "select";
  return "text";
}
