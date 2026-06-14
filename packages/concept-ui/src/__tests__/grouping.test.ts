import { describe, expect, it } from "vitest";
import {
  RELATION_COLLAPSE_THRESHOLD,
  deriveMarketsFromTerms,
  groupRelations,
  shouldCollapse,
  termsByLocale,
  termsByMarket,
} from "../grouping";
import type { Market, Relation, Term } from "../types";

const rel = (id: string, sourceId: string, targetId: string, type: Relation["type"]): Relation => ({
  id,
  sourceId,
  targetId,
  type,
});

const term = (text: string, locale: string, status: Term["status"], market?: string): Term => ({
  text,
  locale,
  status,
  ...(market ? { validity: { tags: { market } } } : {}),
});

describe("groupRelations", () => {
  it("buckets by type in canonical order and resolves the far end", () => {
    const relations = [
      rel("a", "x", "y", "RELATED"),
      rel("b", "z", "x", "BROADER"), // incoming
      rel("c", "x", "w", "RELATED"),
    ];
    const groups = groupRelations(relations, "x");
    expect(groups.map((g) => g.type)).toEqual(["BROADER", "RELATED"]);

    const broader = groups[0];
    expect(broader.items).toHaveLength(1);
    expect(broader.items[0].otherId).toBe("z");
    expect(broader.items[0].outgoing).toBe(false);

    const related = groups[1];
    expect(related.items.map((i) => i.otherId)).toEqual(["y", "w"]);
    expect(related.items.every((i) => i.outgoing)).toBe(true);
  });

  it("ignores relations that do not touch the subject and self-loops", () => {
    const relations = [rel("a", "p", "q", "RELATED"), rel("b", "x", "x", "RELATED")];
    expect(groupRelations(relations, "x")).toEqual([]);
  });
});

describe("shouldCollapse", () => {
  it("collapses only past the threshold", () => {
    const items = Array.from({ length: RELATION_COLLAPSE_THRESHOLD + 1 }, (_, i) =>
      rel(`r${i}`, "x", `t${i}`, "RELATED"),
    );
    const [group] = groupRelations(items, "x");
    expect(shouldCollapse(group)).toBe(true);

    const fewer = groupRelations(items.slice(0, RELATION_COLLAPSE_THRESHOLD), "x");
    expect(shouldCollapse(fewer[0])).toBe(false);
  });
});

describe("termsByLocale", () => {
  it("groups terms by locale preserving first-seen order", () => {
    const groups = termsByLocale([
      term("Cart", "en-US", "preferred"),
      term("Warenkorb", "de-DE", "preferred"),
      term("Basket", "en-US", "admitted"),
    ]);
    expect(groups.map((g) => g.locale)).toEqual(["en-US", "de-DE"]);
    expect(groups[0].terms).toHaveLength(2);
  });
});

describe("termsByMarket", () => {
  const markets: Market[] = [
    { id: "dach", name: "DACH", locales: ["de-DE", "de-AT"] },
    { id: "us", name: "United States", locales: ["en-US"] },
  ];

  it("groups by covering market and trails uncovered locales", () => {
    const groups = termsByMarket(
      [
        term("Kasse", "de-DE", "preferred"),
        term("Checkout", "en-US", "preferred"),
        term("Caisse", "fr-CA", "approved"),
      ],
      markets,
    );
    expect(groups.map((g) => g.name)).toEqual(["DACH", "United States", "Other locales"]);
    expect(groups[2].market).toBeNull();
    expect(groups[2].locales[0].locale).toBe("fr-CA");
  });

  it("skips markets with no matching terms", () => {
    const groups = termsByMarket([term("Checkout", "en-US", "preferred")], markets);
    expect(groups.map((g) => g.name)).toEqual(["United States"]);
  });
});

describe("deriveMarketsFromTerms", () => {
  it("synthesises markets from the market validity tag", () => {
    const markets = deriveMarketsFromTerms([
      term("Kasse", "de-DE", "preferred", "dach"),
      term("Kassa", "de-AT", "approved", "dach"),
      term("Paiement", "fr-FR", "approved", "france"),
      term("Checkout", "en-US", "preferred"), // untagged → ignored
    ]);
    expect(markets).toEqual([
      { name: "dach", locales: ["de-DE", "de-AT"] },
      { name: "france", locales: ["fr-FR"] },
    ]);
  });
});
