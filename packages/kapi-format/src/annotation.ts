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
 * `.kapi/collections/<name>/annotations/<producer-namespace>.klfl`
 * (inside a kapi project) or as Session.PutOverlay calls keyed by
 * (kind, blockHash) when running through a BlockStore session.
 *
 * Each `.klfl` file is JSON Lines. The first line is a header
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
 *      `.klf` blocks is unchanged.
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
 * The top-level structure of an `annotations/*.klfl` file after
 * all lines have been parsed. On disk, `header` is the first line
 * of the file and every line after it is one `Annotation`.
 */
export interface AnnotationFile {
  header: AnnotationFileHeader;
  annotations: Annotation[];
}

/**
 * The header record (first line of the .klfl file). Identifies the
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
 * One annotation record. Subsequent lines in the .klfl file.
 */
export interface Annotation {
  type: "annotation";
  /** Stable within the file. Not required to be globally unique. */
  id: string;
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
 * Where an annotation is attached. Four shapes, discriminated by
 * `kind`.
 */
export type AnnotationAnchor = BlockAnchor | RunAnchor | RangeAnchor | FormAnchor;

/**
 * Attach to a whole block. Used for block-level metadata like
 * review status, MT confidence, or "this block contains PII".
 */
export interface BlockAnchor {
  kind: "block";
  block: string;
}

/**
 * Attach to a specific `ph`, `pcOpen`, or `sub` run within a
 * block. Used for run-scoped metadata like "this placeholder is
 * a brand name" or "this inline link is a glossary match".
 *
 * `path` names the location of the run within the block's nested
 * runs structure. `runId` is the target run's `id` field, used by
 * validators to confirm the anchor still points at the run it
 * originally described.
 */
export interface RunAnchor {
  kind: "run";
  block: string;
  path: RunPath;
  runId: string;
}

/**
 * Attach to a character range inside a text run. Used when a
 * post-processor finds a substring (a protected term, a URL, a
 * named entity) that doesn't correspond to an existing typed run.
 *
 * Character offsets are UTF-16 code units into the target text
 * run's `text` field. Range anchors are fragile under re-extraction
 * because text content can change; validators re-resolve them
 * each time the block changes.
 */
export interface RangeAnchor {
  kind: "range";
  block: string;
  path: RunPath;
  offset: number;
  length: number;
}

/**
 * Attach to a specific plural form or select case within a block.
 * Used when an annotation applies to one form of a group but not
 * others — "this 'few' form has been professionally reviewed",
 * "this 'female' case is flagged by QA".
 */
export interface FormAnchor {
  kind: "form";
  block: string;
  /** Path to the containing plural or select run. */
  path: RunPath;
  /**
   * Which form (for plural runs) or case (for select runs). For
   * plural, must be one of `PluralForm`. For select, any string
   * matching a key in the select run's `cases`.
   */
  key: string;
}

/**
 * A path through a block's nested runs structure. Empty path
 * refers to `Block.source` itself.
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
 * Result of resolving an annotation anchor against a block. On
 * success, `target` is the resolved entity; on failure, `reason`
 * is a machine-readable tag indicating why.
 */
export type AnchorResolution =
  | { ok: true; kind: "block"; block: Block }
  | { ok: true; kind: "run"; run: Run }
  | { ok: true; kind: "range"; text: string; offset: number; length: number }
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
 * Resolve an annotation anchor against the target block. Returns
 * either the resolved entity or a machine-readable failure reason
 * suitable for orphan-detection validators.
 */
export function resolveAnchor(block: Block, anchor: AnnotationAnchor): AnchorResolution {
  if (anchor.block !== block.id) {
    return { ok: false, reason: "block-not-found" };
  }

  if (anchor.kind === "block") {
    return { ok: true, kind: "block", block };
  }

  // Walk the path except the last step, then handle the final
  // step according to the anchor kind.
  const runs = walkPath(block.source, anchor.path);
  if (runs === null) return { ok: false, reason: "path-out-of-bounds" };

  if (anchor.kind === "run") {
    const run = runs.run;
    if (run === null) return { ok: false, reason: "path-out-of-bounds" };
    const id = runIdOf(run);
    if (id === null) return { ok: false, reason: "path-wrong-kind" };
    if (id !== anchor.runId) return { ok: false, reason: "run-id-mismatch" };
    return { ok: true, kind: "run", run };
  }

  if (anchor.kind === "range") {
    const run = runs.run;
    if (run === null || !("text" in run)) {
      return { ok: false, reason: "path-wrong-kind" };
    }
    if (anchor.offset < 0 || anchor.offset + anchor.length > run.text.length) {
      return { ok: false, reason: "range-out-of-bounds" };
    }
    return {
      ok: true,
      kind: "range",
      text: run.text,
      offset: anchor.offset,
      length: anchor.length,
    };
  }

  // FormAnchor — the path points at a plural/select run; step into
  // its matching form/case.
  const run = runs.run;
  if (run === null) return { ok: false, reason: "path-out-of-bounds" };
  if ("plural" in run) {
    const form = run.plural.forms[anchor.key as PluralForm];
    if (!form) return { ok: false, reason: "form-not-found" };
    return { ok: true, kind: "form", runs: form };
  }
  if ("select" in run) {
    const caseRuns = run.select.cases[anchor.key];
    if (!caseRuns) return { ok: false, reason: "form-not-found" };
    return { ok: true, kind: "form", runs: caseRuns };
  }
  return { ok: false, reason: "path-wrong-kind" };
}

interface WalkResult {
  /** The run at the end of the path, or null if the path is empty. */
  run: Run | null;
}

/**
 * Walk a path through a run tree. Returns the run the path lands
 * on, or null if any step is out of bounds / wrong kind.
 */
function walkPath(topRuns: Run[], path: RunPath): WalkResult | null {
  if (path.length === 0) {
    return { run: null };
  }

  let currentRuns: Run[] = topRuns;
  let currentRun: Run | null = null;

  for (let i = 0; i < path.length; i++) {
    const step = path[i];
    if (typeof step === "number") {
      if (step < 0 || step >= currentRuns.length) return null;
      currentRun = currentRuns[step];
      // The next step, if any, will be stepping into the current
      // run (a plural / select) or indexing into the current run's
      // children, which only makes sense for plural/select runs.
      // currentRuns is updated at the next iteration only if the
      // next step is a plural / select descent.
    } else if ("plural" in step) {
      if (currentRun === null || !("plural" in currentRun)) return null;
      const form = currentRun.plural.forms[step.plural];
      if (!form) return null;
      currentRuns = form;
      currentRun = null;
    } else {
      // { select: string }
      if (currentRun === null || !("select" in currentRun)) return null;
      const caseRuns = currentRun.select.cases[step.select];
      if (!caseRuns) return null;
      currentRuns = caseRuns;
      currentRun = null;
    }
  }

  return { run: currentRun };
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
      return `annotation "${annotation.id}" targets block "${annotation.anchor.block}" which does not match`;
    case "path-out-of-bounds":
      return `annotation "${annotation.id}" path is out of bounds in block "${annotation.anchor.block}"`;
    case "path-wrong-kind":
      return `annotation "${annotation.id}" path lands on a run of the wrong kind for its anchor`;
    case "run-id-mismatch":
      return `annotation "${annotation.id}" resolves to a run whose id does not match the recorded id (possible orphan)`;
    case "range-out-of-bounds":
      return `annotation "${annotation.id}" character range exceeds the target text run`;
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
