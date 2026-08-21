import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * Pins the auth contract of the multipart upload paths.
 *
 * The server has required `X-Bowrain-Csrf` on every cookie-authenticated
 * state-changing request since #1239. The three upload methods hand-rolled
 * their headers instead of deriving them from the adapter's `headers()`, so in
 * cookie (BFF) mode they sent none — and every upload from the web app came
 * back 403 "missing CSRF header" from 2026-07-15 onwards (#1587). Nothing in
 * the unit suite noticed, because nothing asserted on the request headers.
 *
 * Content-Type must stay absent: the browser sets it, with the multipart
 * boundary, and forcing it would corrupt the body.
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

function headersOf(fetchMock: ReturnType<typeof mockFetch>): Record<string, string> {
  return (fetchMock.mock.calls[0][1] as RequestInit).headers as Record<string, string>;
}

const file = () => new File(["hello"], "a.md", { type: "text/markdown" });

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("multipart uploads in cookie (BFF) mode", () => {
  const uploads: Array<[string, (a: RestApiAdapter) => Promise<unknown>]> = [
    ["uploadFiles", (a) => a.uploadFiles("acme", "p1", [file()])],
    ["uploadToCollection", (a) => a.uploadToCollection("acme", "p1", "c1", [file()])],
    ["uploadContextScanSources", (a) => a.uploadContextScanSources("acme", [file()])],
  ];

  for (const [name, call] of uploads) {
    it(`${name} sends the CSRF header the server requires`, async () => {
      const fetchMock = mockFetch({ uploads: [] });
      await call(new RestApiAdapter("http://server"));

      const headers = headersOf(fetchMock);
      expect(headers["X-Bowrain-Csrf"]).toBe("1");
      // The browser owns Content-Type for multipart — it carries the boundary.
      expect(headers["Content-Type"]).toBeUndefined();
      expect((fetchMock.mock.calls[0][1] as RequestInit).credentials).toBe("same-origin");
    });

    it(`${name} sends Bearer auth and no CSRF header in token mode`, async () => {
      const fetchMock = mockFetch({ uploads: [] });
      await call(new RestApiAdapter("http://server", "tok-123"));

      const headers = headersOf(fetchMock);
      expect(headers["Authorization"]).toBe("Bearer tok-123");
      // Bearer requests are CSRF-exempt server-side; a custom header would
      // force an unwanted cross-origin preflight.
      expect(headers["X-Bowrain-Csrf"]).toBeUndefined();
      expect(headers["Content-Type"]).toBeUndefined();
    });
  }
});
