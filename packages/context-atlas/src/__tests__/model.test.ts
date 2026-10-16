import { describe, expect, it } from "vite-plus/test";
import {
  ABSENT,
  DERIVED_AXES,
  NOT_SET,
  UNKNOWN,
  addFlowAxis,
  aggregateFlow,
  applyQuestion,
  attentionDelta,
  buildRaster,
  canGoBack,
  canGoForward,
  cohort,
  compareQueries,
  crossMatrix,
  dayEnd,
  dayStart,
  defaultQuery,
  evalContext,
  eventHistogram,
  exportInvestigation,
  facetCounts,
  goBack,
  goForward,
  layoutFlow,
  layoutTiles,
  mapPlaces,
  moveFlowAxis,
  parseQueryHash,
  prepareAtlas,
  previewSet,
  pushQuery,
  rasterBins,
  removeFlowAxis,
  serialiseQuery,
  setFlowAxis,
  shiftWindow,
  startHistory,
  stateAt,
  transitions,
  validateQuery,
  withFollow,
  withRetained,
  withSnapshot,
  withWindow,
  withoutFilter,
  type AtlasQuery,
} from "../model";
import type { AtlasData, AtlasRecord } from "../types";
import { makeAtlasFixture, makeEmptyAtlas, makeOneRecordAtlas } from "../stories/fixtures";

const fixture = makeAtlasFixture();
const atlas = prepareAtlas(fixture);

function rec(partial: Partial<AtlasRecord> & { id: string }): AtlasRecord {
  return {
    family: "rule",
    label: partial.id,
    weight: 1,
    coordinates: {},
    recorded: "2026-09-01T10:00:00Z",
    history: [],
    ...partial,
  };
}

function data(records: AtlasRecord[], extra: Partial<AtlasData> = {}): AtlasData {
  return {
    format: "kapi-context-atlas",
    version: 1,
    generated_at: "2026-09-30T00:00:00Z",
    span: { from: "2026-09-01T00:00:00Z", to: "2026-09-30T00:00:00Z" },
    axes: [
      {
        id: "channel",
        kind: "category",
        source: "structural",
        values: [{ id: "app" }, { id: "web" }],
      },
    ],
    records,
    events: [],
    notes: [],
    ...extra,
  };
}

describe("fixture", () => {
  it("is deterministic and sized for the stories", () => {
    const again = makeAtlasFixture();
    expect(JSON.stringify(again)).toBe(JSON.stringify(fixture));
    expect(fixture.records.length).toBeGreaterThanOrEqual(300);
    expect(fixture.records.length).toBeLessThanOrEqual(600);
    expect(new Set(fixture.records.map((r) => r.id)).size).toBe(fixture.records.length);
    expect(fixture.records.some((r) => r.family === "content" && r.weight > 1)).toBe(true);
    expect(fixture.axes.find((a) => a.id === "market")?.kind).toBe("geo");
  });

  it("keeps every history in time order and inside the span", () => {
    const from = Date.parse(fixture.span.from);
    const to = Date.parse(fixture.span.to);
    for (const r of fixture.records) {
      const ats = r.history.map((h) => Date.parse(h.at));
      expect([...ats].sort((a, b) => a - b)).toEqual(ats);
      for (const t of ats) {
        expect(t).toBeGreaterThanOrEqual(from);
        expect(t).toBeLessThanOrEqual(to);
      }
    }
  });
});

describe("stateAt", () => {
  const r = rec({
    id: "a",
    history: [
      { at: "2026-09-02T10:00:00Z", state: "suggested" },
      { at: "2026-09-05T10:00:00Z", state: "established" },
    ],
  });
  it("is absent before the first entry", () => {
    expect(stateAt(r, Date.parse("2026-09-02T09:59:59Z"))).toBe(ABSENT);
  });
  it("takes the last entry at or before t", () => {
    expect(stateAt(r, Date.parse("2026-09-02T10:00:00Z"))).toBe("suggested");
    expect(stateAt(r, Date.parse("2026-09-05T09:00:00Z"))).toBe("suggested");
    expect(stateAt(r, Date.parse("2026-09-05T10:00:00Z"))).toBe("established");
  });
  it("reads a record with no history as unknown from when it was recorded", () => {
    const none = rec({ id: "b", history: [] });
    expect(stateAt(none, Date.parse("2026-08-01T00:00:00Z"))).toBe(ABSENT);
    expect(stateAt(none, Date.parse("2026-09-02T00:00:00Z"))).toBe(UNKNOWN);
    const zero = rec({ id: "c", recorded: "0001-01-01T00:00:00Z", history: [] });
    expect(stateAt(zero, 0)).toBe(UNKNOWN);
  });
  it("sorts history it is handed out of order", () => {
    const shuffled = prepareAtlas(
      data([
        rec({
          id: "x",
          history: [
            { at: "2026-09-05T10:00:00Z", state: "established" },
            { at: "2026-09-02T10:00:00Z", state: "suggested" },
          ],
        }),
      ]),
    );
    const ctx = evalContext(shuffled, {
      ...defaultQuery(shuffled),
      from: "2026-09-03",
      to: "2026-09-10",
      asOf: "2026-09-10",
    });
    expect(transitions(shuffled, [0], ctx).transitions[0]).toMatchObject({
      from: "suggested",
      to: "established",
    });
  });
});

describe("query", () => {
  it("defaults to the last 30 days, following, flow, origin colour", () => {
    const q = defaultQuery(atlas);
    expect(q.to).toBe(atlas.lastDay);
    expect(q.asOf).toBe(q.to);
    expect(q.follow).toBe(true);
    expect(q.view).toBe("flow");
    expect(q.flowAxes.length).toBeGreaterThanOrEqual(2);
    expect(q.flowAxes.length).toBeLessThanOrEqual(5);
    expect(q.flowAxes).toContain(DERIVED_AXES.statusSnapshot);
  });

  it("validates whatever it is handed", () => {
    const d = defaultQuery(atlas);
    expect(validateQuery(null, atlas)).toEqual(d);
    expect(validateQuery("nonsense", atlas)).toEqual(d);
    const q = validateQuery(
      {
        from: "2026-10-10",
        to: "2026-08-01",
        follow: false,
        asOf: "1999-01-01",
        clock: "sideways",
        filters: { product: ["booking", "nope"], ghost: ["x"], channel: "app" },
        ids: ["missing", fixture.records[0].id],
        flowAxes: ["product", "product", "nope"],
        view: "map",
        lens: "change",
        surrounding: false,
        matrixRows: "nope",
      },
      atlas,
    );
    expect(q.from).toBe("2026-08-01");
    expect(q.to).toBe("2026-10-10");
    expect(q.asOf).toBe(atlas.firstDay);
    expect(q.clock).toBe("recorded");
    expect(q.filters).toEqual({ product: ["booking"] });
    expect(q.ids).toEqual([fixture.records[0].id]);
    expect(q.flowAxes).toEqual(d.flowAxes);
    expect(q.view).toBe("map");
    expect(q.lens).toBe("change");
    expect(q.surrounding).toBe(false);
    expect(q.matrixRows).toBe(d.matrixRows);
  });

  it("refuses the map without placed geography", () => {
    const small = prepareAtlas(makeOneRecordAtlas());
    expect(validateQuery({ view: "map" }, small).view).toBe("flow");
  });

  it("clamps days into the data and keeps the snapshot on the end while following", () => {
    const q = validateQuery(
      { from: "1990-01-01", to: "2999-01-01", follow: true, asOf: "2026-08-01" },
      atlas,
    );
    expect(q.from).toBe(atlas.firstDay);
    expect(q.to).toBe(atlas.lastDay);
    expect(q.asOf).toBe(atlas.lastDay);
  });

  it("round-trips through a link", () => {
    const q: AtlasQuery = {
      ...defaultQuery(atlas),
      filters: { market: ["no", "se"], [DERIVED_AXES.statusSnapshot]: ["contested"] },
      view: "matrix",
    };
    const hash = `#${serialiseQuery(q)}`;
    expect(validateQuery(parseQueryHash(hash), atlas)).toEqual(q);
    expect(parseQueryHash(`foo=1&${serialiseQuery(q)}`)).toEqual(q);
    expect(parseQueryHash("#q=%7Bnot-json")).toBeNull();
    expect(parseQueryHash("#other=1")).toBeNull();
  });

  it("edits the window, the snapshot and the follow flag", () => {
    const q = defaultQuery(atlas);
    const w = withWindow(q, "2026-09-10", "2026-09-01");
    expect([w.from, w.to, w.asOf]).toEqual(["2026-09-01", "2026-09-10", "2026-09-10"]);
    const s = withSnapshot(w, "2026-09-05");
    expect([s.asOf, s.follow]).toEqual(["2026-09-05", false]);
    expect(withWindow(s, "2026-09-02", "2026-09-12").asOf).toBe("2026-09-05");
    expect(withFollow(s, true).asOf).toBe("2026-09-10");
    const moved = shiftWindow(w, atlas, 1);
    expect([moved.from, moved.to]).toEqual(["2026-09-02", "2026-09-11"]);
    const pastEnd = shiftWindow(withWindow(q, "2026-10-05", atlas.lastDay), atlas, 5);
    expect(pastEnd.to).toBe(atlas.lastDay);
  });

  it("retains, adds and toggles within an axis", () => {
    let q = defaultQuery(atlas);
    q = withRetained(q, [{ axis: "market", value: "no" }]);
    expect(q.filters.market).toEqual(["no"]);
    q = withRetained(q, [{ axis: "market", value: "se" }], true);
    expect(q.filters.market).toEqual(["no", "se"]);
    q = withRetained(q, [{ axis: "market", value: "no" }], true);
    expect(q.filters.market).toEqual(["se"]);
    q = withRetained(q, [{ axis: "market", value: "se" }], true);
    expect(q.filters.market).toBeUndefined();
    q = withRetained(q, [
      { axis: "product", value: "booking" },
      { axis: "channel", value: "app" },
    ]);
    expect(q.filters).toEqual({ product: ["booking"], channel: ["app"] });
    expect(withoutFilter(q, "product").filters).toEqual({ channel: ["app"] });
  });

  it("edits flow columns without duplicates, between two and five", () => {
    const q: AtlasQuery = { ...defaultQuery(atlas), flowAxes: ["family", "product", "channel"] };
    expect(setFlowAxis(q, 0, "channel").flowAxes).toEqual(["channel", "product", "family"]);
    expect(setFlowAxis(q, 1, "market").flowAxes).toEqual(["family", "market", "channel"]);
    expect(moveFlowAxis(q, 0, -1)).toBe(q);
    expect(moveFlowAxis(q, 0, 1).flowAxes).toEqual(["product", "family", "channel"]);
    expect(removeFlowAxis(removeFlowAxis(q, 0), 0).flowAxes).toHaveLength(2);
    let wide = q;
    for (let i = 0; i < 6; i++) wide = addFlowAxis(wide, atlas);
    expect(wide.flowAxes).toHaveLength(5);
    expect(new Set(wide.flowAxes).size).toBe(5);
  });

  it("keeps a bounded back and forward history", () => {
    const q0 = defaultQuery(atlas);
    let h = startHistory(q0);
    expect(canGoBack(h)).toBe(false);
    const q1 = { ...q0, view: "matrix" as const };
    const q2 = { ...q0, view: "records" as const };
    h = pushQuery(h, q1);
    h = pushQuery(h, q1);
    expect(h.past).toHaveLength(1);
    h = pushQuery(h, q2);
    h = goBack(h);
    expect(h.present).toEqual(q1);
    expect(canGoForward(h)).toBe(true);
    h = goForward(h);
    expect(h.present).toEqual(q2);
    h = goBack(h);
    h = pushQuery(h, q0);
    expect(canGoForward(h)).toBe(false);
    for (let i = 0; i < 80; i++)
      h = pushQuery(h, { ...q0, from: i % 2 ? q0.from : atlas.firstDay, ids: [String(i)] });
    expect(h.past.length).toBe(60);
  });

  it("arranges the starting questions with the atlas's own axes", () => {
    const q = defaultQuery(atlas);
    const changed = applyQuestion(q, atlas, "changed");
    expect(changed.filters[DERIVED_AXES.activity]).toEqual(["changed"]);
    expect(changed.flowAxes).toContain(DERIVED_AXES.statusStart);
    const applies = applyQuestion(q, atlas, "applies");
    expect(applies.filters.family).toEqual(["rule"]);
    expect(applies.flowAxes[0]).toBe("family");
    const needs = applyQuestion(q, atlas, "needs-person");
    expect(needs.lens).toBe("attention");
    expect(needs.flowAxes.slice(0, 2)).toEqual([
      DERIVED_AXES.statusStart,
      DERIVED_AXES.statusSnapshot,
    ]);
    const life = applyQuestion(q, atlas, "lifetime");
    expect(life.flowAxes[0]).toBe(DERIVED_AXES.lifetime);
    for (const x of [changed, applies, needs, life]) {
      expect(validateQuery(x, atlas)).toEqual(x);
    }
  });
});

describe("derived axes", () => {
  const records = [
    rec({
      id: "upcoming",
      valid_from: "2026-10-01T00:00:00Z",
      history: [{ at: "2026-09-01T10:00:00Z", state: "established" }],
    }),
    rec({
      id: "in-force",
      valid_from: "2026-09-01T00:00:00Z",
      valid_to: "2026-10-01T00:00:00Z",
      history: [{ at: "2026-09-01T10:00:00Z", state: "established" }],
    }),
    rec({
      id: "expired",
      valid_to: "2026-09-20T00:00:00Z",
      history: [{ at: "2026-09-01T10:00:00Z", state: "established" }],
    }),
    rec({ id: "open", history: [{ at: "2026-09-01T10:00:00Z", state: "suggested" }] }),
    rec({
      id: "old",
      recorded: "2026-05-01T00:00:00Z",
      history: [{ at: "2026-05-01T00:00:00Z", state: "established" }],
    }),
    rec({
      id: "fresh",
      recorded: "2026-09-25T00:00:00Z",
      history: [{ at: "2026-09-25T00:00:00Z", state: "suggested" }],
    }),
  ];
  const a = prepareAtlas(data(records));
  const q = { ...defaultQuery(a), from: "2026-09-21", to: "2026-09-28", asOf: "2026-09-28" };
  const ctx = evalContext(a, q);
  const value = (id: string, axis: string) => {
    const p = a.byId.get(id)!;
    return facetCounts(a, [p.index], axis, ctx).keys().next().value;
  };
  it("places the snapshot in each lifetime window (valid_to exclusive)", () => {
    expect(value("upcoming", DERIVED_AXES.lifetime)).toBe("upcoming");
    expect(value("in-force", DERIVED_AXES.lifetime)).toBe("in-force");
    expect(value("expired", DERIVED_AXES.lifetime)).toBe("expired");
    expect(value("open", DERIVED_AXES.lifetime)).toBe("open");
  });
  it("bands the age at the snapshot", () => {
    expect(value("fresh", DERIVED_AXES.age)).toBe("lt-7d");
    expect(value("open", DERIVED_AXES.age)).toBe("7-30d");
    expect(value("old", DERIVED_AXES.age)).toBe("gt-90d");
  });
  it("reads state at the window's start and at the snapshot", () => {
    expect(value("fresh", DERIVED_AXES.statusStart)).toBe(ABSENT);
    expect(value("fresh", DERIVED_AXES.statusSnapshot)).toBe("suggested");
  });
  it("lists a coordinate a record lacks as not set", () => {
    expect(value("open", "channel")).toBe(NOT_SET);
    expect(a.axisById.get("channel")?.values.map((v) => v.id)).toContain(NOT_SET);
  });
});

describe("filtering", () => {
  const q0 = defaultQuery(atlas);
  const ctx0 = evalContext(atlas, q0);
  const all = cohort(atlas, q0, ctx0);

  it("selects only records recorded by the snapshot", () => {
    for (const i of all) {
      expect(stateAt(atlas.records[i].record, ctx0.snapshotMs)).not.toBe(ABSENT);
    }
    const early = { ...q0, from: atlas.firstDay, to: atlas.firstDay, asOf: atlas.firstDay };
    expect(cohort(atlas, early).length).toBeLessThan(all.length);
  });

  it("is OR within an axis and AND across axes", () => {
    const q = { ...q0, filters: { product: ["booking", "loyalty"], channel: ["app"] } };
    const ctx = evalContext(atlas, q);
    const got = new Set(cohort(atlas, q, ctx));
    const expected = all.filter((i) => {
      const c = atlas.records[i].record.coordinates;
      return (c.product === "booking" || c.product === "loyalty") && c.channel === "app";
    });
    expect([...got].sort()).toEqual(expected.sort());
    expect(expected.length).toBeGreaterThan(0);
  });

  it("releases the named axes for surrounding context and facet counts", () => {
    const q = { ...q0, filters: { product: ["booking"], channel: ["app"] } };
    const ctx = evalContext(atlas, q);
    const sel = cohort(atlas, q, ctx);
    const around = cohort(atlas, q, ctx, ["product"]);
    expect(around.length).toBeGreaterThan(sel.length);
    for (const i of sel) expect(around).toContain(i);
    const facets = facetCounts(atlas, around, "product", ctx);
    expect(facets.size).toBeGreaterThan(1);
    expect(facets.get("booking")?.count).toBe(sel.length);
  });

  it("restricts to an explicit id set", () => {
    const ids = [fixture.records[0].id, fixture.records[1].id];
    const q = { ...defaultQuery(atlas), from: atlas.firstDay, ids };
    expect(
      cohort(atlas, q)
        .map((i) => atlas.records[i].record.id)
        .sort(),
    ).toEqual(
      ids.filter((id) => stateAt(atlas.byId.get(id)!.record, dayEnd(q.asOf)) !== ABSENT).sort(),
    );
  });

  it("filters on what changed in the window, on either clock", () => {
    const q = { ...q0, filters: { [DERIVED_AXES.activity]: ["changed"] } };
    const rec = cohort(atlas, q);
    const eff = cohort(atlas, { ...q, clock: "effective" as const });
    expect(rec.length).toBeGreaterThan(0);
    expect(rec.length).toBeLessThan(all.length);
    expect(eff.length).toBeGreaterThan(0);
  });

  it("previews an identity set without touching the query", () => {
    const set = previewSet(atlas, all, [{ axis: "product", value: "booking" }], ctx0);
    expect(set.size).toBe(
      all.filter((i) => atlas.records[i].record.coordinates.product === "booking").length,
    );
    const both = previewSet(
      atlas,
      all,
      [
        { axis: "product", value: "booking" },
        { axis: "channel", value: "app" },
      ],
      ctx0,
    );
    expect(both.size).toBeLessThanOrEqual(set.size);
    for (const i of both) expect(set.has(i)).toBe(true);
  });
});

describe("flow layout", () => {
  const axisIds = atlas.axes.map((a) => a.id);
  // Many axis orders, many windows: every boundary conserves the total.
  const orders: string[][] = [];
  for (let i = 0; i < axisIds.length; i++) {
    for (let j = 0; j < axisIds.length; j++) {
      if (i === j) continue;
      const k = (i + j + 1) % axisIds.length;
      const cols = [axisIds[i], axisIds[j]];
      if (k !== i && k !== j) cols.push(axisIds[k]);
      orders.push(cols);
    }
  }

  it.each(orders.slice(0, 60).map((o) => [o.join(" → "), o]))(
    "conserves counts at every boundary: %s",
    (_, axes) => {
      const q = {
        ...defaultQuery(atlas),
        flowAxes: axes as string[],
        filters: { channel: ["app"] },
      };
      const ctx = evalContext(atlas, q);
      const fg = cohort(atlas, q, ctx);
      const universe = cohort(atlas, q, ctx, axes as string[]);
      const layout = layoutFlow(atlas, {
        axes: axes as string[],
        universe,
        foreground: fg,
        ctx,
        height: 320,
        columnWidth: 200,
      });
      const totalAll = universe.reduce((s, i) => s + atlas.records[i].weight, 0);
      const totalFg = fg.reduce((s, i) => s + atlas.records[i].weight, 0);
      expect(layout.all.weight).toBe(totalAll);
      expect(layout.fg.weight).toBe(totalFg);
      for (const col of layout.columns) {
        expect(col.nodes.reduce((s, n) => s + n.all.weight, 0)).toBe(totalAll);
        expect(col.nodes.reduce((s, n) => s + n.fg.weight, 0)).toBe(totalFg);
      }
      for (let c = 0; c + 1 < layout.columns.length; c++) {
        const rs = layout.ribbons.filter((r) => r.column === c);
        expect(rs.reduce((s, r) => s + r.all.weight, 0)).toBe(totalAll);
        expect(rs.reduce((s, r) => s + r.fg.weight, 0)).toBe(totalFg);
        // Each node's ribbons sum to the node, on both sides.
        for (const n of layout.columns[c].nodes) {
          expect(rs.filter((r) => r.from === n.value).reduce((s, r) => s + r.all.weight, 0)).toBe(
            n.all.weight,
          );
        }
        for (const n of layout.columns[c + 1].nodes) {
          expect(rs.filter((r) => r.to === n.value).reduce((s, r) => s + r.all.weight, 0)).toBe(
            n.all.weight,
          );
        }
      }
    },
  );

  it("holds the across windows and snapshots", () => {
    const axes = [
      DERIVED_AXES.statusStart,
      DERIVED_AXES.statusSnapshot,
      "family",
      DERIVED_AXES.lifetime,
    ];
    for (const [from, to] of [
      [atlas.firstDay, atlas.lastDay],
      ["2026-08-01", "2026-08-15"],
      ["2026-09-01", "2026-09-30"],
      [atlas.lastDay, atlas.lastDay],
    ]) {
      const q = { ...defaultQuery(atlas), from, to, asOf: to, flowAxes: axes };
      const ctx = evalContext(atlas, q);
      const fg = cohort(atlas, q, ctx);
      const agg = aggregateFlow(atlas, fg, axes, ctx);
      for (let c = 0; c < axes.length; c++) {
        let sum = 0;
        for (const [k, v] of agg.nodes) if (k.startsWith(`${c}\u0000`)) sum += v.weight;
        expect(sum).toBe(agg.total.weight);
      }
    }
  });

  it("fits the height, keeps small nodes visible, and draws ribbons proportionally", () => {
    const q = {
      ...defaultQuery(atlas),
      flowAxes: ["family", "market", DERIVED_AXES.statusSnapshot],
    };
    const ctx = evalContext(atlas, q);
    const fg = cohort(atlas, q, ctx);
    const layout = layoutFlow(atlas, {
      axes: q.flowAxes,
      universe: fg,
      foreground: fg,
      ctx,
      height: 400,
      columnWidth: 220,
      minNodeHeight: 14,
      gap: 6,
    });
    expect(layout.height).toBeLessThanOrEqual(400.001);
    for (const col of layout.columns)
      for (const n of col.nodes) expect(n.height).toBeGreaterThanOrEqual(14);
    for (const r of layout.ribbons) expect(r.thickness).toBeCloseTo(r.all.weight * layout.scale, 6);
    // Ribbons leaving a node stack inside it without overlap.
    const first = layout.columns[0].nodes[0];
    const out = layout.ribbons.filter((r) => r.column === 0 && r.from === first.value);
    let y = first.y;
    for (const r of out) {
      expect(r.sourceY).toBeCloseTo(y, 6);
      y += r.thickness;
    }
  });

  it("keeps a retained value with nothing in it, and draws empty data", () => {
    const q = { ...defaultQuery(atlas), flowAxes: ["family", "product"] };
    const ctx = evalContext(atlas, q);
    const layout = layoutFlow(atlas, {
      axes: q.flowAxes,
      universe: [],
      foreground: [],
      ctx,
      height: 300,
      columnWidth: 200,
      keep: { product: ["booking"] },
    });
    expect(layout.columns[1].nodes.map((n) => n.value)).toEqual(["booking"]);
    expect(layout.ribbons).toHaveLength(0);
  });

  it("tracks a single origin and a mixed one", () => {
    const q = { ...defaultQuery(atlas), flowAxes: ["product", "channel"] };
    const ctx = evalContext(atlas, q);
    const fg = cohort(atlas, q, ctx);
    const agg = aggregateFlow(atlas, fg, q.flowAxes, ctx);
    for (const [key, v] of agg.nodes) {
      if (key.startsWith("0\u0000")) expect(v.origin).toBe(key.split("\u0000")[1]);
    }
    expect(
      [...agg.nodes.entries()].some(([k, v]) => k.startsWith("1\u0000") && v.origin === null),
    ).toBe(true);
  });

  it("measures the change in attention against each instant's own denominator", () => {
    expect(
      attentionDelta({
        weight: 10,
        count: 10,
        attention: 5,
        observedStart: 5,
        attentionStart: 0,
        origin: null,
      }),
    ).toBeCloseTo(0.5);
    expect(
      attentionDelta({
        weight: 10,
        count: 10,
        attention: 5,
        observedStart: 0,
        attentionStart: 0,
        origin: null,
      }),
    ).toBeNull();
  });
});

describe("transitions", () => {
  it("counts every pair and the turnover a net change hides", () => {
    const q = { ...defaultQuery(atlas), from: "2026-08-20", to: "2026-09-30", asOf: "2026-09-30" };
    const ctx = evalContext(atlas, q);
    const fg = cohort(atlas, q, ctx);
    const t = transitions(atlas, fg, ctx);
    expect(t.transitions.reduce((s, x) => s + x.count, 0)).toBe(fg.length);
    expect(t.totalCount).toBe(fg.length);
    expect(t.changedCount).toBe(
      t.transitions.filter((x) => x.from !== x.to).reduce((s, x) => s + x.count, 0),
    );
    for (let i = 1; i < t.transitions.length; i++) {
      expect(t.transitions[i - 1].weight).toBeGreaterThanOrEqual(t.transitions[i].weight);
    }
    expect(t.transitions.some((x) => x.from === ABSENT)).toBe(true);
  });
});

describe("matrix", () => {
  it("crosses two axes and keeps empty cells apart from zero", () => {
    const q = { ...defaultQuery(atlas), filters: { family: ["rule"] } };
    const ctx = evalContext(atlas, q);
    const sel = cohort(atlas, q, ctx);
    const around = cohort(atlas, q, ctx, ["product", "channel"]);
    const m = crossMatrix(atlas, sel, around, "product", "channel", ctx);
    expect(m.rows.length).toBeGreaterThan(0);
    const sum = m.cells.flat().reduce((s, c) => s + c.weight, 0);
    expect(sum).toBe(sel.reduce((s, i) => s + atlas.records[i].weight, 0));
    expect(m.rowTotals.reduce((a, b) => a + b, 0)).toBe(sum);
    for (const cell of m.cells.flat()) {
      if (cell.weight === 0) expect(cell.share).toBeNull();
      else expect(cell.share).toBeGreaterThanOrEqual(0);
    }
  });
});

describe("raster", () => {
  it("splits a window into explicit bins that cover it exactly", () => {
    for (const [from, to, size] of [
      ["2026-09-01", "2026-09-10", 1],
      ["2026-07-17", "2026-10-14", 7],
      ["2026-01-01", "2026-12-31", 30],
    ] as const) {
      const bins = rasterBins(from, to);
      expect(bins[0].from).toBe(from);
      expect(bins[bins.length - 1].to).toBe(to);
      for (let i = 1; i < bins.length; i++) {
        expect(dayStart(bins[i].from) - dayEnd(bins[i - 1].to)).toBe(1);
      }
      expect(Math.round((dayEnd(bins[0].to) + 1 - dayStart(bins[0].from)) / 86_400_000)).toBe(
        Math.min(size, bins.length === 1 ? Infinity : size),
      );
      for (const b of bins) expect(b.endMs).toBe(dayEnd(b.to));
    }
  });

  it("reads state at each bin's end and leaves unrecorded cells empty", () => {
    const q = {
      ...defaultQuery(atlas),
      from: atlas.firstDay,
      to: atlas.lastDay,
      asOf: atlas.lastDay,
    };
    const ctx = evalContext(atlas, q);
    const sel = cohort(atlas, q, ctx);
    const r = buildRaster(atlas, sel, "product", q.from, q.to, ctx);
    expect(r.rows.length).toBeGreaterThan(0);
    // The last bin ends at the snapshot: every selected record is recorded there.
    const lastObserved = r.cells.reduce((s, row) => s + row[row.length - 1].observed, 0);
    expect(lastObserved).toBe(sel.reduce((s, i) => s + atlas.records[i].weight, 0));
    // Observed weight never falls as bins advance: records do not leave.
    for (const row of r.cells)
      for (let b = 1; b < row.length; b++)
        expect(row[b].observed).toBeGreaterThanOrEqual(row[b - 1].observed);
    for (const row of r.cells)
      for (const c of row) if (c.observed === 0) expect(c.share).toBeNull();
  });

  it("marks a bin before any record as empty, not as zero", () => {
    const a = prepareAtlas(
      data([rec({ id: "late", history: [{ at: "2026-09-20T10:00:00Z", state: "suggested" }] })]),
    );
    const q = { ...defaultQuery(a), from: "2026-09-15", to: "2026-09-25", asOf: "2026-09-25" };
    const ctx = evalContext(a, q);
    const r = buildRaster(a, cohort(a, q, ctx), "family", q.from, q.to, ctx);
    expect(r.cells[0][0].share).toBeNull();
    expect(r.cells[0][r.bins.length - 1].share).toBe(1);
  });
});

describe("compare", () => {
  it("separates membership from state change, on one scale, in one order", () => {
    const qa = {
      ...defaultQuery(atlas),
      from: "2026-09-01",
      to: "2026-09-20",
      asOf: "2026-09-20",
      follow: true,
    };
    const qb = {
      ...qa,
      from: "2026-09-10",
      to: "2026-10-05",
      asOf: "2026-10-05",
      filters: { family: ["rule", "term"] },
    };
    const cmp = compareQueries(atlas, qa, qb, ["family", DERIVED_AXES.statusSnapshot]);
    expect(cmp.common + cmp.onlyA).toBe(cmp.a.count);
    expect(cmp.common + cmp.onlyB).toBe(cmp.b.count);
    expect(cmp.onlyA).toBeGreaterThan(0);
    expect(cmp.changedState).toBeGreaterThanOrEqual(0);
    expect(cmp.changedState).toBeLessThanOrEqual(cmp.common);
    for (const ax of cmp.axes) {
      expect(ax.rows.reduce((s, r) => s + r.a, 0)).toBe(cmp.a.weight);
      expect(ax.rows.reduce((s, r) => s + r.b, 0)).toBe(cmp.b.weight);
      for (const r of ax.rows) expect(Math.max(r.a, r.b)).toBeLessThanOrEqual(cmp.maxWeight);
    }
    const fam = cmp.axes[0].rows.map((r) => r.value);
    const catalogue = atlas.axisById.get("family")!.values.map((v) => v.id);
    expect(fam).toEqual(catalogue.filter((v) => fam.includes(v)));
    expect(cmp.axes[1].temporal).toBe(true);
  });

  it("finds nothing changed when A and B are the same query", () => {
    const q = defaultQuery(atlas);
    const cmp = compareQueries(atlas, q, q);
    expect(cmp.onlyA).toBe(0);
    expect(cmp.onlyB).toBe(0);
    expect(cmp.changedState).toBe(0);
  });
});

describe("event histogram", () => {
  it("counts every event once per day on the chosen clock", () => {
    for (const clock of ["recorded", "effective"] as const) {
      const h = eventHistogram(atlas, clock);
      expect(h.days[0]).toBe(atlas.firstDay);
      expect(h.days[h.days.length - 1]).toBe(atlas.lastDay);
      expect(h.totals.reduce((a, b) => a + b, 0) + h.outside).toBe(fixture.events.length);
    }
    // Effective windows open after the span: the effective clock places some outside it.
    expect(eventHistogram(atlas, "effective").outside).toBeGreaterThan(0);
    expect(eventHistogram(atlas, "recorded").outside).toBe(0);
  });
});

describe("investigation export", () => {
  it("names the data, the query and the counts", () => {
    const q = { ...defaultQuery(atlas), filters: { market: ["no"] } };
    const pinned = defaultQuery(atlas);
    const inv = exportInvestigation(atlas, q, {
      pinned,
      selected: fixture.records[0].id,
      now: new Date("2026-10-16T12:00:00Z"),
    });
    expect(inv.format).toBe("kapi-context-investigation");
    expect(inv.version).toBe(1);
    expect(inv.generated_at).toBe("2026-10-16T12:00:00.000Z");
    expect(inv.data).toMatchObject({
      format: "kapi-context-atlas",
      version: 1,
      project: "larkspur-travel",
    });
    expect(inv.data.span).toEqual(fixture.span);
    expect(inv.query).toEqual(q);
    expect(inv.pinned).toEqual(pinned);
    expect(inv.selected?.id).toBe(fixture.records[0].id);
    expect(inv.counts.records).toBe(cohort(atlas, q).length);
    expect(inv.counts.total_records).toBe(fixture.records.length);
    expect(inv.transitions.reduce((s, t) => s + t.count, 0)).toBe(inv.counts.records);
    expect(validateQuery(JSON.parse(JSON.stringify(inv)).query, atlas)).toEqual(q);
  });
});

describe("tiles", () => {
  it("gives every place one tile and never two on one cell", () => {
    const market = atlas.axisById.get("market")!;
    const inputs = market.values
      .filter((v) => v.centroid)
      .map((v) => ({ id: v.id, centroid: v.centroid! }));
    const grid = layoutTiles(inputs);
    expect(grid.tiles).toHaveLength(inputs.length);
    expect(new Set(grid.tiles.map((t) => `${t.col},${t.row}`)).size).toBe(inputs.length);
    for (const t of grid.tiles) {
      expect(t.col).toBeGreaterThanOrEqual(0);
      expect(t.row).toBeGreaterThanOrEqual(0);
      expect(t.col).toBeLessThan(grid.cols);
      expect(t.row).toBeLessThan(grid.rows);
    }
    const pos = new Map(grid.tiles.map((t) => [t.id, t]));
    // Rough geography survives: Norway sits north of Spain, Japan east of France.
    expect(pos.get("no")!.row).toBeLessThan(pos.get("es")!.row);
    expect(pos.get("jp")!.col).toBeGreaterThan(pos.get("fr")!.col);
  });

  it("draws a group's countries once each, and knows which groups hold them", () => {
    const placed = mapPlaces([
      { id: "de", centroid: [51, 10] },
      {
        id: "nordics",
        members: [
          { id: "no", label: "Norway", centroid: [62, 10] },
          { id: "se", label: "Sweden", centroid: [62, 15] },
        ],
      },
      {
        id: "northern-europe",
        members: [
          { id: "no", label: "Norway", centroid: [62, 10] },
          { id: "gb", label: "United Kingdom", centroid: [54, -2] },
        ],
      },
    ]);
    expect(placed.places.map((p) => p.id)).toEqual(["de", "no", "se", "gb"]);
    expect(placed.within.get("no")).toEqual(["nordics", "northern-europe"]);
    expect(placed.within.get("de")).toBeUndefined();
    expect(placed.names.get("gb")).toBe("United Kingdom");
  });

  it("handles nothing and one place", () => {
    expect(layoutTiles([])).toEqual({ tiles: [], cols: 0, rows: 0 });
    expect(layoutTiles([{ id: "no", centroid: [62, 10] }])).toEqual({
      tiles: [{ id: "no", col: 0, row: 0 }],
      cols: 1,
      rows: 1,
    });
  });
});

describe("empty and tiny data", () => {
  it("prepares and queries an empty atlas", () => {
    const a = prepareAtlas(makeEmptyAtlas());
    const q = defaultQuery(a);
    expect(q.from).toBe(q.to);
    expect(cohort(a, q)).toEqual([]);
    const layout = layoutFlow(a, {
      axes: q.flowAxes,
      universe: [],
      foreground: [],
      ctx: evalContext(a, q),
      height: 300,
      columnWidth: 200,
    });
    expect(layout.ribbons).toEqual([]);
    expect(eventHistogram(a, "recorded").max).toBe(0);
  });

  it("queries a single record", () => {
    const a = prepareAtlas(makeOneRecordAtlas());
    const q = defaultQuery(a);
    expect(cohort(a, q)).toEqual([0]);
  });

  it("ends the timeline when the atlas was read, not at a window bound next month", () => {
    const one = makeOneRecordAtlas();
    const generated = one.generated_at;
    const a = prepareAtlas({
      ...one,
      // A rule window that closes thirty days after the read stretches the
      // host's span into the future.
      span: {
        from: one.span.from,
        to: new Date(Date.parse(generated) + 30 * 86_400_000).toISOString(),
      },
    });
    expect(a.lastDay).toBe(generated.slice(0, 10));
    expect(defaultQuery(a).to).toBe(generated.slice(0, 10));
  });
});
