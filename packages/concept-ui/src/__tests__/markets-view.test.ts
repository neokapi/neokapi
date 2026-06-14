import { describe, expect, it } from "vitest";
import { buildMarketView, orderLocaleTerms } from "../markets-view";
import { deriveMarketsFromTerms } from "../grouping";
import type { Market, Term } from "../types";

const term = (text: string, locale: string, status: Term["status"], market?: string): Term => ({
  text,
  locale,
  status,
  ...(market ? { validity: { tags: { market } } } : {}),
});

describe("orderLocaleTerms", () => {
  it("orders most-blessed → most-restricted, stable for equal status", () => {
    const ordered = orderLocaleTerms([
      term("Voucher", "en-US", "forbidden"),
      term("Coupon", "en-US", "preferred"),
      term("Promo code", "en-US", "approved"),
    ]);
    expect(ordered.map((t) => t.text)).toEqual(["Coupon", "Promo code", "Voucher"]);
  });
});

describe("buildMarketView", () => {
  const markets: Market[] = [
    { id: "dach", name: "DACH", description: "DE/AT/CH", locales: ["de-DE", "de-AT"] },
    { id: "france", name: "France", locales: ["fr-FR"] },
  ];

  it("annotates each market with banned/preferred flags and a primary term", () => {
    const views = buildMarketView(
      [
        term("Kasse", "de-DE", "preferred", "dach"),
        term("Validation", "fr-FR", "deprecated", "france"),
        term("Paiement", "fr-FR", "approved", "france"),
      ],
      markets,
    );
    const dach = views.find((v) => v.name === "DACH")!;
    expect(dach.description).toBe("DE/AT/CH");
    expect(dach.hasPreferred).toBe(true);
    expect(dach.hasBanned).toBe(false);

    const france = views.find((v) => v.name === "France")!;
    expect(france.hasBanned).toBe(true);
    // The deprecated term is present but not the wording to lead with.
    const fr = france.locales[0];
    expect(fr.primary.text).toBe("Paiement");
    expect(fr.terms.map((t) => t.text)).toEqual(["Paiement", "Validation"]);
    expect(france.termCount).toBe(2);
  });

  it("buckets locales covered by no market under 'Other locales'", () => {
    const views = buildMarketView(
      [term("Checkout", "en-US", "preferred"), term("Kasse", "de-DE", "preferred", "dach")],
      markets,
    );
    expect(views.map((v) => v.name)).toEqual(["DACH", "Other locales"]);
    const other = views.find((v) => v.market === null)!;
    expect(other.locales[0].locale).toBe("en-US");
  });

  it("works with markets derived from validity tags (framework-only mode)", () => {
    const terms = [
      term("Kasse", "de-DE", "preferred", "dach"),
      term("Paiement", "fr-FR", "approved", "france"),
      term("Checkout", "en-US", "preferred"), // untagged → Other locales
    ];
    const derived = deriveMarketsFromTerms(terms);
    const views = buildMarketView(terms, derived);
    expect(views.map((v) => v.name)).toEqual(["dach", "france", "Other locales"]);
    expect(views[0].localeCount).toBe(1);
  });
});
