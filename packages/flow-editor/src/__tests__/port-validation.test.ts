import { describe, it, expect } from "vitest";
import type { IOPort } from "../types";

/**
 * Facet connection validation — mirrors the isValidConnection callback in
 * FlowEditor. A connection is meaningful when the source produces at least one
 * facet the target consumes (matched by type@side). Missing metadata, or a
 * target that consumes nothing (a pass-through), is permitted.
 */
function isFacetConnectionValid(
  srcProduces: IOPort[] | undefined,
  tgtConsumes: IOPort[] | undefined,
): boolean {
  if (!srcProduces || !tgtConsumes || tgtConsumes.length === 0) return true;
  const produced = new Set(srcProduces.map((f) => `${f.type}@${f.side ?? "source"}`));
  return tgtConsumes.some((c) => produced.has(`${c.type}@${c.side ?? "source"}`));
}

const tgt = (type: string): IOPort => ({ type, side: "target" });
const src = (type: string): IOPort => ({ type, side: "source" });

describe("facet connection validation", () => {
  it("source producing a consumed facet is compatible", () => {
    // translate produces target@target; qa-check consumes target@target.
    expect(isFacetConnectionValid([tgt("target")], [tgt("target")])).toBe(true);
  });

  it("matches on side, not just type", () => {
    // produce term@source, consume term@source → ok.
    expect(isFacetConnectionValid([src("term")], [src("term")])).toBe(true);
    // produce term@target but consume term@source → no overlap.
    expect(isFacetConnectionValid([tgt("term")], [src("term")])).toBe(false);
  });

  it("non-overlapping facets are incompatible", () => {
    expect(isFacetConnectionValid([src("word-count")], [tgt("target")])).toBe(false);
    expect(isFacetConnectionValid([tgt("qa")], [tgt("target")])).toBe(false);
  });

  it("missing metadata defaults to compatible", () => {
    expect(isFacetConnectionValid(undefined, [tgt("target")])).toBe(true);
    expect(isFacetConnectionValid([tgt("target")], undefined)).toBe(true);
    expect(isFacetConnectionValid(undefined, undefined)).toBe(true);
  });

  it("a target that consumes nothing accepts any source (pass-through)", () => {
    expect(isFacetConnectionValid([tgt("target")], [])).toBe(true);
  });

  it("multiple produced facets satisfying one consumed facet match", () => {
    expect(
      isFacetConnectionValid(
        [src("tm-match"), src("alt-translation"), tgt("target")],
        [tgt("target")],
      ),
    ).toBe(true);
  });
});
