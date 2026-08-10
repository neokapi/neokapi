// runRange — anchor a flat text offset onto a run sequence.
//
// Overlays in the content model are run-anchored: an OverlaySpanView carries a
// RunRange of [startRun,startOffset) … [endRun,endOffset). Detectors and REST
// payloads that report positions over the block's flattened plain text instead
// need the inverse, and so does any host projecting flat annotations into a
// ContentTree the preview kit can render.
//
// The arithmetic mirrors core/model/overlay.go (`runPosition`, `RunRangeFor`,
// `RunRangeForBytes`) and core/model/run.go (`RunsText`) exactly, so a range
// computed here means the same thing as one computed by the engine:
//   - a text run contributes its code points;
//   - an inline-code run (ph / pcOpen / pcClose / sub) has zero text width;
//   - a plural / select run contributes its "other" branch, falling back to the
//     first branch present;
//   - a boundary landing at the end of a text-bearing run is attributed to the
//     start of the following run, so a leading code attaches to the next span.
//
// Go counts runes and indexes bytes; JavaScript indexes UTF-16 code units. Every
// offset here is a CODE POINT offset (Go's rune), and `runRangeForBytes` takes
// the UTF-8 byte offsets a Go producer reports.

import type { Run, RunRange } from "./types";

/** Code-point length of a string (Go's utf8.RuneCountInString). */
function codePointLength(s: string): number {
  let n = 0;
  for (const _ of s) n++;
  return n;
}

/** UTF-8 byte length of one code point. */
function utf8Length(ch: string): number {
  const cp = ch.codePointAt(0) ?? 0;
  if (cp < 0x80) return 1;
  if (cp < 0x800) return 2;
  if (cp < 0x10000) return 3;
  return 4;
}

/** The branch a plural/select run contributes to the flat text. */
function otherBranch(branches: Record<string, Run[]>): Run[] {
  const other = branches.other;
  if (other) return other;
  for (const key of Object.keys(branches)) return branches[key];
  return [];
}

/**
 * The text-only flattening of a run sequence — the string overlay offsets index
 * into. Mirrors model.RunsText: inline codes contribute nothing, plural/select
 * contribute their "other" branch.
 *
 * This is deliberately distinct from `runsText` in renderDoc, which is the
 * render projection and stops at text runs.
 */
export function runsPlainText(runs: Run[] | undefined): string {
  if (!runs) return "";
  let out = "";
  for (const run of runs) {
    if (typeof run.text === "string") out += run.text;
    else if (run.plural) out += runsPlainText(otherBranch(run.plural.forms));
    else if (run.select) out += runsPlainText(otherBranch(run.select.cases));
  }
  return out;
}

/** The code-point width one run contributes to the flat text. */
function runFlatLength(run: Run): number {
  if (typeof run.text === "string") return codePointLength(run.text);
  if (run.plural) return runsFlatLength(otherBranch(run.plural.forms));
  if (run.select) return runsFlatLength(otherBranch(run.select.cases));
  return 0;
}

function runsFlatLength(runs: Run[]): number {
  let n = 0;
  for (const run of runs) n += runFlatLength(run);
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
 * The RunRange covering the half-open code-point span [start, end) of the
 * sequence's flat text. Mirrors model.RunRangeFor.
 */
export function runRangeForChars(runs: Run[] | undefined, start: number, end: number): RunRange {
  const seq = runs ?? [];
  const [startRun, startOffset] = runPosition(seq, start);
  const [endRun, endOffset] = runPosition(seq, end);
  return { startRun, startOffset, endRun, endOffset };
}

/** Code-point offset of a UTF-8 byte offset into `text`. */
function byteToCharOffset(text: string, byteOffset: number): number {
  if (byteOffset <= 0) return 0;
  let bytes = 0;
  let chars = 0;
  for (const ch of text) {
    if (bytes >= byteOffset) return chars;
    bytes += utf8Length(ch);
    chars++;
  }
  return chars;
}

/**
 * The RunRange covering the half-open UTF-8 byte span [byteStart, byteEnd) of
 * the sequence's flat text. Mirrors model.RunRangeForBytes — the conversion a
 * consumer needs for any offset a Go producer reported (entity spans, term
 * matches), which are byte offsets into the block's plain source text.
 */
export function runRangeForBytes(
  runs: Run[] | undefined,
  byteStart: number,
  byteEnd: number,
): RunRange {
  const seq = runs ?? [];
  const text = runsPlainText(seq);
  return runRangeForChars(seq, byteToCharOffset(text, byteStart), byteToCharOffset(text, byteEnd));
}

/**
 * The surface text a UTF-8 byte span covers in the sequence's flat text — the
 * companion of `runRangeForBytes` for a consumer that also wants to show what
 * the span matched, without handling offset conventions itself.
 */
export function textForBytes(runs: Run[] | undefined, byteStart: number, byteEnd: number): string {
  const text = runsPlainText(runs);
  // Array.from splits into code points, which is the unit the offsets count in.
  const chars = Array.from(text);
  return chars.slice(byteToCharOffset(text, byteStart), byteToCharOffset(text, byteEnd)).join("");
}
