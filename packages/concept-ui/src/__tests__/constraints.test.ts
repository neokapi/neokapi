import { describe, expect, it } from "vitest";
import {
  buildConstraintModel,
  constraintSummary,
  marketLabelFor,
  windowPhrase,
} from "../constraints";
import type { Concept, Market, Relation, Term } from "../types";

const MARKETS: Market[] = [
  { id: "dach", name: "DACH", locales: ["de-DE", "de-AT"] },
  { id: "france", name: "France", locales: ["fr-FR"] },
  { id: "us", name: "United States", locales: ["en-US"] },
];

const term = (
  text: string,
  locale: string,
  status: Term["status"],
  validity?: Term["validity"],
): Term => ({ text, locale, status, validity });

// A concept with: an open-end preferred term (valid-from only), a closed window
// banned term (valid-to in the past), and a plain always-valid term (no window).
const concept: Concept = {
  id: "checkout",
  terms: [
    term("Kasse", "de-DE", "preferred", {
      validFrom: "2026-01-01T00:00:00Z",
      tags: { market: "dach" },
    }),
    term("Validation de commande", "fr-FR", "deprecated", {
      validTo: "2026-06-01T00:00:00Z",
      tags: { market: "france" },
    }),
    term("Checkout", "en-US", "preferred"),
    term("Voucher", "en-US", "forbidden", undefined),
  ],
};

describe("marketLabelFor", () => {
  it("resolves an explicit market tag to the named market", () => {
    expect(marketLabelFor(concept.terms[0], MARKETS)).toBe("DACH");
  });

  it("falls back to the market that covers the locale", () => {
    expect(marketLabelFor(term("Checkout", "en-US", "preferred"), MARKETS)).toBe("United States");
  });

  it("is undefined when nothing covers the locale", () => {
    expect(marketLabelFor(term("X", "ja-JP", "approved"), MARKETS)).toBeUndefined();
  });
});

describe("windowPhrase", () => {
  // The month-year label is delegated to toLocaleDateString(undefined, …), which
  // depends on the runtime locale AND timezone. Derive the expected labels the
  // same way so these assertions exercise the composition (→ / since / until /
  // always) rather than a hardcoded en-US/UTC rendering.
  const monthYear = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { month: "short", year: "numeric" });
  const from = monthYear("2026-01-01T00:00:00Z");
  const to = monthYear("2026-06-01T00:00:00Z");

  it("phrases closed, open-end, open-start, and absent windows", () => {
    expect(windowPhrase("2026-01-01T00:00:00Z", "2026-06-01T00:00:00Z")).toBe(`${from} → ${to}`);
    expect(windowPhrase("2026-01-01T00:00:00Z")).toBe(`since ${from}`);
    expect(windowPhrase(undefined, "2026-06-01T00:00:00Z")).toBe(`until ${to}`);
    expect(windowPhrase()).toBe("always");
  });
});

describe("buildConstraintModel", () => {
  const asOf = "2026-03-01T00:00:00Z";

  it("makes a lane only for dated terms/relations", () => {
    const model = buildConstraintModel(concept, { asOf, markets: MARKETS });
    expect(model.hasWindows).toBe(true);
    // Two dated terms; the two windowless terms are excluded from the chart.
    // Open-start lanes (valid-to only) sort first — they began before the chart.
    expect(model.lanes).toHaveLength(2);
    expect(model.lanes.map((l) => l.label)).toEqual(["Validation de commande", "Kasse"]);
  });

  it("flags open-start / open-end and positions bars within 0..1", () => {
    const model = buildConstraintModel(concept, { asOf, markets: MARKETS });
    const kasse = model.lanes.find((l) => l.label === "Kasse")!;
    expect(kasse.openStart).toBe(false);
    expect(kasse.openEnd).toBe(true); // valid-from only → runs to the edge
    expect(kasse.end).toBe(1);
    expect(kasse.start).toBeGreaterThan(0);
    expect(kasse.start).toBeLessThan(1);

    const dep = model.lanes.find((l) => l.label.startsWith("Validation"))!;
    expect(dep.openStart).toBe(true); // valid-to only
    expect(dep.start).toBe(0);
    expect(dep.end).toBeGreaterThan(0);
    expect(dep.end).toBeLessThanOrEqual(1);
  });

  it("computes as-of active state per window", () => {
    const model = buildConstraintModel(concept, { asOf, markets: MARKETS });
    // Kasse valid-from 2026-01 → active at 2026-03.
    expect(model.lanes.find((l) => l.label === "Kasse")!.active).toBe(true);
    // Validation valid-to 2026-06 → still active at 2026-03.
    expect(model.lanes.find((l) => l.label.startsWith("Validation"))!.active).toBe(true);
    // As of after the valid-to it is no longer in force.
    const later = buildConstraintModel(concept, { asOf: "2026-07-01T00:00:00Z", markets: MARKETS });
    expect(later.lanes.find((l) => l.label.startsWith("Validation"))!.active).toBe(false);
  });

  it("includes a now marker inside the scale and ordered ticks", () => {
    const model = buildConstraintModel(concept, { asOf, markets: MARKETS });
    expect(model.scale.nowPos).toBeGreaterThan(0);
    expect(model.scale.nowPos).toBeLessThan(1);
    const positions = model.scale.ticks.map((t) => t.pos);
    const sorted = [...positions].sort((a, b) => a - b);
    expect(positions).toEqual(sorted);
    positions.forEach((p) => {
      expect(p).toBeGreaterThanOrEqual(0);
      expect(p).toBeLessThanOrEqual(1);
    });
  });

  it("turns dated relations into lanes with a neighbour label", () => {
    const relations: Relation[] = [
      {
        id: "r1",
        sourceId: "checkout",
        targetId: "promo",
        type: "USE_INSTEAD",
        validity: { validFrom: "2026-02-01T00:00:00Z" },
      },
    ];
    const model = buildConstraintModel(concept, {
      asOf,
      markets: MARKETS,
      relations,
      labelFor: (id) => (id === "promo" ? "Promo code" : id),
    });
    const lane = model.lanes.find((l) => l.kind === "relation")!;
    expect(lane.label).toBe("use instead Promo code");
    expect(lane.openEnd).toBe(true);
  });

  it("reports no windows when nothing is dated", () => {
    const flat: Concept = { id: "x", terms: [term("Checkout", "en-US", "preferred")] };
    const model = buildConstraintModel(flat, { asOf });
    expect(model.hasWindows).toBe(false);
    expect(model.lanes).toEqual([]);
  });
});

describe("constraintSummary", () => {
  it("splits banned vs preferred and places each by market", () => {
    const summary = constraintSummary(concept, { asOf: "2026-03-01T00:00:00Z", markets: MARKETS });
    expect(summary.banned.map((p) => p.text)).toEqual(["Validation de commande", "Voucher"]);
    expect(summary.preferred.map((p) => p.text)).toEqual(["Kasse", "Checkout"]);
    // Banned ordered by market (France before United States).
    expect(summary.banned.map((p) => p.market)).toEqual(["France", "United States"]);
  });

  it("flags as-of in-force state on placements", () => {
    const later = constraintSummary(concept, { asOf: "2026-07-01T00:00:00Z", markets: MARKETS });
    const dep = later.banned.find((p) => p.text === "Validation de commande")!;
    expect(dep.active).toBe(false); // valid-to 2026-06 has passed
    const voucher = later.banned.find((p) => p.text === "Voucher")!;
    expect(voucher.active).toBe(true); // always-banned
  });

  it("works on a flat termbase with no dates or markets", () => {
    const flat: Concept = {
      id: "x",
      terms: [term("Voucher", "en-US", "forbidden"), term("Coupon", "en-US", "preferred")],
    };
    const summary = constraintSummary(flat);
    expect(summary.banned.map((p) => p.text)).toEqual(["Voucher"]);
    expect(summary.preferred.map((p) => p.text)).toEqual(["Coupon"]);
  });
});
