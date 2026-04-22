import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const setTranslationsMock = vi.fn();
const loadTranslationsMock = vi.fn<(locale: string, url: string) => Promise<void>>();

vi.mock("../src/runtime/index.ts", () => ({
  setTranslations: setTranslationsMock,
  loadTranslations: loadTranslationsMock,
  t: () => "",
  tx: () => null,
  useNeokapi: () => 0,
}));

// Imports MUST come after vi.mock so the mock is hoisted into place.
import { neokapiDecorator, neokapiGlobalType } from "../src/storybook/index.ts";

describe("neokapiGlobalType", () => {
  it("produces a Storybook toolbar config from locales", () => {
    const result = neokapiGlobalType({
      locales: [
        { value: "en", title: "English" },
        { value: "qps", title: "Pseudo" },
      ],
    });
    expect(result).toEqual({
      name: "Language",
      description: "UI language",
      defaultValue: "en",
      toolbar: {
        icon: "globe",
        items: [
          { value: "en", title: "English" },
          { value: "qps", title: "Pseudo" },
        ],
        dynamicTitle: true,
      },
    });
  });

  it('falls back to "en" when locales is empty', () => {
    const result = neokapiGlobalType({ locales: [] });
    expect(result.defaultValue).toBe("en");
    expect(result.toolbar.items).toEqual([]);
  });
});

describe("neokapiDecorator", () => {
  beforeEach(() => {
    setTranslationsMock.mockClear();
    loadTranslationsMock.mockClear();
    loadTranslationsMock.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  function makeContext(locale?: string) {
    return { globals: { locale } } as any;
  }

  it("returns the Story result synchronously", () => {
    const Story = vi.fn(() => "story-output");
    const decorator = neokapiDecorator({
      locales: [{ value: "en", title: "English" }],
    });
    const result = decorator(Story as any, makeContext("en"));
    expect(Story).toHaveBeenCalled();
    expect(result).toBe("story-output");
  });

  it("calls setTranslations with empty dict for a locale without a url", async () => {
    const decorator = neokapiDecorator({
      locales: [{ value: "en", title: "English" }],
    });
    const Story = vi.fn(() => null);
    decorator(Story as any, makeContext("en"));
    await new Promise((r) => setTimeout(r, 10));
    expect(setTranslationsMock).toHaveBeenCalledWith("en", {});
    expect(loadTranslationsMock).not.toHaveBeenCalled();
  });

  it("calls loadTranslations with the locale url when provided", async () => {
    const decorator = neokapiDecorator({
      locales: [
        { value: "en", title: "English" },
        { value: "qps", title: "Pseudo", url: "/translations/qps.json" },
      ],
    });
    const Story = vi.fn(() => null);
    decorator(Story as any, makeContext("qps"));
    await new Promise((r) => setTimeout(r, 10));
    expect(loadTranslationsMock).toHaveBeenCalledWith("qps", "/translations/qps.json");
  });

  it("falls back to empty dict when loadTranslations rejects", async () => {
    loadTranslationsMock.mockRejectedValueOnce(new Error("network"));
    const decorator = neokapiDecorator({
      locales: [{ value: "qps", title: "Pseudo", url: "/x.json" }],
    });
    const Story = vi.fn(() => null);
    decorator(Story as any, makeContext("qps"));
    await new Promise((r) => setTimeout(r, 10));
    expect(setTranslationsMock).toHaveBeenCalledWith("qps", {});
  });

  it("does not re-apply translations when the locale is unchanged", async () => {
    const decorator = neokapiDecorator({
      locales: [{ value: "en", title: "English", url: "/en.json" }],
    });
    const Story = vi.fn(() => null);
    decorator(Story as any, makeContext("en"));
    decorator(Story as any, makeContext("en"));
    decorator(Story as any, makeContext("en"));
    await new Promise((r) => setTimeout(r, 10));
    expect(loadTranslationsMock).toHaveBeenCalledTimes(1);
  });

  it("re-applies translations when the locale changes", async () => {
    const decorator = neokapiDecorator({
      locales: [
        { value: "en", title: "English", url: "/en.json" },
        { value: "qps", title: "Pseudo", url: "/qps.json" },
      ],
    });
    const Story = vi.fn(() => null);
    decorator(Story as any, makeContext("en"));
    await new Promise((r) => setTimeout(r, 10));
    decorator(Story as any, makeContext("qps"));
    await new Promise((r) => setTimeout(r, 10));
    expect(loadTranslationsMock).toHaveBeenCalledTimes(2);
    expect(loadTranslationsMock).toHaveBeenNthCalledWith(1, "en", "/en.json");
    expect(loadTranslationsMock).toHaveBeenNthCalledWith(2, "qps", "/qps.json");
  });

  it("uses the first locale when context.globals.locale is undefined", async () => {
    const decorator = neokapiDecorator({
      locales: [
        { value: "en", title: "English" },
        { value: "qps", title: "Pseudo" },
      ],
    });
    const Story = vi.fn(() => null);
    decorator(Story as any, makeContext(undefined));
    await new Promise((r) => setTimeout(r, 10));
    expect(setTranslationsMock).toHaveBeenCalledWith("en", {});
  });

  it("skips fetch when running without a fetch global (SSR-safe)", async () => {
    const origFetch = (globalThis as any).fetch;
    delete (globalThis as any).fetch;
    try {
      const decorator = neokapiDecorator({
        locales: [{ value: "qps", title: "Pseudo", url: "/qps.json" }],
      });
      const Story = vi.fn(() => null);
      decorator(Story as any, makeContext("qps"));
      await new Promise((r) => setTimeout(r, 10));
      expect(loadTranslationsMock).not.toHaveBeenCalled();
      expect(setTranslationsMock).toHaveBeenCalledWith("qps", {});
    } finally {
      (globalThis as any).fetch = origFetch;
    }
  });
});
