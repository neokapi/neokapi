// @vitest-environment jsdom
/**
 * A `play` function's interactions have to survive to the rendered story.
 *
 * Storybook runs loaders, then mounts the story, then runs `play`. Fetching the
 * catalog from the decorator instead put it in force after `play` had run, and
 * the re-key that made `__t` read the new dictionary threw the state `play` had
 * built: the click opened a sheet, the remount closed it, and the Interactions
 * panel recorded every step as passed.
 *
 * These tests drive the same three phases by hand, because that ordering is the
 * whole fix.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, createElement, useEffect, useState } from "react";
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
import {
  neokapiDecorator,
  neokapiLoader,
  type NeokapiStorybookOptions,
} from "../src/storybook/index.ts";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const SHEET_TEXT = "Back to checks";

/**
 * A story with the shape the issue was found on: a button that opens a sheet,
 * and a mount counter so a remount is visible rather than inferred.
 */
function makeSheetStory() {
  let mounts = 0;
  function Sheet() {
    const [open, setOpen] = useState(false);
    useEffect(() => {
      mounts += 1;
    }, []);
    return createElement(
      "div",
      null,
      createElement("button", { type: "button", onClick: () => setOpen(true) }, "Open"),
      open ? createElement("span", null, SHEET_TEXT) : null,
    );
  }
  return { Story: () => createElement(Sheet), mounts: () => mounts };
}

describe("a play function's state", () => {
  let container: HTMLElement;
  let root: Root;

  beforeEach(() => {
    setTranslationsMock.mockClear();
    loadTranslationsMock.mockClear();
    // A catalog arrives on a later tick, the way a fetch does.
    loadTranslationsMock.mockImplementation(() => new Promise((resolve) => setTimeout(resolve, 0)));
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
   * Storybook's render phase: the decorator is invoked inside a React render.
   * One `Host` component across renders, so a remount can only come from the
   * decorator's own key and never from React seeing a new component type.
   */
  function mount(
    decorator: ReturnType<typeof neokapiDecorator>,
    Story: () => unknown,
    locale: string,
  ) {
    const Host = ({ loc }: { loc: string }) =>
      decorator(Story as never, { globals: { locale: loc } } as never) as never;
    act(() => {
      root.render(createElement(Host, { loc: locale }));
    });
    return {
      rerender(next: string) {
        act(() => {
          root.render(createElement(Host, { loc: next }));
        });
      },
    };
  }

  /** Storybook's play phase, reduced to the one interaction that matters. */
  function play() {
    const button = container.querySelector("button");
    expect(button, "the story rendered a button to click").not.toBeNull();
    act(() => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
  }

  /** Let every pending catalog fetch and its effects settle. */
  async function settle() {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 10));
    });
  }

  const opts = (): NeokapiStorybookOptions => ({
    locales: [
      { value: "en", title: "English" },
      { value: "qps", title: "Pseudo", url: "/qps.json" },
    ],
  });

  it("survives the catalog landing when a loader put the locale in force", async () => {
    const i18n = opts();
    const loader = neokapiLoader(i18n);
    const decorator = neokapiDecorator(i18n);
    const { Story, mounts } = makeSheetStory();

    await loader({ globals: { locale: "qps" } } as never);
    expect(loadTranslationsMock).toHaveBeenCalledWith("qps", "/qps.json");

    mount(decorator, Story, "qps");
    play();
    expect(container.textContent).toContain(SHEET_TEXT);

    await settle();
    expect(container.textContent).toContain(SHEET_TEXT);
    expect(mounts()).toBe(1);
  });

  it("mounts the story once for a locale that declares no catalog", async () => {
    const i18n = opts();
    const loader = neokapiLoader(i18n);
    const decorator = neokapiDecorator(i18n);
    const { Story, mounts } = makeSheetStory();

    await loader({ globals: { locale: "en" } } as never);
    mount(decorator, Story, "en");
    play();
    await settle();

    expect(container.textContent).toContain(SHEET_TEXT);
    expect(mounts()).toBe(1);
    expect(setTranslationsMock).toHaveBeenCalledWith("en", {});
  });

  it("mounts once when the catalog fetch fails", async () => {
    loadTranslationsMock.mockRejectedValueOnce(new Error("network"));
    const i18n = opts();
    const loader = neokapiLoader(i18n);
    const decorator = neokapiDecorator(i18n);
    const { Story, mounts } = makeSheetStory();

    await loader({ globals: { locale: "qps" } } as never);
    mount(decorator, Story, "qps");
    play();
    await settle();

    expect(container.textContent).toContain(SHEET_TEXT);
    expect(mounts()).toBe(1);
    expect(setTranslationsMock).toHaveBeenCalledWith("qps", {});
  });

  it("does not re-fetch when a story re-renders in the same locale", async () => {
    const i18n = opts();
    const loader = neokapiLoader(i18n);
    const decorator = neokapiDecorator(i18n);
    const { Story, mounts } = makeSheetStory();

    await loader({ globals: { locale: "qps" } } as never);
    mount(decorator, Story, "qps");
    play();
    // Storybook re-runs loaders on every render, including an args change.
    await loader({ globals: { locale: "qps" } } as never);
    await settle();

    expect(loadTranslationsMock).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain(SHEET_TEXT);
    expect(mounts()).toBe(1);
  });

  it("re-keys the story on a genuine locale switch", async () => {
    const i18n = opts();
    const loader = neokapiLoader(i18n);
    const decorator = neokapiDecorator(i18n);
    const { Story, mounts } = makeSheetStory();

    await loader({ globals: { locale: "en" } } as never);
    const story = mount(decorator, Story, "en");
    play();
    expect(container.textContent).toContain(SHEET_TEXT);

    await loader({ globals: { locale: "qps" } } as never);
    story.rerender("qps");
    await settle();

    // A locale switch is a decision someone made, and Storybook re-runs play
    // against the new mount. The sheet closing there is the story starting over.
    expect(mounts()).toBe(2);
    expect(container.textContent).not.toContain(SHEET_TEXT);
  });

  it("remounts, and drops the state, in a preview that registered no loader", async () => {
    const i18n = opts();
    const decorator = neokapiDecorator(i18n);
    const { Story, mounts } = makeSheetStory();

    mount(decorator, Story, "qps");
    play();
    expect(container.textContent).toContain(SHEET_TEXT);

    await settle();
    expect(mounts()).toBe(2);
    expect(container.textContent).not.toContain(SHEET_TEXT);
  });
});
