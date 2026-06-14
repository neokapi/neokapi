import { describe, it, expect, vi } from "vitest";
import { deriveCapabilities } from "@neokapi/concept-ui";
import {
  createLocalConceptSource,
  type ConceptBackend,
  type ConceptDTO,
  type RelationDTO,
} from "../lib/localConceptSource";

const HANDLE = "tb-handle";

function concept(overrides: Partial<ConceptDTO> = {}): ConceptDTO {
  return {
    id: "c1",
    project_id: "proj-1",
    domain: "Brand",
    definition: "Flagship product.",
    source: "brand_vocabulary",
    terms: [
      {
        text: "Skyline",
        locale: "en-US",
        status: "preferred",
        part_of_speech: "noun",
        validity: { tags: { market: "dach" }, valid_from: "2025-01-01T00:00:00Z" },
      },
    ],
    properties: { owner: "brand" },
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-02-01T00:00:00Z",
    ...overrides,
  };
}

/** A backend whose every method is a recording stub returning a canned value. */
function fakeBackend(over: Partial<ConceptBackend> = {}): ConceptBackend {
  return {
    searchTerms: vi.fn().mockResolvedValue({ concepts: [concept()], total_count: 1 }),
    getConceptForView: vi.fn().mockResolvedValue(concept()),
    getRelations: vi.fn().mockResolvedValue([]),
    addRelation: vi.fn(),
    removeRelation: vi.fn().mockResolvedValue(undefined),
    setTermStatus: vi.fn().mockResolvedValue(undefined),
    updateConcept: vi.fn().mockResolvedValue(undefined),
    ...over,
  };
}

describe("createLocalConceptSource", () => {
  it("derives edit-on / rich-off capabilities", () => {
    const source = createLocalConceptSource(HANDLE, fakeBackend());
    const caps = deriveCapabilities(source);
    expect(caps.editRelations).toBe(true);
    expect(caps.editTerms).toBe(true);
    expect(caps.markets).toBe(false);
    expect(caps.observations).toBe(false);
    expect(caps.comments).toBe(false);
    expect(caps.timeline).toBe(false);
    expect(caps.whereUsed).toBe(false);
  });

  it("maps listConcepts via SearchTerms with paging defaults", async () => {
    const backend = fakeBackend();
    const source = createLocalConceptSource(HANDLE, backend);

    const result = await source.listConcepts({ text: "sky" });

    expect(backend.searchTerms).toHaveBeenCalledWith(HANDLE, "sky", "", "", 0, 200);
    expect(result.total).toBe(1);
    expect(result.concepts).toHaveLength(1);
    const c = result.concepts[0];
    expect(c.id).toBe("c1");
    expect(c.source).toBe("brand_vocabulary");
    expect(c.terms[0].partOfSpeech).toBe("noun");
    expect(c.terms[0].validity).toEqual({
      tags: { market: "dach" },
      validFrom: "2025-01-01T00:00:00Z",
    });
  });

  it("passes empty query text and honours explicit offset/limit", async () => {
    const backend = fakeBackend();
    const source = createLocalConceptSource(HANDLE, backend);
    await source.listConcepts({ offset: 40, limit: 20 });
    expect(backend.searchTerms).toHaveBeenCalledWith(HANDLE, "", "", "", 40, 20);
  });

  it("returns an empty result when the backend is unavailable", async () => {
    const backend = fakeBackend({ searchTerms: vi.fn().mockResolvedValue(null) });
    const source = createLocalConceptSource(HANDLE, backend);
    expect(await source.listConcepts({})).toEqual({ concepts: [], total: 0 });
  });

  it("maps getConcept snake_case → camelCase (and drops project_id)", async () => {
    const backend = fakeBackend();
    const source = createLocalConceptSource(HANDLE, backend);

    const c = await source.getConcept("c1");

    expect(backend.getConceptForView).toHaveBeenCalledWith(HANDLE, "c1");
    expect(c).not.toBeNull();
    expect(c).not.toHaveProperty("project_id");
    expect(c?.createdAt).toBe("2025-01-01T00:00:00Z");
    expect(c?.updatedAt).toBe("2025-02-01T00:00:00Z");
    expect(c?.properties).toEqual({ owner: "brand" });
    expect(c?.terms[0].partOfSpeech).toBe("noun");
  });

  it("returns null when the concept is missing", async () => {
    const backend = fakeBackend({ getConceptForView: vi.fn().mockResolvedValue(null) });
    const source = createLocalConceptSource(HANDLE, backend);
    expect(await source.getConcept("nope")).toBeNull();
  });

  it("maps getRelations to camelCase endpoints + validity", async () => {
    const rels: RelationDTO[] = [
      {
        id: "r1",
        source_id: "c1",
        target_id: "c2",
        type: "RELATED",
        note: "see also",
        validity: { valid_to: "2026-01-01T00:00:00Z" },
      },
    ];
    const backend = fakeBackend({ getRelations: vi.fn().mockResolvedValue(rels) });
    const source = createLocalConceptSource(HANDLE, backend);

    const out = await source.getRelations("c1");

    expect(backend.getRelations).toHaveBeenCalledWith(HANDLE, "c1");
    expect(out).toEqual([
      {
        id: "r1",
        sourceId: "c1",
        targetId: "c2",
        type: "RELATED",
        note: "see also",
        validity: { validTo: "2026-01-01T00:00:00Z" },
      },
    ]);
  });

  it("getRelations tolerates a null backend response", async () => {
    const backend = fakeBackend({ getRelations: vi.fn().mockResolvedValue(null) });
    const source = createLocalConceptSource(HANDLE, backend);
    expect(await source.getRelations("c1")).toEqual([]);
  });

  it("flattens RelationInput.validity into the AddRelationRequest and maps the result", async () => {
    const created: RelationDTO = {
      id: "r9",
      source_id: "c1",
      target_id: "c2",
      type: "BROADER",
      validity: { valid_from: "2025-03-01T00:00:00Z", tags: { channel: "web" } },
    };
    const backend = fakeBackend({ addRelation: vi.fn().mockResolvedValue(created) });
    const source = createLocalConceptSource(HANDLE, backend);

    const rel = await source.addRelation!("c1", {
      targetId: "c2",
      type: "BROADER",
      note: "parent",
      validity: { validFrom: "2025-03-01T00:00:00Z", tags: { channel: "web" } },
    });

    expect(backend.addRelation).toHaveBeenCalledWith(HANDLE, {
      source_id: "c1",
      target_id: "c2",
      type: "BROADER",
      note: "parent",
      valid_from: "2025-03-01T00:00:00Z",
      valid_to: undefined,
      tags: { channel: "web" },
    });
    expect(rel.id).toBe("r9");
    expect(rel.sourceId).toBe("c1");
    expect(rel.validity).toEqual({ validFrom: "2025-03-01T00:00:00Z", tags: { channel: "web" } });
  });

  it("throws when addRelation has no backend (outside Wails)", async () => {
    const backend = fakeBackend({ addRelation: vi.fn().mockResolvedValue(null) });
    const source = createLocalConceptSource(HANDLE, backend);
    await expect(source.addRelation!("c1", { targetId: "c2", type: "RELATED" })).rejects.toThrow(
      /backend unavailable/,
    );
  });

  it("removeRelation forwards the relation id", async () => {
    const backend = fakeBackend();
    const source = createLocalConceptSource(HANDLE, backend);
    await source.removeRelation!("r1");
    expect(backend.removeRelation).toHaveBeenCalledWith(HANDLE, "r1");
  });

  it("setTermStatus builds the request from the term ref + validity", async () => {
    const backend = fakeBackend();
    const source = createLocalConceptSource(HANDLE, backend);

    await source.setTermStatus!("c1", { locale: "fr-FR", text: "Horizon" }, "deprecated", {
      validTo: "2025-12-31T00:00:00Z",
      tags: { market: "fr" },
    });

    expect(backend.setTermStatus).toHaveBeenCalledWith(HANDLE, {
      concept_id: "c1",
      locale: "fr-FR",
      text: "Horizon",
      status: "deprecated",
      valid_from: undefined,
      valid_to: "2025-12-31T00:00:00Z",
      tags: { market: "fr" },
    });
  });

  it("updateTerm patches the matching term and re-persists the whole concept", async () => {
    const loaded = concept({
      terms: [
        { text: "Skyline", locale: "en-US", status: "preferred" },
        { text: "Horizon", locale: "fr-FR", status: "deprecated" },
      ],
    });
    const backend = fakeBackend({ getConceptForView: vi.fn().mockResolvedValue(loaded) });
    const source = createLocalConceptSource(HANDLE, backend);

    await source.updateTerm!(
      "c1",
      { locale: "fr-FR", text: "Horizon" },
      { status: "forbidden", note: "Do not use.", validity: { tags: { market: "fr" } } },
    );

    expect(backend.updateConcept).toHaveBeenCalledTimes(1);
    const req = (backend.updateConcept as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(req).toMatchObject({
      concept_id: "c1",
      project_id: "proj-1",
      domain: "Brand",
      definition: "Flagship product.",
    });
    // The full term list is re-sent; only the matched term changed.
    expect(req.terms).toHaveLength(2);
    expect(req.terms[0]).toMatchObject({ text: "Skyline", status: "preferred" });
    expect(req.terms[1]).toMatchObject({
      text: "Horizon",
      status: "forbidden",
      note: "Do not use.",
      validity: { tags: { market: "fr" } },
    });
  });

  it("updateTerm is a no-op when the term is not found", async () => {
    const backend = fakeBackend();
    const source = createLocalConceptSource(HANDLE, backend);
    await source.updateTerm!("c1", { locale: "es-ES", text: "Nope" }, { status: "approved" });
    expect(backend.updateConcept).not.toHaveBeenCalled();
  });
});
