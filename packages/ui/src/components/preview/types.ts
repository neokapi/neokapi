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

// ── Rich block detail (mirrors the extended anatomy serializer) ──────────────
// These optional fields let the Block inspector show everything the content
// model carries — committed-target provenance, stand-off overlays and
// block-level annotations — not just the run sequences. They are omitted by
// older engine builds, so every consumer treats them as optional.

/** Provenance of a committed translation (mirrors model.Origin). */
export interface Origin {
  kind?: string; // human | tm | mt | ai
  engine?: string;
  tool?: string;
  reference?: string;
  timestamp?: string;
}

/** Lifecycle + provenance metadata for one target variant. */
export interface TargetMeta {
  status?: string; // "" | draft | translated | reviewed | signed-off
  score?: number;
  origin?: Origin;
  tone?: string;
  channel?: string;
}

export interface RunRange {
  startRun: number;
  startOffset: number;
  endRun: number;
  endOffset: number;
}

/** One run-anchored span within an overlay, with its extracted text. */
export interface OverlaySpan {
  id?: string;
  range: RunRange;
  props?: Record<string, string>;
  /** The source/target text the span covers (extracted by the engine). */
  text?: string;
  ignorable?: boolean;
}

export type OverlayType = "segmentation" | "term" | "entity" | "qa" | "alignment" | "redaction";

/** A typed stand-off interpretation layered over one side of a block. */
export interface OverlayView {
  type: OverlayType;
  /** "source" or the target variant key (e.g. "fr-FR"). */
  side: string;
  layer?: string;
  spans: OverlaySpan[];
}

/** A block-level annotation (alt-translation, note, or generic). */
export interface AnnotationView {
  key: string;
  type: string; // alt-translation | note | <generic kind>
  summary?: string;
  fields?: Record<string, unknown>;
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
  type?: string;
  translatable?: boolean;
  sourceLocale?: string;
  source?: Run[];
  targets?: Record<string, Run[]>;
  /** Per-variant lifecycle + provenance, keyed like `targets`. */
  targetMeta?: Record<string, TargetMeta>;
  segments?: SegmentSpan[];
  /** Stand-off overlays (terms, entities, QA, alignment, extra segmentation). */
  overlays?: OverlayView[];
  /** Block-level annotations. */
  annotations?: AnnotationView[];
  hasSkeleton?: boolean;
  isReferent?: boolean;
  preserveWhitespace?: boolean;
  /** Content-addressable identity hash, when present. */
  identity?: string;
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
