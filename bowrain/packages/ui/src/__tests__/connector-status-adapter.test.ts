import { describe, it, expect, vi, beforeEach, afterEach } from "vite-plus/test";
import { RestApiAdapter } from "../api/rest-adapter";

/**
 * Pins getConnectorStatus and detectInstallationRepo against the payloads the
 * SERVER actually sends. The connector status marshals the Go SyncStatus
 * struct with NO json tags — PascalCase keys — and the GitHub-setup wizard's
 * import tracking reads `errors` off the mapped result. The founder drill
 * exposed that a failed import's recorded error has to survive this exact
 * wire → adapter mapping (previously the endpoint 404ed instead, and the
 * wizard showed "Importing…" forever).
 */

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

function adapter() {
  return new RestApiAdapter("http://server");
}

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("getConnectorStatus", () => {
  it("maps the degraded failed-import payload (production shape)", async () => {
    // What HandleConnectorStatus returns for a never-synced connector whose
    // initial ingest failed and whose live probe re-fails on the same clone.
    mockFetch({
      ConnectorID: "conn-1",
      LastSync: "0001-01-01T00:00:00Z",
      ItemCount: 0,
      FileCount: 0,
      WordCount: 0,
      PendingPull: 0,
      PendingPush: 0,
      Errors: [
        "git clone: fatal: could not read Username for 'https://github.com': terminal prompts disabled\n: exit status 128",
      ],
    });

    const status = await adapter().getConnectorStatus("acme", "conn-1");

    expect(status.lastSync).toBe("0001-01-01T00:00:00Z");
    expect(status.errors).toHaveLength(1);
    expect(status.errors[0]).toContain("exit status 128");
  });

  it("maps a healthy synced payload with null Errors", async () => {
    mockFetch({
      ConnectorID: "conn-1",
      LastSync: "2026-07-18T09:30:00Z",
      ItemCount: 12,
      FileCount: 12,
      WordCount: 340,
      PendingPull: 0,
      PendingPush: 0,
      Errors: null,
    });

    const status = await adapter().getConnectorStatus("acme", "conn-1");

    expect(status.lastSync).toBe("2026-07-18T09:30:00Z");
    expect(status.errors).toEqual([]);
  });
});

describe("detectInstallationRepo", () => {
  it("calls the detect endpoint and passes scope/patterns as query params", async () => {
    const fetchMock = mockFetch({
      monorepo_markers: ["pnpm-workspace.yaml"],
      workspaces: ["apps/web"],
      signals: [
        {
          id: "react-i18next",
          label: "i18next catalogs",
          dir: "apps/web/public/locales",
          files: 3,
        },
      ],
      proposed_patterns: "apps/web/public/locales/**/*.json",
      match_count: 3,
      match_preview: ["apps/web/public/locales/en/common.json"],
      truncated: false,
    });

    const det = await adapter().detectInstallationRepo("acme", "42", "acme/site", {
      scope: "apps/web",
      patterns: "docs/**/*.md",
    });

    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/acme/github/installations/42/repositories/acme/site/detect");
    expect(url).toContain("scope=apps%2Fweb");
    expect(url).toContain("patterns=docs%2F**%2F*.md");
    expect(det.signals[0].id).toBe("react-i18next");
    expect(det.match_count).toBe(3);
  });
});
