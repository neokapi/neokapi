import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";
import type { FlowDefinitionInfo } from "../types/api";

/**
 * Wiring tests for the flow-definition REST methods (Bowrain AD-013). These
 * back the web superset flow editor: they must hit /api/v1/{ws}/{projectId}/flows
 * with the right verbs so the editor can list/create/update/delete the
 * server-side, connector-agnostic flow definitions automation run_flow actions
 * reference.
 */
describe("RestApiAdapter flow definitions", () => {
  const originalFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  function jsonResponse(body: unknown, status = 200) {
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  }

  const sampleDef: FlowDefinitionInfo = {
    id: "flow-1",
    name: "Custom",
    source: "project",
    nodes: [],
    edges: [],
  };

  it("listFlowDefinitions GETs the project flows endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse([sampleDef]));
    globalThis.fetch = fetchMock;

    const adapter = new RestApiAdapter("https://api.example");
    const defs = await adapter.listFlowDefinitions("acme", "proj-1");

    expect(defs).toEqual([sampleDef]);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://api.example/api/v1/acme/proj-1/flows");
    expect((init as RequestInit).method ?? "GET").toBe("GET");
  });

  it("createFlowDefinition POSTs the definition", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleDef, 201));
    globalThis.fetch = fetchMock;

    const adapter = new RestApiAdapter("https://api.example");
    await adapter.createFlowDefinition("acme", "proj-1", sampleDef);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://api.example/api/v1/acme/proj-1/flows");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual(sampleDef);
  });

  it("updateFlowDefinition PUTs to the flow id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleDef));
    globalThis.fetch = fetchMock;

    const adapter = new RestApiAdapter("https://api.example");
    await adapter.updateFlowDefinition("acme", "proj-1", "flow-1", sampleDef);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://api.example/api/v1/acme/proj-1/flows/flow-1");
    expect((init as RequestInit).method).toBe("PUT");
  });

  it("deleteFlowDefinition DELETEs the flow id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    globalThis.fetch = fetchMock;

    const adapter = new RestApiAdapter("https://api.example");
    await adapter.deleteFlowDefinition("acme", "proj-1", "flow-1");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://api.example/api/v1/acme/proj-1/flows/flow-1");
    expect((init as RequestInit).method).toBe("DELETE");
  });

  it("getFlowDefinition GETs a single flow", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleDef));
    globalThis.fetch = fetchMock;

    const adapter = new RestApiAdapter("https://api.example");
    const def = await adapter.getFlowDefinition("acme", "proj-1", "flow-1");

    expect(def).toEqual(sampleDef);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toBe("https://api.example/api/v1/acme/proj-1/flows/flow-1");
  });
});
