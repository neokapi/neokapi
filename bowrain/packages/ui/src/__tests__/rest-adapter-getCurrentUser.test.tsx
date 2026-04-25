import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * Regression test for the /claim/{token} hang (#425):
 *
 * Before the fix, getCurrentUser routed through fetchJSON, which on 401
 * fired onSessionExpired and returned a never-resolving promise. ClaimPage's
 * `setCheckingAuth(false)` in the finally then never ran, so the page hung
 * on the loading spinner instead of falling through to the unauthenticated
 * "Sign in to claim" UI. JoinPage had the same shape.
 *
 * After the fix, getCurrentUser does its own fetch, treating 401 (and any
 * other non-OK) as the expected negative answer to "do we have a session?"
 * and returns null. Callers render the unauthenticated UI immediately.
 */
describe("RestApiAdapter.getCurrentUser", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("resolves to null on 401 (does not hang, does not redirect)", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response("", { status: 401 }));

    const adapter = new RestApiAdapter("https://api.example");
    const onSessionExpired = vi.fn();
    adapter.onSessionExpired = onSessionExpired;

    // The mere fact that this awaits is the regression check — pre-fix this
    // returned a never-resolving promise and the test would time out.
    const result = await adapter.getCurrentUser();

    expect(result).toBeNull();
    // getCurrentUser is the unauth-state check — it must NOT trigger
    // the session-expired redirect on its own 401.
    expect(onSessionExpired).not.toHaveBeenCalled();
  });

  it("resolves to null on 4xx other than 401", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response("Not found", { status: 404 }));

    const adapter = new RestApiAdapter("https://api.example");
    expect(await adapter.getCurrentUser()).toBeNull();
  });

  it("resolves to null on network error", async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error("offline"));

    const adapter = new RestApiAdapter("https://api.example");
    expect(await adapter.getCurrentUser()).toBeNull();
  });

  it("returns the user object on 200", async () => {
    const user = { id: "u1", email: "a@b.c", name: "Alice" };
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(user), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const adapter = new RestApiAdapter("https://api.example");
    const got = await adapter.getCurrentUser();
    expect(got).toEqual(user);
  });

  it("includes the bearer token when set", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 401 }));
    globalThis.fetch = fetchMock;

    const adapter = new RestApiAdapter("https://api.example", "tok-xyz");
    await adapter.getCurrentUser();

    const [, init] = fetchMock.mock.calls[0];
    expect((init as RequestInit).headers).toMatchObject({ Authorization: "Bearer tok-xyz" });
  });
});
