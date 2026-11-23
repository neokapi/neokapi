import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";
import { flowDefinitionFromWire, flowDefinitionsFromWire } from "../api/flowDefinition";
import { ApiError } from "../errors/ApiError";
import { asFlowChanged, FlowChangedError } from "../errors/FlowChangedError";

/**
 * A server older than the one that writes empty lists sends `"edges": null`
 * for every one-step built-in flow, and a stored definition may omit either
 * field. The Flows tab iterated `edges` and failed on every project. These
 * tests feed both shapes through each place a definition enters the client.
 */

// A one-step built-in as an older server lists it.
const nullEdges = {
  id: "qa",
  name: "QA",
  source: "built-in",
  nodes: [{ id: "n1", type: "tool", name: "qa-check", position: { x: 0, y: 0 } }],
  edges: null,
};
// A stored definition written without either list.
const missing = { id: "f1", name: "Mine", source: "project", revision: "r1" };

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    text: async () => JSON.stringify(body),
    headers: { get: () => null },
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("flowDefinitionFromWire", () => {
  it("reads null edges as an empty list and keeps the nodes", () => {
    const def = flowDefinitionFromWire(nullEdges);
    expect(def.edges).toEqual([]);
    expect(def.nodes).toHaveLength(1);
  });

  it("reads missing nodes and edges as empty lists and keeps the other fields", () => {
    const def = flowDefinitionFromWire(missing);
    expect(def).toEqual({ ...missing, nodes: [], edges: [] });
  });

  it("reads a missing list of definitions as empty", () => {
    expect(flowDefinitionsFromWire(null)).toEqual([]);
  });
});

describe("RestApiAdapter flow definitions", () => {
  const api = () => new RestApiAdapter("http://server");

  it("lists definitions with null or missing edges as lists", async () => {
    mockFetch([nullEdges, missing]);
    const flows = await api().listFlowDefinitions("ws", "p1");
    expect(flows.map((f) => [f.nodes.length, f.edges])).toEqual([
      [1, []],
      [0, []],
    ]);
  });

  it("reads one definition with null edges as a list", async () => {
    mockFetch(nullEdges);
    expect((await api().getFlowDefinition("ws", "p1", "qa")).edges).toEqual([]);
  });

  it("reads the saved definition a create or an update returns", async () => {
    mockFetch(missing);
    expect(
      (await api().createFlowDefinition("ws", "p1", flowDefinitionFromWire(missing))).edges,
    ).toEqual([]);
    mockFetch(missing);
    const saved = await api().updateFlowDefinition(
      "ws",
      "p1",
      "f1",
      flowDefinitionFromWire(missing),
    );
    expect(saved.nodes).toEqual([]);
    expect(saved.revision).toBe("r1");
  });
});

describe("FlowChangedError", () => {
  it("carries the current flow with lists, from a 409 body", () => {
    const err = asFlowChanged(
      new ApiError({
        message: "conflict",
        status: 409,
        body: { code: "flow_changed", current: missing },
        bodyText: "",
      }),
    );
    expect(err).toBeInstanceOf(FlowChangedError);
    expect(err?.current.edges).toEqual([]);
    expect(err?.current.nodes).toEqual([]);
  });
});
