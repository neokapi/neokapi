// Lab data contracts consumed by the explorers and the flow-visualization
// page:
//   - FlowTrace  ← core/flow/trace.go        (the --trace / runWithTrace output)
//   - ContentTree ← core/editor/anatomy.go    (the labInspect output)
//
// The ContentTree family is generated from Go into @neokapi/contract-types
// (AD-034) and re-exported/refined below — see the section comment. FlowTrace
// stays a local mirror. Both are projections of the content model, not wire
// contracts; hosts import them from here.

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
// The ContentTree family is no longer hand-mirrored here: the structural
// envelope types are generated from the Go source of truth into
// @neokapi/contract-types (content.gen.ts, via scripts/gen-contract-types —
// AD-034) and re-exported below. ContentTree is a labeled PROJECTION of the
// content model, not a wire contract; its runs use the model's canonical Run
// JSON (RFC 0001).
//
// Where this file intentionally diverges from the generated reality, it keeps
// a local refinement built ON TOP of the generated type (Omit + extend), so
// field drift still propagates from Go:
//   - `Run`/`CodeRun`: the local Run keeps the historical loose shape (an
//     interface with all-optional discriminator keys, and a merged `CodeRun`
//     for ph/pcOpen/pcClose with most fields optional). The generated
//     contract-types `Run` is the strict union with required per-kind fields;
//     the many existing fixtures and the coded-text bridge author runs against
//     the loose shape. TODO(#817/AD-034): migrate consumers to the strict
//     generated `Run` union and drop the local one.
//   - `OverlayType` / `ContentNodeKind`: hand string-literal narrowings of
//     fields the Go structs type as plain strings.
//   - `OverlayView.spans`: kept required (the Go tag is omitempty, so the
//     engine may omit it; existing consumers assume presence — unchanged).

import type {
  Anchor as GenAnchor,
  AnnotationView as GenAnnotationView,
  ContentNode as GenContentNode,
  ContentStats as GenContentStats,
  ContentTree as GenContentTree,
  GeometryAnnotation,
  GeometryView as GenGeometryView,
  GlyphView as GenGlyphView,
  MediaView as GenMediaView,
  OriginView,
  OverlaySpanView,
  OverlayView as GenOverlayView,
  RelationView as GenRelationView,
  RenderNode as GenRenderNode,
  RunPos as GenRunPos,
  SegmentSpan as GenSegmentSpan,
  StructureView as GenStructureView,
  TargetMeta as GenTargetMeta,
  TimingView as GenTimingView,
} from "@neokapi/contract-types";

// Exact matches: pure re-exports (some under their historical local names).
export type ContentStats = GenContentStats;
export type SegmentSpan = GenSegmentSpan;
export type TargetMeta = GenTargetMeta;
export type Origin = OriginView;
export type Anchor = GenAnchor;
export type RunPos = GenRunPos;
export type OverlaySpan = OverlaySpanView;
export type AnnotationView = GenAnnotationView;
export type StructureView = GenStructureView;
export type RelationView = GenRelationView;
export type GeometryView = GenGeometryView;
export type GlyphView = GenGlyphView;
export type TimingView = GenTimingView;
export type MediaView = GenMediaView;
/** Page geometry on a render node — generated model.GeometryAnnotation. */
export type RenderGeometry = GeometryAnnotation;

export type RunKind = "text" | "ph" | "pcOpen" | "pcClose" | "sub" | "plural" | "select";

/**
 * Merged loose shape for ph/pcOpen/pcClose payloads (see the divergence note
 * above; the strict per-kind interfaces are PlaceholderRun / PcOpenRun /
 * PcCloseRun in @neokapi/contract-types).
 */
export interface CodeRun {
  id: string;
  type?: string;
  subType?: string;
  data?: string;
  equiv?: string;
  disp?: string;
  /**
   * Canonical, format-neutral attributes for link/image runs (mirrors the Go
   * PcOpenRun/PlaceholderRun Attrs map): `href`, `src`, `alt`, `title`. Required
   * to project a hyperlink/image into the preview's semantic markup — without it
   * a link renders as bare text. Omitted by older engine builds.
   */
  attrs?: Record<string, string>;
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
 * Loose local mirror (see the divergence note above); the strict generated
 * union is `Run` in @neokapi/contract-types.
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

export type OverlayType =
  | "segmentation"
  | "term"
  | "entity"
  | "qa"
  | "alignment"
  | "redaction"
  | "tm";

/**
 * A typed stand-off interpretation layered over one side of a block.
 * Refinement of the generated OverlayView: `type` narrowed to OverlayType,
 * `spans` kept required (see the divergence note above).
 */
export interface OverlayView extends Omit<GenOverlayView, "type" | "spans"> {
  type: OverlayType;
  spans: OverlaySpan[];
}

export type ContentNodeKind = "layer" | "group" | "block" | "data" | "media";

/**
 * One node in a ContentTree. Refinement of the generated ContentNode: `kind`
 * narrowed to ContentNodeKind, runs/overlays/children repointed at the local
 * loose Run and refined OverlayView/ContentNode.
 */
export interface ContentNode extends Omit<
  GenContentNode,
  "kind" | "source" | "targets" | "overlays" | "children"
> {
  kind: ContentNodeKind;
  source?: Run[];
  targets?: Record<string, Run[]>;
  overlays?: OverlayView[];
  children?: ContentNode[];
}

/**
 * RenderNode is the normalized generative-projection render AST built in Go
 * (core/projection) and shipped on ContentTree.render. Refinement of the
 * generated RenderNode: runs/children repointed at the local loose Run and
 * this recursive type.
 */
export interface RenderNode extends Omit<GenRenderNode, "runs" | "children"> {
  runs?: Run[];
  children?: RenderNode[];
}

/**
 * The hierarchical content-model view (the labInspect output) — a labeled
 * projection (AD-034). Refinement of the generated ContentTree over the
 * refined ContentNode/RenderNode.
 */
export interface ContentTree extends Omit<GenContentTree, "root" | "render"> {
  root: ContentNode[];
  render?: RenderNode;
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
