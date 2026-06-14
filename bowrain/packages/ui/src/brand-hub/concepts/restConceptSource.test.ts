import { describe, it, expect, vi } from "vitest";
import { resolveCapabilities } from "@neokapi/concept-ui";
import type { ApiAdapter } from "../../api/adapter";
import type { ConceptInfo, TermSearchResult } from "../../types/api";
import type {
  Comment as GraphComment,
  ConceptRelation,
  ConceptStory,
  ConceptUsage,
  Market as GraphMarket,
  Observation as GraphObservation,
} from "../../types/brand-graph";
import {
  asGovernedEditError,
  createRestConceptSource,
  GovernedEditError,
  isGovernedEditError,
} from "./restConceptSource";

const WS = "acme";

function conceptInfo(over: Partial<ConceptInfo> = {}): ConceptInfo {
  return {
    id: "c1",
    project_id: "p1",
    domain: "Brand",
    definition: "The flagship product.",
    terms: [
      { text: "Skyline", locale: "en-US", status: "preferred", part_of_speech: "noun" },
      { text: "Horizon", locale: "de-DE", status: "admitted" },
    ],
    properties: { owner: "brand" },
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-02-01T00:00:00Z",
    ...over,
  };
}

function relation(over: Partial<ConceptRelation> = {}): ConceptRelation {
  return {
    id: "r1",
    source_id: "c1",
    target_id: "c2",
    relation_type: "RELATED",
    note: "see also",
    validity: { valid_from: "2025-01-01T00:00:00Z", tags: { market: "dach" } },
    created_at: "2025-01-01T00:00:00Z",
    ...over,
  };
}

/** A fake ApiAdapter exposing only the slice the source uses. */
function fakeAdapter(over: Partial<ApiAdapter> = {}): ApiAdapter {
  const base = {
    listConcepts: vi.fn(
      async (): Promise<TermSearchResult> => ({ concepts: [conceptInfo()], total_count: 1 }),
    ),
    getConcept: vi.fn(async () => conceptInfo()),
    listConceptRelations: vi.fn(async (): Promise<ConceptRelation[]> => [relation()]),
    listMarkets: vi.fn(
      async (): Promise<GraphMarket[]> => [
        {
          id: "m1",
          workspace_id: "w",
          name: "DACH",
          description: "German-speaking",
          locales: ["de-DE", "de-AT"],
          created_at: "x",
          updated_at: "y",
        },
      ],
    ),
    listObservations: vi.fn(
      async (): Promise<GraphObservation[]> => [
        {
          id: "o1",
          workspace_id: "w",
          concept_id: "c1",
          kind: "competitor",
          quote: "they say sky",
          source: "Acme site",
          url: "https://acme.example",
          created_by: "alice",
          created_at: "2025-03-01T00:00:00Z",
        },
      ],
    ),
    listConceptComments: vi.fn(
      async (): Promise<GraphComment[]> => [
        {
          id: "cm1",
          workspace_id: "w",
          concept_id: "c1",
          parent_id: "",
          body: "looks good",
          author: "bob",
          created_at: "2025-03-02T00:00:00Z",
          resolved: false,
        },
      ],
    ),
    getConceptStory: vi.fn(
      async (): Promise<ConceptStory> => ({
        concept_id: "c1",
        entries: [
          { kind: "revision", at: "2025-01-01T00:00:00Z", actor: "alice", summary: "Created" },
          {
            kind: "comment",
            at: "2025-03-02T00:00:00Z",
            actor: "bob",
            data: { body: "looks good" },
          },
        ],
      }),
    ),
    getConceptBlastRadius: vi.fn(
      async (): Promise<ConceptUsage> => ({
        concept_id: "c1",
        // total_blocks is the workspace scan denominator (rows scanned); blocks
        // is the concept-hit count. They differ so the mapping can't conflate them.
        total_blocks: 200,
        blocks: 12,
        occurrences: 30,
        words: 240,
        projects: [],
        samples: [],
      }),
    ),
    addConceptRelation: vi.fn(async () => relation({ id: "r-new", target_id: "c9" })),
    deleteConceptRelation: vi.fn(async () => undefined),
    updateConcept: vi.fn(async () => undefined),
  };
  return { ...base, ...over } as unknown as ApiAdapter;
}

describe("createRestConceptSource", () => {
  it("advertises every capability", () => {
    const source = createRestConceptSource(fakeAdapter(), WS);
    expect(resolveCapabilities(source)).toEqual({
      editTerms: true,
      editRelations: true,
      markets: true,
      observations: true,
      comments: true,
      timeline: true,
      whereUsed: true,
    });
  });

  it("maps listConcepts query → params and DTOs → summaries", async () => {
    const adapter = fakeAdapter();
    const source = createRestConceptSource(adapter, WS);

    const result = await source.listConcepts({ text: "sky", status: "preferred", limit: 50 });

    expect(adapter.listConcepts).toHaveBeenCalledWith(WS, {
      q: "sky",
      status: "preferred",
      limit: 50,
    });
    expect(result.total).toBe(1);
    expect(result.concepts[0]).toMatchObject({
      id: "c1",
      domain: "Brand",
      definition: "The flagship product.",
    });
    expect(result.concepts[0].terms[0]).toMatchObject({
      text: "Skyline",
      locale: "en-US",
      status: "preferred",
      partOfSpeech: "noun",
    });
  });

  it("maps getConcept and returns null on a 404", async () => {
    const ok = createRestConceptSource(fakeAdapter(), WS);
    expect(await ok.getConcept("c1")).toMatchObject({
      id: "c1",
      updatedAt: "2025-02-01T00:00:00Z",
    });

    const missing = createRestConceptSource(
      fakeAdapter({ getConcept: vi.fn(async () => Promise.reject(new Error("404: not found"))) }),
      WS,
    );
    expect(await missing.getConcept("nope")).toBeNull();
  });

  it("maps relations and remembers each relation's owning concept for removal", async () => {
    const adapter = fakeAdapter();
    const source = createRestConceptSource(adapter, WS);

    const relations = await source.getRelations("c1");
    expect(relations[0]).toMatchObject({
      id: "r1",
      sourceId: "c1",
      targetId: "c2",
      type: "RELATED",
      validity: { validFrom: "2025-01-01T00:00:00Z", tags: { market: "dach" } },
    });

    await source.removeRelation!("r1");
    expect(adapter.deleteConceptRelation).toHaveBeenCalledWith(WS, "c1", "r1");
  });

  it("throws when removing a relation whose concept was never listed", async () => {
    const source = createRestConceptSource(fakeAdapter(), WS);
    await expect(source.removeRelation!("unknown")).rejects.toThrow(/no known owning concept/);
  });

  it("maps the rich reads to concept-ui shapes", async () => {
    const source = createRestConceptSource(fakeAdapter(), WS);

    expect((await source.getMarkets!())[0]).toEqual({
      id: "m1",
      name: "DACH",
      description: "German-speaking",
      locales: ["de-DE", "de-AT"],
    });
    expect((await source.getObservations!("c1"))[0]).toMatchObject({
      id: "o1",
      kind: "competitor",
      quote: "they say sky",
      actor: "alice",
      at: "2025-03-01T00:00:00Z",
    });
    expect((await source.getComments!("c1"))[0]).toMatchObject({
      id: "cm1",
      body: "looks good",
      author: "bob",
      resolved: false,
    });
    const timeline = await source.getTimeline!("c1");
    expect(timeline[0]).toMatchObject({ kind: "revision", summary: "Created" });
    expect(timeline[1]).toMatchObject({
      kind: "comment",
      summary: "Comment posted",
      detail: "looks good",
    });
    expect(await source.getWhereUsed!("c1")).toEqual({
      conceptId: "c1",
      blocks: 12,
      occurrences: 30,
      words: 240,
    });
  });

  it("resolves a neighbour label via getConceptSummary and null on a 404", async () => {
    const adapter = fakeAdapter({
      getConcept: vi.fn(async () => conceptInfo({ id: "c2", definition: "Neighbour concept." })),
    });
    const source = createRestConceptSource(adapter, WS);

    const summary = await source.getConceptSummary!("c2");
    expect(adapter.getConcept).toHaveBeenCalledWith(WS, "c2");
    expect(summary).toMatchObject({
      id: "c2",
      domain: "Brand",
      definition: "Neighbour concept.",
    });
    // The summary is reduced — it carries no full-concept fields like properties.
    expect(summary).not.toHaveProperty("properties");
    expect(summary?.terms[0]).toMatchObject({ text: "Skyline", status: "preferred" });

    const missing = createRestConceptSource(
      fakeAdapter({ getConcept: vi.fn(async () => Promise.reject(new Error("404: not found"))) }),
      WS,
    );
    expect(await missing.getConceptSummary!("nope")).toBeNull();
  });

  it("adds an ordinary relation and maps the result", async () => {
    const adapter = fakeAdapter();
    const source = createRestConceptSource(adapter, WS);

    const created = await source.addRelation!("c1", { targetId: "c9", type: "RELATED" });

    expect(adapter.addConceptRelation).toHaveBeenCalledWith(WS, "c1", {
      target_id: "c9",
      relation_type: "RELATED",
      note: undefined,
      validity: undefined,
    });
    expect(created).toMatchObject({ id: "r-new", targetId: "c9" });

    // The new relation's owner is recorded, so it can be removed without listing.
    await source.removeRelation!("r-new");
    expect(adapter.deleteConceptRelation).toHaveBeenCalledWith(WS, "c1", "r-new");
  });

  it("turns a governed addRelation 409 into a GovernedEditError and notifies", async () => {
    const onGovernedEdit = vi.fn();
    const adapter = fakeAdapter({
      addConceptRelation: vi.fn(async () =>
        Promise.reject(
          new Error(
            '409: {"error":"governed change requires a change-set","detail":"a REPLACED_BY relation","hint":"open a change-set"}',
          ),
        ),
      ),
    });
    const source = createRestConceptSource(adapter, WS, { onGovernedEdit });

    await expect(
      source.addRelation!("c1", { targetId: "c2", type: "REPLACED_BY" }),
    ).rejects.toBeInstanceOf(GovernedEditError);

    expect(onGovernedEdit).toHaveBeenCalledTimes(1);
    const error = onGovernedEdit.mock.calls[0][0] as GovernedEditError;
    expect(error.conceptId).toBe("c1");
    expect(error.detail).toBe("a REPLACED_BY relation");
    expect(error.message).toMatch(/needs review/i);
  });

  it("applies an ordinary setTermStatus via a whole-concept update", async () => {
    const adapter = fakeAdapter();
    const source = createRestConceptSource(adapter, WS);

    await source.setTermStatus!("c1", { locale: "de-DE", text: "Horizon" }, "approved");

    expect(adapter.updateConcept).toHaveBeenCalledWith(WS, {
      project_id: "p1",
      concept_id: "c1",
      domain: "Brand",
      definition: "The flagship product.",
      terms: [
        { text: "Skyline", locale: "en-US", status: "preferred", part_of_speech: "noun" },
        { text: "Horizon", locale: "de-DE", status: "approved" },
      ],
    });
  });

  it("turns a governed setTermStatus 409 (desktop proxy form) into a GovernedEditError", async () => {
    const onGovernedEdit = vi.fn();
    const adapter = fakeAdapter({
      updateConcept: vi.fn(async () =>
        Promise.reject(
          new Error(
            'PUT /api/v1/acme/concepts/c1: server returned 409: {"detail":"a term status transition to/from forbidden or preferred","error":"governed change requires a change-set","hint":"open a change-set"}',
          ),
        ),
      ),
    });
    const source = createRestConceptSource(adapter, WS, { onGovernedEdit });

    await expect(
      source.setTermStatus!("c1", { locale: "en-US", text: "Skyline" }, "forbidden"),
    ).rejects.toBeInstanceOf(GovernedEditError);
    expect(onGovernedEdit).toHaveBeenCalledTimes(1);
    expect((onGovernedEdit.mock.calls[0][0] as GovernedEditError).detail).toMatch(/forbidden/);
  });

  it("re-throws non-governed errors unchanged", async () => {
    const onGovernedEdit = vi.fn();
    const adapter = fakeAdapter({
      addConceptRelation: vi.fn(async () =>
        Promise.reject(new Error('409: {"error":"stale draft conflict"}')),
      ),
    });
    const source = createRestConceptSource(adapter, WS, { onGovernedEdit });

    const error = await source.addRelation!("c1", { targetId: "c2", type: "RELATED" }).catch(
      (e: unknown) => e,
    );
    expect(isGovernedEditError(error)).toBe(false);
    expect(onGovernedEdit).not.toHaveBeenCalled();
  });
});

describe("asGovernedEditError", () => {
  it("recognises the governed-conflict body", () => {
    const err = asGovernedEditError(
      new Error(
        '409: {"error":"governed change requires a change-set","detail":"deleting a concept"}',
      ),
      "c5",
    );
    expect(err).toBeInstanceOf(GovernedEditError);
    expect(err?.conceptId).toBe("c5");
    expect(err?.detail).toBe("deleting a concept");
  });

  it("ignores non-409 and non-governed 409 errors", () => {
    expect(asGovernedEditError(new Error("500: boom"), "c1")).toBeNull();
    expect(asGovernedEditError(new Error('409: {"error":"version conflict"}'), "c1")).toBeNull();
  });
});
