import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * Pins the tasks endpoints against the routes the server actually registers.
 * The dedicated /my/tasks route was retired for /tasks?assignee_id=me
 * (Bowrain AD-011); the adapter kept calling the old path, which fell through
 * to the catch-all and 404ed on fresh workspaces (production drill).
 */

function mockFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
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

describe("listMyTasks", () => {
  it("queries /tasks with assignee_id=me (no retired /my/tasks path)", async () => {
    const fetchMock = mockFetch({ tasks: [], next_cursor: "" });
    const adapter = new RestApiAdapter("http://server");

    await adapter.listMyTasks("acme", { status: "open" });

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).not.toContain("/my/tasks");
    expect(url).toContain("/api/v1/acme/tasks?");
    expect(url).toContain("assignee_id=me");
    expect(url).toContain("status=open");
  });

  it("forwards cursor and limit", async () => {
    const fetchMock = mockFetch({ tasks: [], next_cursor: "" });
    const adapter = new RestApiAdapter("http://server");

    await adapter.listMyTasks("acme", { cursor: "c1", limit: 10 });

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("cursor=c1");
    expect(url).toContain("limit=10");
    expect(url).toContain("assignee_id=me");
  });
});

describe("refreshSession", () => {
  it("posts to the auth refresh endpoint and reports the outcome", async () => {
    const fetchMock = mockFetch({});
    const adapter = new RestApiAdapter("http://server");

    await expect(adapter.refreshSession()).resolves.toBe(true);
    expect(fetchMock.mock.calls[0][0]).toBe("http://server/api/v1/auth/refresh");
  });

  it("deduplicates concurrent refresh attempts", async () => {
    const fetchMock = mockFetch({});
    const adapter = new RestApiAdapter("http://server");

    const [a, b] = await Promise.all([adapter.refreshSession(), adapter.refreshSession()]);
    expect(a).toBe(true);
    expect(b).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reports false when the session cannot be refreshed", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({}),
      text: async () => "",
      headers: { get: () => null },
    });
    vi.stubGlobal("fetch", fetchMock);
    const adapter = new RestApiAdapter("http://server");

    await expect(adapter.refreshSession()).resolves.toBe(false);
  });
});
