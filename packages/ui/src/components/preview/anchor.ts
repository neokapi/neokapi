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
import type { Run, Anchor, RunPos } from "./types";

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
