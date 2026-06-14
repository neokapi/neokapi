import { describe, expect, it } from "vitest";
import { isBannedStatus, isGovernedRelation, primaryName, relationLabel } from "../concept-meta";
import type { ConceptSummary } from "../types";

const summary = (terms: ConceptSummary["terms"], domain?: string): ConceptSummary => ({
  id: "c",
  domain,
  terms,
});

describe("primaryName", () => {
  it("prefers a preferred term", () => {
    const name = primaryName(
      summary([
        { text: "Voucher", locale: "en-US", status: "forbidden" },
        { text: "Coupon", locale: "en-US", status: "preferred" },
      ]),
    );
    expect(name).toBe("Coupon");
  });

  it("falls back to an English term, then the first term", () => {
    expect(
      primaryName(
        summary([
          { text: "Gutschein", locale: "de-DE", status: "approved" },
          { text: "Coupon", locale: "en-US", status: "approved" },
        ]),
      ),
    ).toBe("Coupon");
    expect(primaryName(summary([{ text: "Panier", locale: "fr-FR", status: "approved" }]))).toBe(
      "Panier",
    );
  });

  it("falls back to the domain, then the id, with no terms", () => {
    expect(primaryName(summary([], "commerce"))).toBe("commerce");
    expect(primaryName(summary([]))).toBe("c");
  });
});

describe("status & relation predicates", () => {
  it("treats forbidden and deprecated as banned", () => {
    expect(isBannedStatus("forbidden")).toBe(true);
    expect(isBannedStatus("deprecated")).toBe(true);
    expect(isBannedStatus("preferred")).toBe(false);
  });

  it("marks REPLACED_BY as governed", () => {
    expect(isGovernedRelation("REPLACED_BY")).toBe(true);
    expect(isGovernedRelation("RELATED")).toBe(false);
  });

  it("labels relations for reading", () => {
    expect(relationLabel("USE_INSTEAD")).toBe("use instead");
  });
});
