/**
 * Runtime fallback chain: loadTranslations with an array of URLs
 * merges fallback-first (primary wins), tolerates failing fallbacks,
 * and rejects when the primary fails.
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import { __t, loadTranslations, setTranslations } from "../src/runtime/index.ts";

function jsonResponse(body: unknown, ok = true): Response {
  return {
    ok,
    status: ok ? 200 : 404,
    statusText: ok ? "OK" : "Not Found",
    json: async () => body,
  } as Response;
}

afterEach(() => {
  vi.unstubAllGlobals();
  setTranslations("", {});
});

describe("loadTranslations fallback chain", () => {
  it("merges fallback-first so the primary wins", async () => {
    const byUrl: Record<string, Record<string, string>> = {
      "/t/de-AT.json": { a: "AT-a" },
      "/t/de.json": { a: "DE-a", b: "DE-b" },
      "/t/en.json": { a: "EN-a", b: "EN-b", c: "EN-c" },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => jsonResponse(byUrl[url])),
    );

    await loadTranslations("de-AT", ["/t/de-AT.json", "/t/de.json", "/t/en.json"], {
      syncDocumentLocale: false,
    });

    expect(__t("a", "src-a")).toBe("AT-a");
    expect(__t("b", "src-b")).toBe("DE-b");
    expect(__t("c", "src-c")).toBe("EN-c");
    expect(__t("d", "src-d")).toBe("src-d");
  });

  it("tolerates a failing fallback", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        if (url === "/t/de.json") throw new Error("network");
        if (url === "/t/en.json") return jsonResponse(null, false);
        return jsonResponse({ a: "AT-a" });
      }),
    );

    await loadTranslations("de-AT", ["/t/de-AT.json", "/t/de.json", "/t/en.json"], {
      syncDocumentLocale: false,
    });
    expect(__t("a", "src")).toBe("AT-a");
  });

  it("rejects when the primary fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("offline");
      }),
    );
    await expect(
      loadTranslations("de", ["/t/de.json", "/t/en.json"], { syncDocumentLocale: false }),
    ).rejects.toThrow("offline");
  });

  it("single-URL form keeps its original contract", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ x: "X" })),
    );
    await loadTranslations("de", "/t/de.json", { syncDocumentLocale: false });
    expect(__t("x", "src")).toBe("X");
  });
});
