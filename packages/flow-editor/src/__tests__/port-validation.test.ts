import { describe, it, expect } from "vitest";
import type { IOPort } from "../types";

/**
 * port connection validation — mirrors the isValidConnection callback in
 * FlowEditor. A connection is meaningful when the source produces at least one
 * port the target consumes (matched by type@side). Missing metadata, or a
 * target that consumes nothing (a pass-through), is permitted.
 */
function isPortConnectionValid(
  srcProduces: IOPort[] | undefined,
  tgtConsumes: IOPort[] | undefined,
): boolean {
  if (!srcProduces || !tgtConsumes || tgtConsumes.length === 0) return true;
  const produced = new Set(srcProduces.map((f) => `${f.type}@${f.side ?? "source"}`));
  return tgtConsumes.some((c) => produced.has(`${c.type}@${c.side ?? "source"}`));
}

const tgt = (type: string): IOPort => ({ type, side: "target" });
const src = (type: string): IOPort => ({ type, side: "source" });

describe("port connection validation", () => {
  it("source producing a consumed port is compatible", () => {
    // translate produces target@target; qa consumes target@target.
    expect(isPortConnectionValid([tgt("target")], [tgt("target")])).toBe(true);
  });

  it("matches on side, not just type", () => {
    // produce term@source, consume term@source → ok.
    expect(isPortConnectionValid([src("term")], [src("term")])).toBe(true);
    // produce term@target but consume term@source → no overlap.
    expect(isPortConnectionValid([tgt("term")], [src("term")])).toBe(false);
  });

  it("non-overlapping ports are incompatible", () => {
    expect(isPortConnectionValid([src("word-count")], [tgt("target")])).toBe(false);
    expect(isPortConnectionValid([tgt("qa")], [tgt("target")])).toBe(false);
  });

  it("missing metadata defaults to compatible", () => {
    expect(isPortConnectionValid(undefined, [tgt("target")])).toBe(true);
    expect(isPortConnectionValid([tgt("target")], undefined)).toBe(true);
    expect(isPortConnectionValid(undefined, undefined)).toBe(true);
  });

  it("a target that consumes nothing accepts any source (pass-through)", () => {
    expect(isPortConnectionValid([tgt("target")], [])).toBe(true);
  });

  it("multiple produced ports satisfying one consumed port match", () => {
    expect(
      isPortConnectionValid(
        [src("tm-match"), src("alt-translation"), tgt("target")],
        [tgt("target")],
      ),
    ).toBe(true);
  });
});
