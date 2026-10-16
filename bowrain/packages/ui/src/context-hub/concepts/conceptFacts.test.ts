import { describe, it, expect } from "vite-plus/test";
import type { ConceptSummary } from "@neokapi/concept-ui";
import type { ConceptStoryEntry } from "../../types/brand-graph";
import {
  coordinatesOf,
  discouragedTerms,
  headlineTerm,
  languageCount,
  languagesInOrder,
  marketsFor,
  provenanceOf,
  replacementFor,
} from "./conceptFacts";

const cart: ConceptSummary = {
  id: "c-cart",
  terms: [
    { text: "Warenkorb", locale: "de-DE", status: "approved" },
    { text: "basket", locale: "en-US", status: "deprecated" },
    { text: "cart", locale: "en-US", status: "preferred" },
    { text: "trolley", locale: "en-US", status: "forbidden" },
    { text: "panier", locale: "fr-FR", status: "proposed" },
  ],
};

describe("headlineTerm", () => {
  it("heads a concept with its source-language term, a usable one first", () => {
    expect(headlineTerm(cart, { sourceLocale: "en-US" })?.text).toBe("cart");
  });

  it("heads it in another language when the source names none", () => {
    const only: ConceptSummary = {
      id: "c-1",
      terms: [{ text: "panier", locale: "fr-FR", status: "proposed" }],
    };
    expect(headlineTerm(only, { sourceLocale: "en-US" })?.status).toBe("proposed");
  });
});

describe("languageCount", () => {
  it("counts each language once, whatever its terms' statuses", () => {
    expect(languageCount(cart)).toBe(3);
  });
});

describe("replacementFor", () => {
  it("gives a discouraged term the preferred term in its language", () => {
    const basket = cart.terms[1];
    expect(replacementFor(basket, cart.terms)?.text).toBe("cart");
  });

  it("falls back to any usable term in the language", () => {
    const terms = [
      { text: "Einkaufswagen", locale: "de-DE", status: "forbidden" as const },
      { text: "Warenkorb", locale: "de-DE", status: "approved" as const },
    ];
    expect(replacementFor(terms[0], terms)?.text).toBe("Warenkorb");
  });

  it("offers nothing when the language has no usable term", () => {
    const terms = [{ text: "QuickPay", locale: "en-US", status: "forbidden" as const }];
    expect(replacementFor(terms[0], terms)).toBeUndefined();
  });
});

describe("discouragedTerms and languagesInOrder", () => {
  it("separates the forms a concept discourages from the ones it uses", () => {
    expect(discouragedTerms(cart).map((t) => t.text)).toEqual(["basket", "trolley"]);
    const languages = languagesInOrder(cart);
    expect(languages.flatMap((g) => g.terms.map((t) => t.text))).toEqual([
      "Warenkorb",
      "cart",
      "panier",
    ]);
  });

  it("puts the source language first and keeps the store's order for the rest", () => {
    expect(languagesInOrder(cart, "en-US").map((g) => g.locale)).toEqual([
      "en-US",
      "de-DE",
      "fr-FR",
    ]);
  });
});

describe("coordinatesOf", () => {
  it("reads axis=value pairs and skips malformed ones", () => {
    expect(coordinatesOf({ coordinates: "brand=kapimart, channel=store,=x,bare" })).toEqual([
      ["brand", "kapimart"],
      ["channel", "store"],
    ]);
    expect(coordinatesOf(undefined)).toEqual([]);
  });
});

describe("provenanceOf", () => {
  const entry = (partial: Partial<ConceptStoryEntry>): ConceptStoryEntry => ({
    kind: "revision",
    at: "2026-06-01T09:00:00Z",
    ...partial,
  });

  it("names who added a concept and who revised it last", () => {
    const story = [
      entry({ kind: "comment", at: "2026-06-13T10:00:00Z", actor: "sam" }),
      entry({ kind: "revision", at: "2026-06-12T10:00:00Z", actor: "maria" }),
      entry({ kind: "revision", at: "2026-06-01T09:00:00Z", actor: "alex" }),
    ];
    const { added, changed } = provenanceOf(story);
    expect(added?.actor).toBe("alex");
    expect(changed?.actor).toBe("maria");
  });

  it("does not repeat the adder as the last change", () => {
    const { added, changed } = provenanceOf([entry({ actor: "alex" })]);
    expect(added?.actor).toBe("alex");
    expect(changed).toBeUndefined();
  });

  it("ignores entries that name nobody", () => {
    expect(provenanceOf([entry({})])).toEqual({});
    expect(provenanceOf(undefined)).toEqual({});
  });
});

describe("marketsFor", () => {
  it("names the markets that cover a locale", () => {
    const markets = [
      { name: "dach", locales: ["de-DE", "de-AT"] },
      { name: "eu", locales: ["de-DE", "fr-FR"] },
    ];
    expect(marketsFor("de-DE", markets)).toEqual(["dach", "eu"]);
    expect(marketsFor("en-US", markets)).toEqual([]);
    expect(marketsFor("de-DE", undefined)).toEqual([]);
  });
});
