import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";
import { FlowChangedError, asFlowChanged } from "../errors/FlowChangedError";
import type { FlowDefinitionInfo } from "../types/api";

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

afterEach(() => {
  vi.unstubAllGlobals();
});

const mine: FlowDefinitionInfo = {
  id: "flow-1",
  name: "Blake's edit",
  source: "project",
  nodes: [],
  edges: [],
  revision: 1,
};

describe("updateFlowDefinition", () => {
  it("sends the revision the edit was based on", async () => {
    const fetchMock = mockFetch({ ...mine, revision: 2 });
    const saved = await new RestApiAdapter("http://server").updateFlowDefinition(
      "acme",
      "p-1",
      "flow-1",
      mine,
    );
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.base_revision).toBe(1);
    expect(saved.revision).toBe(2);
  });

  it("turns the server's refusal of a stale save into FlowChangedError with the current flow", async () => {
    const current = { ...mine, name: "Alex's edit", revision: 2 };
    mockFetch({ code: "flow_changed", error: "changed", current }, 409);
    const err = await new RestApiAdapter("http://server")
      .updateFlowDefinition("acme", "p-1", "flow-1", mine)
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(FlowChangedError);
    expect((err as FlowChangedError).current).toEqual(current);
    expect(asFlowChanged(err)).toBe(err);
  });

  it("leaves any other conflict as it is", async () => {
    mockFetch({ error: "slug_taken", message: "taken" }, 409);
    const err = await new RestApiAdapter("http://server")
      .updateFlowDefinition("acme", "p-1", "flow-1", mine)
      .catch((e: unknown) => e);
    expect(err).not.toBeInstanceOf(FlowChangedError);
    expect(asFlowChanged(err)).toBeNull();
  });
});
