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
});
