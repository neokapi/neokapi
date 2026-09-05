// anchor — put a flat text offset onto a run sequence.
//
// Overlays in the content model are run-anchored: an OverlaySpanView carries an
// Anchor whose half-open span runs from one RunPos to another. Detectors and
// REST payloads that report positions over the block's flattened plain text
// instead need the inverse, and so does any host projecting flat annotations
// into a ContentTree the preview kit can render.
//
// The arithmetic mirrors core/model/anchor.go (`runPosition`, `RangeAnchor`,
// `RangeAnchorForBytes`) and core/model/run.go (`RunsText`) exactly, so an
// anchor computed here means the same thing as one computed by the engine:
//   - a text run contributes its code points;
//   - an inline-code run (ph / pcOpen / pcClose / sub) has zero text width;
//   - a plural / select run contributes its "other" branch, falling back to the
//     first branch present;
//   - a boundary landing at the end of a text-bearing run is attributed to the
//     start of the following run, so a leading code attaches to the next span.
//
// Go counts runes and indexes bytes; JavaScript indexes UTF-16 code units. Every
// offset here is a CODE POINT offset (Go's rune), and `rangeAnchorForBytes` takes
// the UTF-8 byte offsets a Go producer reports.

import { otherBranch, projectRuns, projectRunsText, type RunSpec } from "@neokapi/kapi-format";
import { byteToCharOffset } from "../../lib/offsets";
import { runKind, type Run, type Anchor, type RunPos } from "./types";

/** Code-point length of a string (Go's utf8.RuneCountInString). */
function codePointLength(s: string): number {
  let n = 0;
  for (const _ of s) n++;
  return n;
}

/**
 * The offset domain: the string an overlay range indexes into.
 *
 * This projection is not a reading, it is a coordinate system, and it is
 * pinned to `model.RunsText` on the Go side — an inline code has zero width
 * *there*, so it must have zero width here, or every position computed by the
 * engine lands somewhere else. That is why the codes are `dropped` with a
 * reason and not merely absent: the next person to widen this projection has to
 * read what they would break.
 */
const ZERO_WIDTH = { dropped: "zero text width in model.RunsText, the domain this mirrors" };

const OFFSET_DOMAIN: RunSpec<Run, string> = {
  text: (r) => r.text ?? "",
  ph: ZERO_WIDTH,
  pcOpen: ZERO_WIDTH,
  pcClose: ZERO_WIDTH,
  sub: ZERO_WIDTH,
  plural: { expand: (r) => projectRuns(otherBranch(r.plural?.forms ?? {}), OFFSET_DOMAIN) },
  select: { expand: (r) => projectRuns(otherBranch(r.select?.cases ?? {}), OFFSET_DOMAIN) },
  // Widthless, like every other non-text run in this domain: a marker here
  // would shift every position after it. The reporter still says it was met.
  fallback: () => "",
};

/**
 * The text-only flattening of a run sequence — the string overlay offsets index
 * into. Mirrors model.RunsText: inline codes contribute nothing, plural/select
 * contribute their "other" branch.
 *
 * This is deliberately distinct from `runsText` in renderDoc, which is the
 * document reading: the same characters, but codes carried alongside as chips
 * rather than dropped.
 */
export function runsPlainText(runs: Run[] | undefined): string {
  return projectRunsText(runs, OFFSET_DOMAIN);
}

/** The code-point width one run contributes to the flat text. */
function runFlatLength(run: Run): number {
  return runsFlatLength([run]);
}

function runsFlatLength(runs: Run[]): number {
  let n = 0;
  for (const part of projectRuns(runs, OFFSET_DOMAIN)) n += codePointLength(part);
  return n;
}

/** Locate a code-point offset as a [run index, offset within that run] pair. */
function runPosition(runs: Run[], offset: number): [number, number] {
  if (offset <= 0) return [0, 0];
  let pos = 0;
  for (let i = 0; i < runs.length; i++) {
    const width = runFlatLength(runs[i]);
    if (width === 0) continue;
    if (offset < pos + width) return [i, offset - pos];
    if (offset === pos + width) return [i + 1, 0];
    pos += width;
  }
  return [runs.length, 0];
}

/**
 * The Anchor covering the half-open code-point span [start, end) of the
 * sequence's flat text. Mirrors model.RangeAnchor.
 */
export function rangeAnchorForChars(runs: Run[] | undefined, start: number, end: number): Anchor {
  const seq = runs ?? [];
  const [startRun, startOffset] = runPosition(seq, start);
  const [endRun, endOffset] = runPosition(seq, end);
  return { kind: "range", start: runPos(startRun, startOffset), end: runPos(endRun, endOffset) };
}

/**
 * A boundary in its canonical form. A zero offset is left off, matching the
 * JSON a Go producer writes, so an anchor computed here deep-equals the same
 * anchor arriving over the wire.
 */
function runPos(run: number, offset: number): RunPos {
  return offset === 0 ? { run } : { run, offset };
}

/**
 * A boundary as the wire gives it. Go writes an anchor's `start` and `end`
 * with `omitzero`, so a range beginning at the first run carries no `start`
 * key at all: an absent position is the zero position.
 */
export function runPosOf(pos: RunPos | undefined): RunPos {
  return pos ?? { run: 0 };
}

/**
 * The Anchor covering the half-open UTF-8 byte span [byteStart, byteEnd) of
 * the sequence's flat text. Mirrors model.RangeAnchorForBytes — the conversion a
 * consumer needs for any offset a Go producer reported (entity spans, term
 * matches), which are byte offsets into the block's plain source text.
 */
export function rangeAnchorForBytes(
  runs: Run[] | undefined,
  byteStart: number,
  byteEnd: number,
): Anchor {
  const seq = runs ?? [];
  const text = runsPlainText(seq);
  return rangeAnchorForChars(
    seq,
    byteToCharOffset(text, byteStart),
    byteToCharOffset(text, byteEnd),
  );
}

/**
 * The surface text a UTF-8 byte span covers in the sequence's flat text — the
 * companion of `rangeAnchorForBytes` for a consumer that also wants to show what
 * the span matched, without handling offset conventions itself.
 */
export function textForBytes(runs: Run[] | undefined, byteStart: number, byteEnd: number): string {
  const text = runsPlainText(runs);
  // Array.from splits into code points, which is the unit the offsets count in.
  const chars = Array.from(text);
  return chars.slice(byteToCharOffset(text, byteStart), byteToCharOffset(text, byteEnd)).join("");
}

// ── Anchor → character span ──────────────────────────────────────────────────
//
// The inverse of `rangeAnchorForChars`: where in the flat text an anchor sits,
// so a finding recorded against the runs can be drawn over the text a reader
// sees. Every kind resolves to a half-open span of that text: a block anchor to
// all of it, a range to its characters, a run anchor to the width the run has
// there (zero for an inline code, which a renderer marks as its chip), and a
// form anchor to the plural or select run it names, since the reading shows
// one branch of that run in its place.

/** A half-open span of UTF-16 indices into a flat text. */
export interface CharSpan {
  start: number;
  end: number;
}

/** One hop of an anchor's path. */
type PathStep = NonNullable<Anchor["path"]>[number];

/** UTF-16 index of a code-point offset into `text`. */
function utf16Index(text: string, codePoints: number): number {
  let seen = 0;
  let index = 0;
  for (const ch of text) {
    if (seen >= codePoints) break;
    index += ch.length;
    seen++;
  }
  return index;
}

/** The code-point width of the first `count` runs of a sequence. */
function widthBefore(runs: Run[], count: number): number {
  let n = 0;
  for (let i = 0; i < count && i < runs.length; i++) n += runFlatLength(runs[i]);
  return n;
}

/** The id an inline-code run carries, which a run anchor names it by. */
function runIdOf(run: Run): string | undefined {
  switch (runKind(run)) {
    case "ph":
      return run.ph?.id;
    case "pcOpen":
      return run.pcOpen?.id;
    case "pcClose":
      return run.pcClose?.id;
    case "sub":
      return run.sub?.id;
    default:
      return undefined;
  }
}

/** The branches of a plural or select run, or null for any other kind. */
function branchesOf(run: Run): Record<string, Run[]> | null {
  switch (runKind(run)) {
    case "plural":
      return run.plural?.forms ?? {};
    case "select":
      return run.select?.cases ?? {};
    default:
      return null;
  }
}

/** The branch a path step descends into, or null for an index step. */
function branchKey(step: PathStep): string | null {
  if (typeof step === "number") return null;
  const s = step as { plural?: string; select?: string };
  return s.plural ?? s.select ?? null;
}

/**
 * Code-point offset of a boundary inside `runs`, or null when it lies outside
 * them. A boundary one past the last run is the end of the sequence.
 */
function boundaryOffset(runs: Run[], pos: RunPos): number | null {
  const offset = pos.offset ?? 0;
  if (offset < 0 || pos.run < 0 || pos.run > runs.length) return null;
  if (pos.run === runs.length) return offset === 0 ? widthBefore(runs, runs.length) : null;
  if (offset > runFlatLength(runs[pos.run])) return null;
  return widthBefore(runs, pos.run) + offset;
}

/**
 * The span of `runsPlainText(runs)` an anchor covers, as UTF-16 indices, or
 * null when the anchor addresses nothing in that text: a kind this build does
 * not know, a path that does not resolve, a run the sequence does not hold.
 *
 * A position inside a branch the reading does not show (a plural's `one` form
 * where the text reads `other`) resolves to the whole plural run, which is
 * where that branch would be read if the pivot chose it.
 */
export function charSpanForAnchor(runs: Run[] | undefined, anchor: Anchor): CharSpan | null {
  const top = runs ?? [];
  const span = codePointSpanForAnchor(top, anchor);
  if (!span) return null;
  const text = runsPlainText(top);
  return { start: utf16Index(text, span.start), end: utf16Index(text, span.end) };
}

function codePointSpanForAnchor(top: Run[], anchor: Anchor): CharSpan | null {
  // `seq` is the sequence the path has reached and `base` where it begins in
  // the flat text; `landed` is the run the last index step named.
  let seq = top;
  let base = 0;
  let landed: Run | null = null;
  let landedAt = 0;
  for (const step of anchor.path ?? []) {
    if (typeof step === "number") {
      if (step < 0 || step >= seq.length) return null;
      landed = seq[step];
      landedAt = base + widthBefore(seq, step);
      continue;
    }
    if (landed === null) return null;
    const branches = branchesOf(landed);
    const key = branchKey(step);
    if (!branches || key === null) return null;
    const branch = branches[key];
    if (!branch) return null;
    if (branch !== otherBranch(branches)) {
      return { start: landedAt, end: landedAt + runFlatLength(landed) };
    }
    seq = branch;
    base = landedAt;
    landed = null;
  }

  switch (anchor.kind) {
    case "block":
      return { start: base, end: base + widthBefore(seq, seq.length) };
    case "range": {
      const start = boundaryOffset(seq, runPosOf(anchor.start));
      const end = boundaryOffset(seq, runPosOf(anchor.end));
      if (start === null || end === null || end < start) return null;
      return { start: base + start, end: base + end };
    }
    case "run": {
      if (landed !== null && runIdOf(landed) === anchor.runId) {
        return { start: landedAt, end: landedAt + runFlatLength(landed) };
      }
      const i = seq.findIndex((r) => runIdOf(r) === anchor.runId);
      if (i < 0) return null;
      const at = base + widthBefore(seq, i);
      return { start: at, end: at + runFlatLength(seq[i]) };
    }
    case "form": {
      if (landed === null || branchesOf(landed) === null) return null;
      return { start: landedAt, end: landedAt + runFlatLength(landed) };
    }
    default:
      return null;
  }
}
