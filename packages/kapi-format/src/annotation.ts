/**
 * @neokapi/kapi-format — annotation overlays.
 *
 * Annotations are non-authoritative analytical overlays on a
 * Block/Run graph. They describe properties of content (protected
 * terms, glossary matches, review status, MT confidence, QA flags,
 * translator notes) without changing how the content is stored,
 * edited, or served back. A tool that doesn't understand an
 * annotation type MUST ignore it and process the authoritative
 * content correctly.
 *
 * Annotations live as overlay files on disk under
 * `.kapi/collections/<name>/annotations/<producer-namespace>.overlays.jsonl`
 * (inside a kapi project) or as Session.PutOverlay calls keyed by
 * (kind, blockHash) when running through a BlockStore session.
 *
 * Each `.overlays.jsonl` file is JSON Lines. The first line is a header
 * record; the rest are annotation records. Multiple annotation
 * files can coexist without coordination — different producers own
 * different files.
 *
 * Lifecycle rules:
 *
 *   1. Annotations are non-authoritative. Consumers that don't
 *      understand a type skip its file entirely.
 *   2. Annotations are layered. Producers write independent files;
 *      no cross-file merge semantics.
 *   3. Annotations are derivable. Losing an annotation file costs
 *      only regeneration; the authoritative content in the
 *      `.kbf.json` blocks is unchanged.
 *   4. Annotations can become stale when blocks change. Validators
 *      detect orphans (anchors that no longer resolve) and flag
 *      them. Producers re-run to refresh.
 *   5. Annotations never mutate authoritative content. Producers
 *      are read-only on `documents/`.
 *
 * Inline annotations (markers that wrap a range inside the runs
 * sequence, the way XLIFF 2.0's <mrk> does) are a possible future
 * extension. The current spec is overlay-only.
 */

import type { Block, PluralForm, Run } from "./block.ts";

// ─── Annotation file shape ────────────────────────────────────────

/**
 * The top-level structure of an `annotations/*.overlays.jsonl` file after
 * all lines have been parsed. On disk, `header` is the first line
 * of the file and every line after it is one `Annotation`.
 */
export interface AnnotationFile {
  header: AnnotationFileHeader;
  annotations: Annotation[];
}

/**
 * The header record (first line of the .overlays.jsonl file). Identifies the
 * annotation type, versions it, and records which archive state it
 * was produced against.
 */
export interface AnnotationFileHeader {
  type: "header";
  /**
   * Namespaced annotation type, e.g. `@neokapi/term-detector`,
   * `acme/glossary-v2`, `bowrain/review-status`. No central registry
   * — consumers that understand a namespace consume the file;
   * consumers that don't skip it.
   */
  annotationType: string;
  /** Version of the annotation type's data shape (producer-owned). */
  annotationVersion: string;
  producer: {
    id: string;
    version: string;
  };
  /** ISO 8601 timestamp of when the file was generated. */
  created: string;
  /**
   * SHA-256 of the source archive's manifest.json at the time the
   * annotations were produced. Consumers compare this against the
   * current manifest hash to detect potentially-stale annotations.
   */
  targetArchive: string;
}

/**
 * One annotation record. Subsequent lines in the .overlays.jsonl file.
 */
export interface Annotation {
  type: "annotation";
  /** Stable within the file. Not required to be globally unique. */
  id: string;
  /** The block this annotation is about. */
  block: string;
  anchor: AnnotationAnchor;
  /**
   * Producer-specific payload. The framework imposes no schema on
   * this field; consumers that understand the annotation type know
   * how to read it.
   */
  data: unknown;
}

// ─── Anchor shapes ────────────────────────────────────────────────

/**
 * Where inside a block an annotation sits. Four shapes, discriminated by
 * `kind`.
 *
 * Positions are run-relative rather than offsets into flattened text, so a
 * boundary stays where it was put when a neighbouring run is rewritten and can
 * sit either side of a placeholder. They are pathed, so a position inside a
 * plural form or select case is addressable rather than approximated.
 */
export type AnnotationAnchor = BlockAnchor | RunAnchor | RangeAnchor | FormAnchor;

/**
 * The whole block. Used for block-level metadata like review status, MT
 * confidence, or "this block contains PII".
 */
export interface BlockAnchor {
  kind: "block";
}

/**
 * One `ph`, `pcOpen` or `sub` run. `path` names the run sequence and `runId`
 * the run within it, which is what lets a validator confirm the anchor still
 * points at the run it described.
 */
export interface RunAnchor {
  kind: "run";
  path: RunPath;
  runId: string;
}

/**
 * A half-open span of the run sequence at `path`. Used when a producer finds a
 * substring — a protected term, a URL, a named entity — that no typed run
 * already covers. A span may cross run boundaries, so a phrase running through
 * a bold segment is one range rather than three.
 */
export interface RangeAnchor {
  kind: "range";
  path: RunPath;
  /** Omitted when it is the zero position, which is what the wire form does. */
  start?: RunPos;
  end?: RunPos;
}

/**
 * One plural form or select case — "this 'few' form has been professionally
 * reviewed", "this 'female' case is flagged by QA".
 */
export interface FormAnchor {
  kind: "form";
  /** Path to the containing plural or select run. */
  path: RunPath;
  /**
   * Which form (for plural runs) or case (for select runs). For plural, must
   * be one of `PluralForm`. For select, any key of the run's `cases`.
   */
  key: string;
}

/**
 * A character boundary in a run sequence: an index into the sequence and a
 * rune offset into that run's text. A run carrying no text takes offset 0, and
 * an index equal to the sequence length is the boundary past the last run.
 */
export interface RunPos {
  run: number;
  offset?: number;
}

/**
 * A path through a block's nested runs structure. Empty path refers to
 * `Block.source` itself.
 */
export type RunPath = RunPathStep[];

/**
 * One step in a `RunPath`. Discriminated by shape:
 * - `number` — index into a `Run[]` sequence.
 * - `{ plural: PluralForm }` — step into a `plural` run's form.
 * - `{ select: string }` — step into a `select` run's case.
 */
export type RunPathStep = number | { plural: PluralForm } | { select: string };

// ─── Anchor resolution ────────────────────────────────────────────

/**
 * Result of resolving an anchor against a block. On success the resolved
 * entity; on failure a machine-readable reason.
 */
export type AnchorResolution =
  | { ok: true; kind: "block"; block: Block }
  | { ok: true; kind: "run"; run: Run }
  | { ok: true; kind: "range"; text: string; runs: Run[] }
  | { ok: true; kind: "form"; runs: Run[] }
  | {
      ok: false;
      reason:
        | "block-not-found"
        | "path-out-of-bounds"
        | "path-wrong-kind"
        | "run-id-mismatch"
        | "range-out-of-bounds"
        | "form-not-found";
    };

/**
 * Resolve an anchor against the block it belongs to. Mirrors ResolveAnchor in
 * core/kbf.
 */
export function resolveAnchor(block: Block, anchor: AnnotationAnchor): AnchorResolution {
  if (anchor.kind === "block") {
    return { ok: true, kind: "block", block };
  }

  const walked = walkPath(block.source, anchor.path);
  if (walked === null) return { ok: false, reason: "path-out-of-bounds" };
  const { runs: seq, run: landed } = walked;

  if (anchor.kind === "run") {
    const run = landed ?? seq.find((r) => runIdOf(r) === anchor.runId) ?? null;
    if (run === null) return { ok: false, reason: "run-id-mismatch" };
    const id = runIdOf(run);
    if (id === null) return { ok: false, reason: "path-wrong-kind" };
    if (id !== anchor.runId) return { ok: false, reason: "run-id-mismatch" };
    return { ok: true, kind: "run", run };
  }

  if (anchor.kind === "range") {
    if (!rangeInBounds(seq, anchor)) {
      return { ok: false, reason: "range-out-of-bounds" };
    }
    const runs = extractRuns(seq, anchor);
    return { ok: true, kind: "range", text: runsText(runs), runs };
  }

  // A form belongs to the run the path landed on: a plural run carries no id
  // of its own, so it is addressed by position.
  if (landed === null) return { ok: false, reason: "path-out-of-bounds" };
  if ("plural" in landed) {
    const form = landed.plural.forms[anchor.key as PluralForm];
    if (!form) return { ok: false, reason: "form-not-found" };
    return { ok: true, kind: "form", runs: form };
  }
  if ("select" in landed) {
    const caseRuns = landed.select.cases[anchor.key];
    if (!caseRuns) return { ok: false, reason: "form-not-found" };
    return { ok: true, kind: "form", runs: caseRuns };
  }
  return { ok: false, reason: "path-wrong-kind" };
}

interface WalkResult {
  /** The sequence the path addresses — `topRuns` for an empty path. */
  runs: Run[];
  /** The run an index step last landed on, which a form anchor is about. */
  run: Run | null;
}

/**
 * Walk a path through a run tree. Returns the sequence it addresses and the
 * run it landed on, or null if any step is out of bounds or the wrong kind.
 */
function walkPath(topRuns: Run[], path: RunPath): WalkResult | null {
  let currentRuns: Run[] = topRuns;
  let currentRun: Run | null = null;

  for (const step of path) {
    if (typeof step === "number") {
      if (step < 0 || step >= currentRuns.length) return null;
      currentRun = currentRuns[step];
    } else if ("plural" in step) {
      if (currentRun === null || !("plural" in currentRun)) return null;
      const form = currentRun.plural.forms[step.plural];
      if (!form) return null;
      currentRuns = form;
      currentRun = null;
    } else {
      if (currentRun === null || !("select" in currentRun)) return null;
      const caseRuns = currentRun.select.cases[step.select];
      if (!caseRuns) return null;
      currentRuns = caseRuns;
      currentRun = null;
    }
  }

  return { runs: currentRuns, run: currentRun };
}

/**
 * Rune length of a run's text, or 0 for a run that carries none.
 *
 * Code points, not UTF-16 units: an offset must count the same thing here as
 * it does in the model, or the two sides disagree about where a position is
 * the moment content leaves the Basic Multilingual Plane.
 */
function runTextLength(run: Run): number {
  return "text" in run ? Array.from(run.text).length : 0;
}

/**
 * A boundary as the wire gives it. Go writes `start` and `end` with
 * `omitzero`, so a range beginning at the first run carries no `start` key at
 * all: an absent position is the zero position, not a malformed anchor. The
 * resolver reads annotation files off disk, where no type is policing them, so
 * it takes them as they come.
 */
function posOf(pos: RunPos | undefined): RunPos {
  return pos ?? { run: 0 };
}

function offsetInBounds(runs: Run[], pos: RunPos): boolean {
  const offset = pos.offset ?? 0;
  if (offset < 0) return false;
  if (pos.run === runs.length) return offset === 0;
  if (pos.run < 0 || pos.run > runs.length) return false;
  const run = runs[pos.run];
  return "text" in run ? offset <= runTextLength(run) : offset === 0;
}

function rangeInBounds(runs: Run[], anchor: RangeAnchor): boolean {
  const start = posOf(anchor.start);
  const end = posOf(anchor.end);
  if (start.run < 0 || end.run < start.run) return false;
  if (end.run > runs.length) return false;
  return offsetInBounds(runs, start) && offsetInBounds(runs, end);
}

/**
 * The sub-sequence a range covers. Boundary text runs are split at their
 * offsets; a run carrying no text is atomic and comes through whole unless it
 * sits on the exclusive end.
 */
function extractRuns(runs: Run[], anchor: RangeAnchor): Run[] {
  const out: Run[] = [];
  const start = posOf(anchor.start);
  const end = posOf(anchor.end);
  const startOffset = start.offset ?? 0;
  const endOffset = end.offset ?? 0;

  for (let i = start.run; i <= end.run && i < runs.length; i++) {
    const run = runs[i];
    if ("text" in run) {
      const chars = Array.from(run.text);
      const from = i === start.run ? Math.min(startOffset, chars.length) : 0;
      const to = i === end.run ? Math.min(endOffset, chars.length) : chars.length;
      if (from < to) out.push({ text: chars.slice(from, to).join("") });
      continue;
    }
    if (i === end.run && endOffset === 0) continue;
    out.push(run);
  }
  return out;
}

function runsText(runs: Run[]): string {
  return runs.map((r) => ("text" in r ? r.text : "")).join("");
}

function runIdOf(run: Run): string | null {
  if ("ph" in run) return run.ph.id;
  if ("pcOpen" in run) return run.pcOpen.id;
  if ("sub" in run) return run.sub.id;
  return null;
}

// ─── Validation utilities ────────────────────────────────────────

/**
 * Check an annotation's anchor against a block and return an
 * error if it doesn't resolve. Intended for orphan detection when
 * a producer's annotations are loaded after the block they
 * reference has changed.
 */
export function validateAnchor(
  block: Block,
  annotation: Annotation,
): AnnotationValidationError | null {
  // The record names the block it is about, so validating it against one that
  // is not that block is a mismatch rather than a resolution failure.
  if (annotation.block !== block.id) {
    return {
      annotationId: annotation.id,
      blockId: annotation.block,
      reason: "block-not-found",
      message: messageFor("block-not-found", annotation),
    };
  }
  const result = resolveAnchor(block, annotation.anchor);
  if (result.ok) return null;
  return {
    annotationId: annotation.id,
    blockId: block.id,
    reason: result.reason,
    message: messageFor(result.reason, annotation),
  };
}

function messageFor(
  reason: (AnchorResolution & { ok: false })["reason"],
  annotation: Annotation,
): string {
  switch (reason) {
    case "block-not-found":
      return `annotation "${annotation.id}" targets block "${annotation.block}" which does not match`;
    case "path-out-of-bounds":
      return `annotation "${annotation.id}" path is out of bounds in block "${annotation.block}"`;
    case "path-wrong-kind":
      return `annotation "${annotation.id}" path lands on a run of the wrong kind for its anchor`;
    case "run-id-mismatch":
      return `annotation "${annotation.id}" resolves to a run whose id does not match the recorded id (possible orphan)`;
    case "range-out-of-bounds":
      return `annotation "${annotation.id}" range is out of bounds in the run sequence it addresses`;
    case "form-not-found":
      return `annotation "${annotation.id}" targets a plural form or select case that does not exist on the block`;
  }
}

export interface AnnotationValidationError {
  annotationId: string;
  blockId: string;
  reason: (AnchorResolution & { ok: false })["reason"];
  message: string;
}
