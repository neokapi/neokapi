import { inset, mixHex, shadeStep, squarify, type Rect, type Tile } from "../lib/treemap";

const rect: Rect = { x: 10, y: 20, w: 360, h: 414 };
const EPS = 1e-6;

function overlap(a: Rect, b: Rect): number {
  const w = Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x);
  const h = Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y);
  return w > EPS && h > EPS ? w * h : 0;
}

const cases: Record<string, number[]> = {
  one: [5],
  even: [1, 1, 1, 1],
  skewed: [18420, 2140, 980, 610, 412, 3],
  classic: [6, 6, 4, 3, 2, 2, 1],
  many: Array.from({ length: 40 }, (_, i) => ((i * 37) % 11) + 1),
};

describe("squarify", () => {
  it.each(Object.entries(cases))(
    "lays out %s proportionally, without overlap, filling the rect",
    (_, values) => {
      const tiles = squarify(values, (v) => v, rect);
      expect(tiles).toHaveLength(values.length);
      const total = values.reduce((s, v) => s + v, 0);
      const area = rect.w * rect.h;
      let covered = 0;
      for (const t of tiles) {
        expect(t.w * t.h).toBeCloseTo((t.item / total) * area, 3);
        expect(t.x).toBeGreaterThanOrEqual(rect.x - EPS);
        expect(t.y).toBeGreaterThanOrEqual(rect.y - EPS);
        expect(t.x + t.w).toBeLessThanOrEqual(rect.x + rect.w + EPS);
        expect(t.y + t.h).toBeLessThanOrEqual(rect.y + rect.h + EPS);
        covered += t.w * t.h;
      }
      expect(covered).toBeCloseTo(area, 3);
      for (let i = 0; i < tiles.length; i++)
        for (let j = i + 1; j < tiles.length; j++) expect(overlap(tiles[i], tiles[j])).toBe(0);
    },
  );

  it("keeps a stable order: largest first, equal values as given", () => {
    const items = [
      { id: "a", v: 2 },
      { id: "b", v: 5 },
      { id: "c", v: 2 },
      { id: "d", v: 5 },
    ];
    const ids = (ts: Tile<(typeof items)[number]>[]) => ts.map((t) => t.item.id);
    const first = squarify(items, (i) => i.v, rect);
    expect(ids(first)).toEqual(["b", "d", "a", "c"]);
    expect(squarify(items, (i) => i.v, rect)).toEqual(first);
  });

  it("keeps the tiles close to square", () => {
    const tiles = squarify(cases.classic, (v) => v, { x: 0, y: 0, w: 600, h: 400 });
    for (const t of tiles) expect(Math.max(t.w / t.h, t.h / t.w)).toBeLessThan(3);
  });

  it("leaves out items with no value and an empty rect", () => {
    expect(squarify([3, 0, -1], (v) => v, rect).map((t) => t.item)).toEqual([3]);
    expect(squarify([1, 2], (v) => v, { x: 0, y: 0, w: 0, h: 10 })).toEqual([]);
    expect(squarify([], (v: number) => v, rect)).toEqual([]);
  });
});

describe("treemap helpers", () => {
  it("insets without going negative", () => {
    expect(inset({ x: 0, y: 0, w: 10, h: 10 }, 2)).toEqual({ x: 2, y: 2, w: 6, h: 6 });
    expect(inset({ x: 0, y: 0, w: 2, h: 10 }, 2)).toEqual({ x: 1, y: 2, w: 0, h: 6 });
  });

  it("puts a share in its step", () => {
    expect([0, 0.19, 0.2, 0.5, 0.99, 1, 2, NaN].map((s) => shadeStep(s, 5))).toEqual([
      0, 0, 1, 2, 4, 4, 4, 0,
    ]);
  });

  it("mixes two colours", () => {
    expect(mixHex("#ffffff", "#000000", 0)).toBe("#ffffff");
    expect(mixHex("#ffffff", "#000000", 1)).toBe("#000000");
    expect(mixHex("#ffffff", "#0060c1", 0.5)).toBe("#80b0e0");
    expect(mixHex("rgb(1,2,3)", "#000000", 0.5)).toBe("rgb(1,2,3)");
  });
});
