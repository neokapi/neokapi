import { describe, it, expect } from "vitest";
import type { ContentNode, ContentTree } from "./types";
import { collectCues, activeCueIndex, formatTimecode, formatDuration } from "./timeline";
import { flattenGeometry, topUnits, boxPercent, extentOf, boxStyle } from "./geometry";

function block(id: string, extra: Partial<ContentNode> = {}): ContentNode {
  return { kind: "block", id, ...extra };
}
function tree(root: ContentNode[]): ContentTree {
  return { format: "test", root } as ContentTree;
}

describe("timeline.collectCues", () => {
  it("keeps only timed blocks, sorts by start then end, and re-indexes", () => {
    const t = tree([
      block("b", { timing: { startMs: 2000, endMs: 3000 } }),
      block("a", { timing: { startMs: 1000, endMs: 1500 } }),
      block("untimed"),
      {
        kind: "layer",
        id: "L",
        children: [block("c", { timing: { startMs: 3000, endMs: 4000 } })],
      },
    ]);
    const cues = collectCues(t);
    expect(cues.map((c) => c.node.id)).toEqual(["a", "b", "c"]);
    expect(cues.map((c) => c.index)).toEqual([0, 1, 2]);
  });

  it("breaks start-time ties by end time", () => {
    const cues = collectCues(
      tree([
        block("long", { timing: { startMs: 0, endMs: 5000 } }),
        block("short", { timing: { startMs: 0, endMs: 1000 } }),
      ]),
    );
    expect(cues.map((c) => c.node.id)).toEqual(["short", "long"]);
  });
});

describe("timeline.activeCueIndex", () => {
  const cues = collectCues(
    tree([
      block("a", { timing: { startMs: 0, endMs: 1000 } }),
      block("b", { timing: { startMs: 1000, endMs: 2000 } }),
    ]),
  );
  it("uses a half-open span so the boundary belongs to the next cue", () => {
    expect(activeCueIndex(cues, 999)).toBe(0);
    expect(activeCueIndex(cues, 1000)).toBe(1); // not still cue 0
  });
  it("returns -1 when no cue contains the playhead", () => {
    expect(activeCueIndex(cues, 5000)).toBe(-1);
  });
  it("prefers the latest-starting cue when spans overlap", () => {
    const overlap = collectCues(
      tree([
        block("base", { timing: { startMs: 0, endMs: 4000 } }),
        block("inner", { timing: { startMs: 1000, endMs: 2000 } }),
      ]),
    );
    expect(activeCueIndex(overlap, 1500)).toBe(overlap.find((c) => c.node.id === "inner")!.index);
  });
});

describe("timeline.formatTimecode / formatDuration", () => {
  it("renders HH:MM:SS.mmm zero-padded", () => {
    expect(formatTimecode(0)).toBe("00:00:00.000");
    expect(formatTimecode(3661500)).toBe("01:01:01.500");
    expect(formatTimecode(65250)).toBe("00:01:05.250");
  });
  it("clamps negatives to zero", () => {
    expect(formatTimecode(-10)).toBe("00:00:00.000");
  });
  it("formats a compact duration", () => {
    expect(formatDuration(1400)).toBe("1.4s");
    expect(formatDuration(-5)).toBe("0.0s");
  });
});

describe("geometry.flattenGeometry", () => {
  it("splits placed/unplaced and counts reading order across all blocks", () => {
    const { placed, unplaced } = flattenGeometry(
      tree([
        block("p1", { geometry: { x: 0, y: 0, w: 10, h: 10 } }),
        block("u1"),
        {
          kind: "group",
          id: "g",
          children: [block("p2", { geometry: { x: 5, y: 5, w: 10, h: 10 } })],
        },
      ]),
    );
    expect(placed.map((b) => b.node.id)).toEqual(["p1", "p2"]);
    expect(placed.map((b) => b.order)).toEqual([1, 3]); // u1 is order 2, still counted
    expect(unplaced.map((n) => n.id)).toEqual(["u1"]);
  });
});

describe("geometry.topUnits", () => {
  it("leaves top-left coordinates unchanged", () => {
    expect(topUnits("top-left", 10, 5, 100)).toBe(10);
    expect(topUnits(undefined, 10, 5, 100)).toBe(10);
  });
  it("flips bottom-left coordinates to top-down", () => {
    expect(topUnits("bottom-left", 10, 5, 100)).toBe(85); // 100 - 10 - 5
  });
});

describe("geometry.boxPercent / boxStyle", () => {
  it("maps a box to %-of-extent positioning", () => {
    const p = boxPercent({ x: 25, y: 50, w: 50, h: 25 }, 100, 100);
    expect(p).toEqual({ left: 25, top: 50, width: 50, height: 25 });
  });
  it("flips Y for a bottom-left origin", () => {
    const p = boxPercent({ x: 0, y: 10, w: 10, h: 10, origin: "bottom-left" }, 100, 100);
    expect(p.top).toBe(80); // (100 - 10 - 10) / 100 * 100
  });
  it("emits CSS percent strings", () => {
    expect(boxStyle({ x: 0, y: 0, w: 50, h: 50 }, 100, 100)).toEqual({
      left: "0%",
      top: "0%",
      width: "50%",
      height: "50%",
    });
  });
});

describe("geometry.extentOf", () => {
  it("uses the normalized resolution grid when declared", () => {
    expect(extentOf([{ g: { x: 0, y: 0, w: 1, h: 1, resolution: 512 } }])).toEqual({
      extentW: 512,
      extentH: 512,
    });
  });
  it("falls back to the surface's natural pixel size", () => {
    expect(extentOf([{ g: { x: 0, y: 0, w: 1, h: 1 } }], 640, 480)).toEqual({
      extentW: 640,
      extentH: 480,
    });
  });
  it("falls back to the max box extent with a small margin", () => {
    const { extentW, extentH } = extentOf([{ g: { x: 0, y: 0, w: 100, h: 200 } }]);
    expect(extentW).toBeCloseTo(104);
    expect(extentH).toBeCloseTo(208);
  });
});
