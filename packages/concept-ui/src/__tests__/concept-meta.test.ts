import { describe, expect, it } from "vitest";
import { isBannedStatus, isGovernedRelation, primaryName, relationLabel } from "../concept-meta";
import type { ConceptSummary } from "../types";

const summary = (terms: ConceptSummary["terms"], domain?: string): ConceptSummary => ({
  id: "c",
  domain,
  terms,
});

// The KapiMart shape that made an English-source project read in Arabic: the
// store lists ar first and every locale carries a preferred term.
const multilingual = summary([
  { text: "مستودع", locale: "ar", status: "preferred" },
  { text: "Lager", locale: "de-DE", status: "preferred" },
  { text: "Warehouse", locale: "en-US", status: "preferred" },
  { text: "Entrepôt", locale: "fr-FR", status: "preferred" },
]);

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

  it("names the concept in the source locale", () => {
    expect(primaryName(multilingual, { sourceLocale: "en-US" })).toBe("Warehouse");
    expect(primaryName(multilingual, { sourceLocale: "de-DE" })).toBe("Lager");
  });

  it("matches a source locale on its language when no region matches", () => {
    expect(primaryName(multilingual, { sourceLocale: "en-GB" })).toBe("Warehouse");
    expect(primaryName(multilingual, { sourceLocale: "ar-EG" })).toBe("مستودع");
  });

  it("prefers an exact locale match over a match on the language alone", () => {
    const regional = summary([
      { text: "Colour", locale: "en-GB", status: "preferred" },
      { text: "Color", locale: "en-US", status: "preferred" },
    ]);
    expect(primaryName(regional, { sourceLocale: "en-US" })).toBe("Color");
    expect(primaryName(regional, { sourceLocale: "en-GB" })).toBe("Colour");
  });

  it("takes any term in the source locale when none there is preferred", () => {
    const mixed = summary([
      { text: "مستودع", locale: "ar", status: "preferred" },
      { text: "Warehouse", locale: "en-US", status: "admitted" },
    ]);
    expect(primaryName(mixed, { sourceLocale: "en-US" })).toBe("Warehouse");
  });

  it("falls back to the viewer's locale when the source locale has no term", () => {
    expect(primaryName(multilingual, { sourceLocale: "nb-NO", uiLocale: "fr-FR" })).toBe(
      "Entrepôt",
    );
  });

  it("falls back to English when neither locale has a term", () => {
    expect(primaryName(multilingual, { sourceLocale: "nb-NO", uiLocale: "sv-SE" })).toBe(
      "Warehouse",
    );
  });

  it("falls back to English with no locale hints at all", () => {
    expect(primaryName(multilingual)).toBe("Warehouse");
    expect(
      primaryName(
        summary([
          { text: "Gutschein", locale: "de-DE", status: "approved" },
          { text: "Coupon", locale: "en-US", status: "approved" },
        ]),
      ),
    ).toBe("Coupon");
  });

  it("prefers an English preferred term over an English admitted one", () => {
    const both = summary([
      { text: "Bin", locale: "en-US", status: "admitted" },
      { text: "Warehouse", locale: "en", status: "preferred" },
    ]);
    expect(primaryName(both)).toBe("Warehouse");
  });

  it("takes the first preferred term in locale order when no locale hint lands", () => {
    const noEnglish = summary([
      { text: "Entrepôt", locale: "fr-FR", status: "admitted" },
      { text: "مستودع", locale: "ar", status: "preferred" },
      { text: "Lager", locale: "de-DE", status: "preferred" },
    ]);
    expect(primaryName(noEnglish)).toBe("مستودع");
  });

  it("names the same concept the same way whatever order the store lists terms in", () => {
    const forward = summary([
      { text: "Entrepôt", locale: "fr-FR", status: "admitted" },
      { text: "Lager", locale: "de-DE", status: "admitted" },
    ]);
    const reversed = summary([...forward.terms].reverse());
    expect(primaryName(forward)).toBe(primaryName(reversed));
    expect(primaryName(forward)).toBe("Lager"); // de-DE sorts before fr-FR
  });

  it("falls back to the first term, then the domain, then the id", () => {
    expect(primaryName(summary([{ text: "Panier", locale: "fr-FR", status: "approved" }]))).toBe(
      "Panier",
    );
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
