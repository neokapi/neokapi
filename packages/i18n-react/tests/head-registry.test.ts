/**
 * The head translatable registry: register / replace / unregister / subscribe.
 * This is the data the review overlay's head panel lists.
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  __resetHeadRegistry,
  headTranslatables,
  registerHeadTranslatable,
  subscribeHeadRegistry,
  unregisterHeadTranslatable,
} from "../src/runtime/head.ts";

afterEach(() => {
  __resetHeadRegistry();
});

describe("head registry", () => {
  it("lists registered head translatables in registration order", () => {
    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    registerHeadTranslatable({ kind: "description", hash: "h2", source: "A page." });

    const list = headTranslatables();
    expect(list).toEqual([
      { kind: "title", hash: "h1", source: "Home" },
      { kind: "description", hash: "h2", source: "A page." },
    ]);
  });

  it("replaces the entry for a kind rather than duplicating it", () => {
    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    registerHeadTranslatable({ kind: "title", hash: "h9", source: "Home 2" });

    const list = headTranslatables();
    expect(list).toHaveLength(1);
    expect(list[0]).toEqual({ kind: "title", hash: "h9", source: "Home 2" });
  });

  it("unregister removes a kind", () => {
    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    registerHeadTranslatable({ kind: "og:title", hash: "h2", source: "Home" });
    unregisterHeadTranslatable("title");

    expect(headTranslatables().map((e) => e.kind)).toEqual(["og:title"]);
  });

  it("notifies subscribers on change, and only on real changes", () => {
    const fn = vi.fn();
    const off = subscribeHeadRegistry(fn);

    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    expect(fn).toHaveBeenCalledTimes(1);

    // Same entry again — no change, no notification.
    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    expect(fn).toHaveBeenCalledTimes(1);

    unregisterHeadTranslatable("title");
    expect(fn).toHaveBeenCalledTimes(2);

    off();
    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    expect(fn).toHaveBeenCalledTimes(2); // unsubscribed
  });

  it("is empty by default so the overlay panel is a no-op", () => {
    expect(headTranslatables()).toEqual([]);
  });
});
