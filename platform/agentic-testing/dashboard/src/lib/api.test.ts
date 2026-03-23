import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  fetchWorkspaces,
  fetchProjects,
  fetchBlocks,
  fetchAuditLog,
  fetchMembers,
  fetchTranslationProgress,
} from "./api";

function mockFetch(body: unknown, ok = true) {
  return vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok,
      json: () => Promise.resolve(body),
    }),
  );
}

function mockFetchError() {
  return vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network error")));
}

beforeEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// fetchTranslationProgress — the crash site
// ---------------------------------------------------------------------------

describe("fetchTranslationProgress", () => {
  it("handles blocks with missing targets", async () => {
    mockFetch([
      { id: "1", source: "Hello", targets: { "fr-FR": "Bonjour" } },
      { id: "2", source: "World" }, // no targets field
      { id: "3", source: "Bye", targets: undefined },
    ]);

    const progress = await fetchTranslationProgress("ws", "proj", ["fr-FR", "de-DE"]);

    expect(progress).toEqual([
      { locale: "fr-FR", translated: 1, total: 3 },
      { locale: "de-DE", translated: 0, total: 3 },
    ]);
  });

  it("handles empty block array", async () => {
    mockFetch([]);

    const progress = await fetchTranslationProgress("ws", "proj", ["fr-FR"]);

    expect(progress).toEqual([{ locale: "fr-FR", translated: 0, total: 0 }]);
  });

  it("counts partial locale coverage correctly", async () => {
    mockFetch([
      { id: "1", source: "A", targets: { "fr-FR": "A", "de-DE": "A" } },
      { id: "2", source: "B", targets: { "fr-FR": "B" } },
      { id: "3", source: "C", targets: { "de-DE": "C" } },
    ]);

    const progress = await fetchTranslationProgress("ws", "proj", ["fr-FR", "de-DE", "ja-JP"]);

    expect(progress).toEqual([
      { locale: "fr-FR", translated: 2, total: 3 },
      { locale: "de-DE", translated: 2, total: 3 },
      { locale: "ja-JP", translated: 0, total: 3 },
    ]);
  });

  it("returns fallback when API fails", async () => {
    mockFetchError();

    const progress = await fetchTranslationProgress("ws", "proj", ["fr-FR"]);

    expect(progress).toEqual([{ locale: "fr-FR", translated: 0, total: 0 }]);
  });
});

// ---------------------------------------------------------------------------
// Fallback behavior on API errors
// ---------------------------------------------------------------------------

describe("API fallback on errors", () => {
  it("fetchWorkspaces returns [] on network error", async () => {
    mockFetchError();
    expect(await fetchWorkspaces()).toEqual([]);
  });

  it("fetchWorkspaces returns [] on HTTP error", async () => {
    mockFetch(null, false);
    expect(await fetchWorkspaces()).toEqual([]);
  });

  it("fetchProjects returns [] on error", async () => {
    mockFetchError();
    expect(await fetchProjects("ws")).toEqual([]);
  });

  it("fetchBlocks returns [] on error", async () => {
    mockFetchError();
    expect(await fetchBlocks("ws", "proj")).toEqual([]);
  });

  it("fetchAuditLog returns [] on error", async () => {
    mockFetchError();
    expect(await fetchAuditLog("ws")).toEqual([]);
  });

  it("fetchMembers returns [] on error", async () => {
    mockFetchError();
    expect(await fetchMembers("ws")).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Successful API responses
// ---------------------------------------------------------------------------

describe("API successful responses", () => {
  it("fetchWorkspaces returns workspace array", async () => {
    mockFetch([{ id: "1", name: "Test", slug: "test" }]);
    const ws = await fetchWorkspaces();
    expect(ws).toHaveLength(1);
    expect(ws[0].slug).toBe("test");
  });

  it("fetchProjects returns project array", async () => {
    mockFetch([
      {
        id: "p1",
        name: "Excalidraw",
        target_languages: ["fr-FR", "de-DE"],
      },
    ]);
    const projects = await fetchProjects("ws");
    expect(projects[0].target_languages).toEqual(["fr-FR", "de-DE"]);
  });

  it("fetchBlocks returns blocks with targets", async () => {
    mockFetch([{ id: "b1", source: "Hello", targets: { "fr-FR": "Bonjour" } }]);
    const blocks = await fetchBlocks("ws", "proj");
    expect(blocks[0].targets["fr-FR"]).toBe("Bonjour");
  });
});
