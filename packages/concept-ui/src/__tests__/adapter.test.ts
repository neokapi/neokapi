import { describe, expect, it } from "vitest";
import { deriveCapabilities, resolveCapabilities } from "../adapter";
import type { ConceptCapabilities, ConceptDataSource } from "../adapter";

// A minimal source implementing only the three required core reads. Every
// optional method is absent, so a correct derivation must report no rich
// capabilities and no edit affordances — this is the kapi-desktop / local
// SQLite floor the whole graceful-degradation contract rests on.
const core: ConceptDataSource = {
  listConcepts: () => ({ concepts: [], total: 0 }),
  getConcept: () => null,
  getRelations: () => [],
};

/** core plus the named optional methods (each a no-op of the right shape). */
function sourceWith(extra: Partial<ConceptDataSource>): ConceptDataSource {
  return { ...core, ...extra };
}

const ALL_FALSE: ConceptCapabilities = {
  editTerms: false,
  editRelations: false,
  markets: false,
  observations: false,
  comments: false,
  timeline: false,
  whereUsed: false,
};

describe("deriveCapabilities", () => {
  it("reports nothing for a core-only source", () => {
    expect(deriveCapabilities(core)).toEqual(ALL_FALSE);
  });

  it("enables editTerms when either term mutation is present", () => {
    expect(deriveCapabilities(sourceWith({ updateTerm: () => {} })).editTerms).toBe(true);
    expect(deriveCapabilities(sourceWith({ setTermStatus: () => {} })).editTerms).toBe(true);
  });

  it("enables editRelations only when BOTH add and remove are present", () => {
    expect(deriveCapabilities(sourceWith({ addRelation: () => ({}) as never })).editRelations).toBe(
      false,
    );
    expect(deriveCapabilities(sourceWith({ removeRelation: () => {} })).editRelations).toBe(false);
    expect(
      deriveCapabilities(sourceWith({ addRelation: () => ({}) as never, removeRelation: () => {} }))
        .editRelations,
    ).toBe(true);
  });

  it("gates each rich read on its own method", () => {
    expect(deriveCapabilities(sourceWith({ getMarkets: () => [] })).markets).toBe(true);
    expect(deriveCapabilities(sourceWith({ getObservations: () => [] })).observations).toBe(true);
    expect(deriveCapabilities(sourceWith({ getComments: () => [] })).comments).toBe(true);
    expect(deriveCapabilities(sourceWith({ getTimeline: () => [] })).timeline).toBe(true);
    expect(deriveCapabilities(sourceWith({ getWhereUsed: () => null })).whereUsed).toBe(true);
  });

  it("does not let one rich read enable the others", () => {
    const caps = deriveCapabilities(sourceWith({ getMarkets: () => [] }));
    expect(caps).toEqual({ ...ALL_FALSE, markets: true });
  });

  it("ignores getConceptSummary (a labelling helper, not a capability)", () => {
    expect(deriveCapabilities(sourceWith({ getConceptSummary: () => null }))).toEqual(ALL_FALSE);
  });
});

describe("resolveCapabilities", () => {
  it("equals the derivation when no explicit overrides are given", () => {
    const src = sourceWith({ getMarkets: () => [], getComments: () => [] });
    expect(resolveCapabilities(src)).toEqual(deriveCapabilities(src));
  });

  it("lets an explicit override hide a method that is present", () => {
    const src = sourceWith({ getMarkets: () => [], capabilities: { markets: false } });
    expect(deriveCapabilities(src).markets).toBe(true); // method is present
    expect(resolveCapabilities(src).markets).toBe(false); // but advertised off
  });

  it("applies a partial override and leaves the rest derived", () => {
    const src = sourceWith({
      getTimeline: () => [],
      getComments: () => [],
      capabilities: { comments: false },
    });
    const caps = resolveCapabilities(src);
    expect(caps.timeline).toBe(true); // still derived from the method
    expect(caps.comments).toBe(false); // overridden off
  });

  it("can advertise a capability the method-presence derivation would not set", () => {
    const src = sourceWith({ capabilities: { editTerms: true } });
    expect(resolveCapabilities(src).editTerms).toBe(true);
  });
});
