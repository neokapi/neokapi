import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  __t,
  loadTranslationChunk,
  loadTranslations,
  setTranslations,
} from "../src/runtime/index.ts";

beforeEach(() => {
  setTranslations("", {});
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("setTranslations({ merge: true })", () => {
  it("OR's entries into the existing dict", () => {
    setTranslations("de", { h1: "Hallo" });
    setTranslations("de", { h2: "Welt" }, { merge: true });
    expect(__t("h1", "Hello")).toBe("Hallo");
    expect(__t("h2", "World")).toBe("Welt");
  });

  it("last write wins on overlap", () => {
    setTranslations("de", { h1: "first" });
    setTranslations("de", { h1: "second" }, { merge: true });
    expect(__t("h1", "x")).toBe("second");
  });

  it("merge into a non-active locale is a no-op", () => {
    setTranslations("de", { h1: "Hallo" });
    setTranslations("fr", { h1: "Bonjour" }, { merge: true });
    expect(__t("h1", "x")).toBe("Hallo"); // untouched
  });

  it("replace clears prior merged entries", () => {
    setTranslations("de", { h1: "a" });
    setTranslations("de", { h2: "b" }, { merge: true });
    setTranslations("de", { h3: "c" });
    expect(__t("h1", "x")).toBe("x"); // gone
    expect(__t("h2", "x")).toBe("x"); // gone
    expect(__t("h3", "x")).toBe("c");
  });
});

describe("loadTranslationChunk", () => {
  it("merges fetched entries into the active dict", async () => {
    setTranslations("de", { h1: "Hallo" });
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ h2: "Welt" })));

    await loadTranslationChunk("de", "/translations/de/Settings.json");
    expect(__t("h1", "x")).toBe("Hallo");
    expect(__t("h2", "x")).toBe("Welt");
  });

  it("dedupes concurrent calls for the same (locale, url)", async () => {
    setTranslations("de", {});
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(
        () =>
          new Promise((resolve) =>
            setTimeout(() => resolve(new Response(JSON.stringify({ h1: "Hallo" }))), 20),
          ),
      );

    const [a, b, c] = await Promise.all([
      loadTranslationChunk("de", "/chunks/a.json"),
      loadTranslationChunk("de", "/chunks/a.json"),
      loadTranslationChunk("de", "/chunks/a.json"),
    ]);
    expect(a).toBeUndefined();
    expect(b).toBeUndefined();
    expect(c).toBeUndefined();
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(__t("h1", "x")).toBe("Hallo");
  });

  it("allows a second fetch after the first settles", async () => {
    setTranslations("de", {});
    // Fresh Response per call — Response bodies are single-use
    // streams and mockResolvedValue would reuse one instance.
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async () => new Response(JSON.stringify({ h1: "a" })));

    await loadTranslationChunk("de", "/chunks/a.json");
    await loadTranslationChunk("de", "/chunks/a.json");
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });

  it("drops in-flight chunks when the locale switches", async () => {
    setTranslations("de", {});
    let resolveFetch!: (r: Response) => void;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const pending = loadTranslationChunk("de", "/chunks/a.json");
    setTranslations("fr", { h1: "Bonjour" });
    resolveFetch(new Response(JSON.stringify({ h2: "should-not-land" })));
    await pending;

    // The chunk's merge targeted "de" but active locale is now "fr" —
    // merge must have been dropped. Only the fr dict should be visible.
    expect(__t("h1", "x")).toBe("Bonjour");
    expect(__t("h2", "fallback")).toBe("fallback");
  });

  it("throws on non-OK HTTP response without polluting the dict", async () => {
    setTranslations("de", { h1: "Hallo" });
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("", { status: 404, statusText: "Not Found" }),
    );
    await expect(loadTranslationChunk("de", "/chunks/missing.json")).rejects.toThrow(/404/);
    expect(__t("h1", "x")).toBe("Hallo"); // unchanged
  });
});

describe("loadTranslations backcompat", () => {
  it("replaces dict by default (prior behaviour)", async () => {
    setTranslations("de", { h1: "Hallo" });
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ h2: "Welt" })));
    await loadTranslations("de", "/translations/de.json");
    expect(__t("h1", "x")).toBe("x"); // gone
    expect(__t("h2", "x")).toBe("Welt");
  });

  it("merges when { merge: true } is passed", async () => {
    setTranslations("de", { h1: "Hallo" });
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ h2: "Welt" })));
    await loadTranslations("de", "/translations/de.json", { merge: true });
    expect(__t("h1", "x")).toBe("Hallo");
    expect(__t("h2", "x")).toBe("Welt");
  });
});
