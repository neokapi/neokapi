import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * URL/method/body construction for the Brand knowledge-graph (AD-021) REST
 * surface: concepts + graph + markets + change-sets. Each method is driven
 * against a stub fetch and the resulting request is asserted, so a route or
 * verb regression is caught without a live server.
 */
describe("RestApiAdapter — brand knowledge graph routes", () => {
  const BASE = "https://api.example";
  const originalFetch = globalThis.fetch;
  let fetchMock: ReturnType<typeof vi.fn>;
  let adapter: RestApiAdapter;

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({}), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    adapter = new RestApiAdapter(BASE);
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  /** The last request: [url, method, parsed-body]. */
  function lastRequest(): { url: string; method: string; body: unknown } {
    const calls = fetchMock.mock.calls;
    const [url, init] = calls[calls.length - 1] as [string, RequestInit];
    const bodyStr = init?.body as string | undefined;
    return {
      url,
      method: (init?.method ?? "GET").toUpperCase(),
      body: bodyStr ? JSON.parse(bodyStr) : undefined,
    };
  }

  // ── Concepts ───────────────────────────────────────────────────────────────

  it("listConcepts builds the /concepts query from params", async () => {
    await adapter.listConcepts("acme", {
      q: "cloud",
      status: "forbidden",
      domain: "marketing",
      market: "dach",
      locale: "en-US",
      source: "brand_vocabulary",
      stream: "review",
      project_id: "p1",
      offset: 20,
      limit: 50,
    });
    const { url, method } = lastRequest();
    expect(method).toBe("GET");
    expect(url.startsWith(`${BASE}/api/v1/acme/concepts?`)).toBe(true);
    const qs = new URL(url).searchParams;
    expect(qs.get("q")).toBe("cloud");
    expect(qs.get("status")).toBe("forbidden");
    expect(qs.get("domain")).toBe("marketing");
    expect(qs.get("market")).toBe("dach");
    expect(qs.get("locale")).toBe("en-US");
    expect(qs.get("source")).toBe("brand_vocabulary");
    expect(qs.get("stream")).toBe("review");
    expect(qs.get("project_id")).toBe("p1");
    expect(qs.get("offset")).toBe("20");
    expect(qs.get("limit")).toBe("50");
  });

  it("listConcepts with no params hits the bare /concepts route", async () => {
    await adapter.listConcepts("acme");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/concepts`);
  });

  it("getConcept encodes the concept id", async () => {
    await adapter.getConcept("acme", "c/1");
    const { url, method } = lastRequest();
    expect(method).toBe("GET");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c%2F1`);
  });

  it("createConcept POSTs to /concepts", async () => {
    await adapter.createConcept("acme", {
      project_id: "",
      domain: "d",
      definition: "def",
      terms: [],
    });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts`);
    expect(body).toMatchObject({ domain: "d", definition: "def" });
  });

  it("getConceptStory hits /story", async () => {
    await adapter.getConceptStory("acme", "c1");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/concepts/c1/story`);
  });

  it("listConceptRelations applies the as_of + market scope", async () => {
    await adapter.listConceptRelations("acme", "c1", {
      as_of: "2026-01-01T00:00:00Z",
      market: "dach",
    });
    const { url, method } = lastRequest();
    expect(method).toBe("GET");
    const u = new URL(url);
    expect(u.pathname).toBe("/api/v1/acme/concepts/c1/relations");
    expect(u.searchParams.get("as_of")).toBe("2026-01-01T00:00:00Z");
    expect(u.searchParams.get("market")).toBe("dach");
  });

  it("listConceptRelations with no scope omits the query", async () => {
    await adapter.listConceptRelations("acme", "c1");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/concepts/c1/relations`);
  });

  it("addConceptRelation POSTs the relation body", async () => {
    await adapter.addConceptRelation("acme", "c1", {
      target_id: "c2",
      relation_type: "RELATED",
      note: "n",
    });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/relations`);
    expect(body).toEqual({ target_id: "c2", relation_type: "RELATED", note: "n" });
  });

  it("deleteConceptRelation DELETEs the relation by id", async () => {
    await adapter.deleteConceptRelation("acme", "c1", "r9");
    const { url, method } = lastRequest();
    expect(method).toBe("DELETE");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/relations/r9`);
  });

  it("getConceptBlastRadius hits /blast-radius", async () => {
    await adapter.getConceptBlastRadius("acme", "c1");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/concepts/c1/blast-radius`);
  });

  it("addObservation POSTs to /observations", async () => {
    await adapter.addObservation("acme", "c1", {
      kind: "competitor",
      quote: "q",
      source: "s",
    });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/observations`);
    expect(body).toMatchObject({ kind: "competitor", quote: "q", source: "s" });
  });

  it("deleteObservation DELETEs by id", async () => {
    await adapter.deleteObservation("acme", "c1", "o3");
    const { url, method } = lastRequest();
    expect(method).toBe("DELETE");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/observations/o3`);
  });

  it("addConceptComment POSTs to /comments", async () => {
    await adapter.addConceptComment("acme", "c1", { body: "hi", parent_id: "p" });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/comments`);
    expect(body).toEqual({ body: "hi", parent_id: "p" });
  });

  it("resolveConceptComment defaults to an empty body (server defaults true)", async () => {
    await adapter.resolveConceptComment("acme", "c1", "cm1");
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/comments/cm1/resolve`);
    expect(body).toEqual({});
  });

  it("resolveConceptComment passes resolved=false explicitly", async () => {
    await adapter.resolveConceptComment("acme", "c1", "cm1", false);
    expect(lastRequest().body).toEqual({ resolved: false });
  });

  it("deleteConceptComment DELETEs by id", async () => {
    await adapter.deleteConceptComment("acme", "c1", "cm1");
    const { url, method } = lastRequest();
    expect(method).toBe("DELETE");
    expect(url).toBe(`${BASE}/api/v1/acme/concepts/c1/comments/cm1`);
  });

  // ── Markets ──────────────────────────────────────────────────────────────

  it("listMarkets GETs /markets", async () => {
    await adapter.listMarkets("acme");
    const { url, method } = lastRequest();
    expect(method).toBe("GET");
    expect(url).toBe(`${BASE}/api/v1/acme/markets`);
  });

  it("createMarket POSTs the market body", async () => {
    await adapter.createMarket("acme", { name: "dach", locales: ["de-DE", "de-AT"] });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/markets`);
    expect(body).toEqual({ name: "dach", locales: ["de-DE", "de-AT"] });
  });

  it("updateMarket PUTs /markets/:mid", async () => {
    await adapter.updateMarket("acme", "m1", { name: "dach2" });
    const { url, method } = lastRequest();
    expect(method).toBe("PUT");
    expect(url).toBe(`${BASE}/api/v1/acme/markets/m1`);
  });

  it("deleteMarket DELETEs /markets/:mid", async () => {
    await adapter.deleteMarket("acme", "m1");
    const { url, method } = lastRequest();
    expect(method).toBe("DELETE");
    expect(url).toBe(`${BASE}/api/v1/acme/markets/m1`);
  });

  // ── Change-sets ──────────────────────────────────────────────────────────

  it("listChangesets filters by status", async () => {
    await adapter.listChangesets("acme", "in_review");
    const { url, method } = lastRequest();
    expect(method).toBe("GET");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets?status=in_review`);
  });

  it("listChangesets without a status hits the bare collection", async () => {
    await adapter.listChangesets("acme");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/changesets`);
  });

  it("getChangeset GETs the detail route", async () => {
    await adapter.getChangeset("acme", "cs1");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/changesets/cs1`);
  });

  it("createChangeset POSTs name + description", async () => {
    await adapter.createChangeset("acme", { name: "Rename cloud", description: "d" });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets`);
    expect(body).toEqual({ name: "Rename cloud", description: "d" });
  });

  it("patchChangeset PATCHes the header", async () => {
    await adapter.patchChangeset("acme", "cs1", { name: "new" });
    const { url, method, body } = lastRequest();
    expect(method).toBe("PATCH");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1`);
    expect(body).toEqual({ name: "new" });
  });

  it("appendChangesetOp POSTs a typed op to /ops", async () => {
    await adapter.appendChangesetOp("acme", "cs1", {
      op: "term.status",
      payload: {
        concept_id: "c1",
        locale: "en-US",
        text: "rival",
        from: "approved",
        to: "forbidden",
      },
      base_rev: 4,
    });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/ops`);
    expect(body).toMatchObject({ op: "term.status", base_rev: 4 });
  });

  it("removeChangesetOp DELETEs /ops/:seq", async () => {
    await adapter.removeChangesetOp("acme", "cs1", 2);
    const { url, method } = lastRequest();
    expect(method).toBe("DELETE");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/ops/2`);
  });

  it("submitChangeset POSTs /submit", async () => {
    await adapter.submitChangeset("acme", "cs1");
    const { url, method } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/submit`);
  });

  it("approveChangeset POSTs the reviewer comment", async () => {
    await adapter.approveChangeset("acme", "cs1", { comment: "lgtm" });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/approve`);
    expect(body).toEqual({ comment: "lgtm" });
  });

  it("approveChangeset with no review sends an empty body", async () => {
    await adapter.approveChangeset("acme", "cs1");
    expect(lastRequest().body).toEqual({});
  });

  it("rejectChangeset POSTs /reject", async () => {
    await adapter.rejectChangeset("acme", "cs1", { comment: "no" });
    const { url, method } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/reject`);
  });

  it("mergeChangeset POSTs /merge", async () => {
    await adapter.mergeChangeset("acme", "cs1");
    const { url, method } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/merge`);
  });

  it("abandonChangeset POSTs /abandon", async () => {
    await adapter.abandonChangeset("acme", "cs1");
    const { url, method } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/abandon`);
  });

  it("getChangesetBlastRadius GETs /blast-radius", async () => {
    await adapter.getChangesetBlastRadius("acme", "cs1");
    expect(lastRequest().url).toBe(`${BASE}/api/v1/acme/changesets/cs1/blast-radius`);
  });

  it("addPilot POSTs project_id + stream", async () => {
    await adapter.addPilot("acme", "cs1", { project_id: "p1", stream: "review" });
    const { url, method, body } = lastRequest();
    expect(method).toBe("POST");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/pilots`);
    expect(body).toEqual({ project_id: "p1", stream: "review" });
  });

  it("removePilot DELETEs /pilots/:project/:stream with encoding", async () => {
    await adapter.removePilot("acme", "cs1", "p1", "feature/x");
    const { url, method } = lastRequest();
    expect(method).toBe("DELETE");
    expect(url).toBe(`${BASE}/api/v1/acme/changesets/cs1/pilots/p1/feature%2Fx`);
  });
});
