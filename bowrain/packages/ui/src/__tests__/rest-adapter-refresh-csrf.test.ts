import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * Pins the auth contract of the session-refresh path.
 *
 * In cookie (BFF) mode the web app holds no refresh token in memory, so the
 * refresh body is empty and the server reads the HttpOnly `bowrain_refresh`
 * cookie instead. That makes the request ambient-credential and
 * state-changing, and the server has required `X-Bowrain-Csrf` on exactly that
 * shape since #1239 — `TestTokenRefresh_CookiePathRequiresCSRF` is the other
 * half of this contract.
 *
 * `tryRefresh` hand-rolled its headers and sent none of it, so every browser
 * refresh came back 403, the adapter reported the session unrecoverable, and
 * the app bounced to the identity provider as soon as the 15-minute access
 * cookie lapsed (#1809). It is the same defect the multipart uploads had in
 * #1587, in the one path whose failure signs the user out.
 *
 * The Bearer case is the inverse and equally load-bearing: a native client
 * authenticates by the token in the body, is CSRF-exempt server-side, and must
 * not send a custom header that would force a cross-origin preflight the
 * server's allowlist rejects.
 */

function mockFetch() {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({}),
    text: async () => "{}",
    headers: { get: () => null },
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function requestOf(fetchMock: ReturnType<typeof mockFetch>): RequestInit {
  return fetchMock.mock.calls[0][1] as RequestInit;
}

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("session refresh", () => {
  it("sends the CSRF header the server requires in cookie (BFF) mode", async () => {
    const fetchMock = mockFetch();
    const adapter = new RestApiAdapter("http://server");

    await expect(adapter.refreshSession()).resolves.toBe(true);

    expect(fetchMock.mock.calls[0][0]).toBe("http://server/api/v1/auth/refresh");
    const init = requestOf(fetchMock);
    const headers = init.headers as Record<string, string>;
    expect(headers["X-Bowrain-Csrf"]).toBe("1");
    expect(headers["Content-Type"]).toBe("application/json");
    expect(headers["Authorization"]).toBeUndefined();
    // The cookie has to travel or the server has nothing to rotate.
    expect(init.credentials).toBe("same-origin");
    // An empty body is what tells the server to read the cookie.
    expect(init.body).toBe("{}");
  });

  it("sends Bearer auth and no CSRF header in token mode", async () => {
    const fetchMock = mockFetch();
    const adapter = new RestApiAdapter("http://server", "tok-123");
    adapter.setRefreshToken("rt-456");

    await expect(adapter.refreshSession()).resolves.toBe(true);

    const headers = requestOf(fetchMock).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok-123");
    expect(headers["X-Bowrain-Csrf"]).toBeUndefined();
    expect(requestOf(fetchMock).body).toBe(JSON.stringify({ refresh_token: "rt-456" }));
  });

  it("refreshes with the CSRF header when a 401 triggers the retry", async () => {
    // The path the user actually hits: a call 401s because the 15-minute access
    // cookie lapsed, the adapter refreshes, and retries. If the refresh 403s,
    // this is where the session is declared dead.
    const responses = [
      {
        ok: false,
        status: 401,
        json: async () => ({}),
        text: async () => "",
        headers: { get: () => null },
      },
      {
        ok: true,
        status: 200,
        json: async () => ({}),
        text: async () => "{}",
        headers: { get: () => null },
      },
      {
        ok: true,
        status: 200,
        json: async () => ({ id: "u1" }),
        text: async () => "{}",
        headers: { get: () => null },
      },
    ];
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(responses.shift()));
    vi.stubGlobal("fetch", fetchMock);

    const adapter = new RestApiAdapter("http://server");
    const onSessionExpired = vi.fn();
    adapter.onSessionExpired = onSessionExpired;

    await adapter.getDigestSettings("acme");

    const refreshCall = fetchMock.mock.calls.find(
      (call: unknown[]) => call[0] === "http://server/api/v1/auth/refresh",
    );
    expect(refreshCall).toBeDefined();
    const headers = (refreshCall![1] as RequestInit).headers as Record<string, string>;
    expect(headers["X-Bowrain-Csrf"]).toBe("1");
    expect(onSessionExpired).not.toHaveBeenCalled();
  });
});
