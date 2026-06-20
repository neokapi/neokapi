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

export type OverlayType =
  | "segmentation"
  | "term"
  | "entity"
  | "qa"
  | "alignment"
  | "redaction"
  | "tm";

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

/**
 * The block's structural role (the WS1 structural layer): what the block IS in
 * the document — a heading (with level), a list item, a table cell, furniture
 * (running header/footer) — and its reading order. Mirrors editor.StructureView.
 */
export interface StructureView {
  /** Normalized semantic role: heading | title | paragraph | list-item | table-cell | caption | … */
  role?: string;
  /** Layout layer / plane: body | furniture | background | overlay | metadata. */
  layer?: string;
  /** Presence condition: "" (visible) | conditional | hidden | print-only | screen-only. */
  visibility?: string;
  /** Heading / nesting level (1–9), 0 when not applicable. */
  level?: number;
  /** Explicit reading-order index when the source provides one. */
  readingOrder?: number;
}

/** A cross-block edge (caption-of / footnote-of / label-for / triggers /
 * references). Mirrors editor.RelationView. */
export interface RelationView {
  type: string;
  target: string;
}

/**
 * Where the block sits on a rendered page (the WS1 structural layer): page
 * number + bounding box in the coordinate space named by origin/resolution.
 * Mirrors editor.GeometryView. Present only for layout-aware sources (PDF,
 * Docling/DocLang, slide/sheet coordinates).
 */
export interface GeometryView {
  /** 1-based page number; 0 = unpaginated/unknown. */
  page?: number;
  x: number;
  y: number;
  w: number;
  h: number;
  /** Edge length of the normalized coordinate grid (DocLang uses 512); 0 = absolute units. */
  resolution?: number;
  /** "top-left" (default) or "bottom-left". */
  origin?: string;
  /** Stacking order within the plane (higher = nearer the viewer); 0 = base. */
  z?: number;
  /** Optional per-character boxes within the block (same coord space as x/y/w/h). */
  glyphs?: GlyphView[];
}

/** One character's text and box (GeometryView.glyphs). Mirrors editor.GlyphView. */
export interface GlyphView {
  text?: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * A block's temporal anchor on a time-based source (subtitles, audio, video):
 * the [startMs, endMs) span it is spoken/shown over, plus an optional source ref
 * (the cue id in the originating file). Mirrors editor.TimingView. Present only
 * for timed sources (SRT/VTT/TTML, audio/video transcripts).
 */
export interface TimingView {
  /** Cue start, milliseconds from the media origin. */
  startMs: number;
  /** Cue end, milliseconds from the media origin. */
  endMs: number;
  /** The cue id / reference in the source file, when one exists. */
  sourceRef?: string;
}

/**
 * Structured descriptor of the binary media a node carries or refers to (the
 * image being OCR'd, the audio/video being subtitled). Mirrors editor.MediaView.
 * `uri` is the default resolvable location; `blobKey`/`hasData` let a host
 * resolve bytes it holds out-of-band (a desktop content store, a wasm blob map).
 */
export interface MediaView {
  /** MIME type (e.g. "image/png", "audio/mpeg", "video/mp4"). */
  mimeType?: string;
  /** Original file name, when known. */
  filename?: string;
  /** A directly resolvable URL/URI for the media, when the source provides one. */
  uri?: string;
  /** An opaque key a host can use to look the bytes up in its own store. */
  blobKey?: string;
  /** True when the engine is holding the bytes inline (resolve via the host). */
  hasData?: boolean;
  /** Byte size, when known. */
  size?: number;
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
  /** The structural role layer (WS1): role, layout layer, level, reading order. */
  structure?: StructureView;
  /** Page geometry (WS1): page + bounding box, for layout-aware sources. */
  geometry?: GeometryView;
  /** Temporal anchor (WS1): [startMs, endMs) cue span, for timed sources. */
  timing?: TimingView;
  /** Cross-block relationship edges (caption-of / footnote-of / …). */
  relations?: RelationView[];
  hasSkeleton?: boolean;
  isReferent?: boolean;
  preserveWhitespace?: boolean;
  /** Content-addressable identity hash, when present. */
  identity?: string;
  // data / media
  /** Legacy/flat MIME hint on a data or media node. Prefer the structured `media`. */
  mimeType?: string;
  /** Structured media descriptor (mime, filename, uri, blob key) for a media node. */
  media?: MediaView;
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
