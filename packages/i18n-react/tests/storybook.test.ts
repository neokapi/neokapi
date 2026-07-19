// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";

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

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

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
  let container: HTMLElement;
  let root: Root;

  beforeEach(() => {
    setTranslationsMock.mockClear();
    loadTranslationsMock.mockClear();
    loadTranslationsMock.mockResolvedValue(undefined);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  /**
   * The decorator renders as a React component (it uses hooks), so
   * tests drive it through a real root. `Host` invokes the decorator
   * the way Storybook does, inside render.
   */
  function mount(
    decorator: ReturnType<typeof neokapiDecorator>,
    Story: () => unknown,
    locale?: string,
  ) {
    const Host = ({ loc }: { loc?: string }) =>
      decorator(Story as never, { globals: { locale: loc } } as never) as never;
    act(() => {
      root.render(createElement(Host, { loc: locale }));
    });
    return {
      rerender(nextLocale?: string) {
        act(() => {
          root.render(createElement(Host, { loc: nextLocale }));
        });
      },
    };
  }

  /** Let the decorator's async apply-effect settle inside act(). */
  async function settle() {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 10));
    });
  }

  it("renders the Story content", () => {
    const Story = vi.fn(() => "story-output");
    const decorator = neokapiDecorator({
      locales: [{ value: "en", title: "English" }],
    });
    mount(decorator, Story, "en");
    expect(Story).toHaveBeenCalled();
    expect(container.textContent).toBe("story-output");
  });

  it("calls setTranslations with empty dict for a locale without a url", async () => {
    const decorator = neokapiDecorator({
      locales: [{ value: "en", title: "English" }],
    });
    mount(decorator, () => null, "en");
    await settle();
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
    mount(decorator, () => null, "qps");
    await settle();
    expect(loadTranslationsMock).toHaveBeenCalledWith("qps", "/translations/qps.json");
  });

  it("falls back to empty dict when loadTranslations rejects", async () => {
    loadTranslationsMock.mockRejectedValueOnce(new Error("network"));
    const decorator = neokapiDecorator({
      locales: [{ value: "qps", title: "Pseudo", url: "/x.json" }],
    });
    mount(decorator, () => null, "qps");
    await settle();
    expect(setTranslationsMock).toHaveBeenCalledWith("qps", {});
  });

  it("does not re-apply translations when the locale is unchanged", async () => {
    const decorator = neokapiDecorator({
      locales: [{ value: "en", title: "English", url: "/en.json" }],
    });
    const handle = mount(decorator, () => null, "en");
    await settle();
    handle.rerender("en");
    handle.rerender("en");
    await settle();
    expect(loadTranslationsMock).toHaveBeenCalledTimes(1);
  });

  it("re-applies translations when the locale changes", async () => {
    const decorator = neokapiDecorator({
      locales: [
        { value: "en", title: "English", url: "/en.json" },
        { value: "qps", title: "Pseudo", url: "/qps.json" },
      ],
    });
    const handle = mount(decorator, () => null, "en");
    await settle();
    handle.rerender("qps");
    await settle();
    expect(loadTranslationsMock).toHaveBeenCalledTimes(2);
    expect(loadTranslationsMock).toHaveBeenNthCalledWith(1, "en", "/en.json");
    expect(loadTranslationsMock).toHaveBeenNthCalledWith(2, "qps", "/qps.json");
  });

  it("remounts the story once the dictionary has landed (stale-locale fix)", async () => {
    let renders = 0;
    const Story = () => {
      renders += 1;
      return "s";
    };
    const decorator = neokapiDecorator({
      locales: [{ value: "qps", title: "Pseudo", url: "/qps.json" }],
    });
    mount(decorator, Story, "qps");
    const before = renders;
    await settle();
    // The key flips from "qps:false" to "qps:true" when the dict is
    // applied — the story must render again against the new dict.
    expect(renders).toBeGreaterThan(before);
  });

  it("uses the first locale when context.globals.locale is undefined", async () => {
    const decorator = neokapiDecorator({
      locales: [
        { value: "en", title: "English" },
        { value: "qps", title: "Pseudo" },
      ],
    });
    mount(decorator, () => null, undefined);
    await settle();
    expect(setTranslationsMock).toHaveBeenCalledWith("en", {});
  });
});
