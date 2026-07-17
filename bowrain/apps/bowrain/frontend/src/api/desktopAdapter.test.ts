import { describe, it, expect, vi, beforeEach } from "vitest";

// Replace the Wails bindings namespace that both the composite (proxy transport
// + onSessionExpired) and the layered WailsApiAdapter import. Only the members
// these tests exercise need to be present. `vi.hoisted` builds the mock before
// the hoisted `vi.mock` factory runs, so the factory can close over it.
const backend = vi.hoisted(() => ({
  ProxyRequest: vi.fn(),
  ProxyMultipart: vi.fn(),
  Logout: vi.fn(),
  GetItemBlocks: vi.fn(),
  GetProject: vi.fn(),
}));
vi.mock("./backend", () => ({ Backend: backend }));

import { createDesktopAdapter } from "./desktopAdapter";

describe("createDesktopAdapter (composite)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("serves a local-first method from its Wails binding, bypassing the proxy", async () => {
    backend.GetItemBlocks.mockResolvedValue([{ id: "b1" }]);
    const api = createDesktopAdapter();

    const blocks = await api.getFileBlocks("acme", "proj-1", "about.json");

    // getFileBlocks is LOCAL_FIRST → WailsApiAdapter → Backend.GetItemBlocks.
    expect(backend.GetItemBlocks).toHaveBeenCalledWith("proj-1", "about.json");
    expect(backend.ProxyRequest).not.toHaveBeenCalled();
    expect(blocks).toEqual([{ id: "b1" }]);
  });

  it("serves a server method through the ProxyRequest transport", async () => {
    backend.ProxyRequest.mockResolvedValue({
      status: 200,
      body: JSON.stringify({ mode: "server", version: "test" }),
    });
    const api = createDesktopAdapter();

    // getConfig is not local-first → RestApiAdapter → the proxy transport, which
    // reaches the real server (server-mode identity, not the "standalone" hardcode).
    const config = await api.getConfig();

    expect(backend.ProxyRequest).toHaveBeenCalledWith("GET", "/api/v1/info", "");
    expect(backend.GetItemBlocks).not.toHaveBeenCalled();
    expect(config).toMatchObject({ mode: "server" });
  });

  it("logs out (returning to ServerConnect) when a 401 cannot be refreshed", async () => {
    // Both the original request and the refresh POST 401 → session expired.
    backend.ProxyRequest.mockResolvedValue({ status: 401, body: "" });
    const api = createDesktopAdapter();

    // fetchJSON blocks forever once onSessionExpired fires, so don't await.
    void api.getConfig();

    await vi.waitFor(() => expect(backend.Logout).toHaveBeenCalled());
  });

  // getProject is REST-first (server view: collections/streams/items) with a
  // fallback to the local working copy only when the server is unreachable.
  describe("getProject", () => {
    it("reads the server project through the proxy, not the local binding", async () => {
      backend.ProxyRequest.mockResolvedValue({
        status: 200,
        body: JSON.stringify({ id: "utulp9hb", name: "Company Website", collections: [{}] }),
      });
      const api = createDesktopAdapter();

      const project = await api.getProject("bowmart", "utulp9hb", "main");

      expect(backend.ProxyRequest).toHaveBeenCalledWith("GET", "/api/v1/bowmart/utulp9hb", "");
      expect(backend.GetProject).not.toHaveBeenCalled();
      expect(project).toMatchObject({ id: "utulp9hb", collections: [{}] });
    });

    it("falls back to the local working copy when the server is unreachable", async () => {
      // A rejected proxy binding = no connection/token or transport failure.
      backend.ProxyRequest.mockRejectedValue(new Error("not connected"));
      backend.GetProject.mockResolvedValue({ id: "utulp9hb", name: "Company Website (offline)" });
      const api = createDesktopAdapter();

      const project = await api.getProject("bowmart", "utulp9hb", "main");

      expect(backend.GetProject).toHaveBeenCalledWith("utulp9hb");
      expect(project).toMatchObject({ name: "Company Website (offline)" });
    });

    it("propagates a server HTTP error (404) without falling back to local", async () => {
      // A reachable server returning 404 is authoritative — the project is gone,
      // not offline, so the stale local copy must not mask it.
      backend.ProxyRequest.mockResolvedValue({ status: 404, body: "not found" });
      const api = createDesktopAdapter();

      await expect(api.getProject("bowmart", "gone", "main")).rejects.toThrow(/404/);
      expect(backend.GetProject).not.toHaveBeenCalled();
    });
  });
});
