// @vitest-environment jsdom
/**
 * The head hooks translate a head string through the runtime dictionary, apply
 * it to the real head element, register it for review, and repaint on a locale
 * switch. Unmount clears the registry so stale slots don't linger.
 */

import { describe, it, expect, beforeEach } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { useTranslatedTitle, useTranslatedDescription } from "../src/head/index.tsx";
import { hashKey } from "../src/runtime/hash.ts";
import { headTranslatables, __resetHeadRegistry } from "../src/runtime/head.ts";
import { setTranslations } from "../src/runtime/index.ts";
import { HEAD_DESCRIPTOR } from "../src/types.ts";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function Head() {
  useTranslatedTitle("Home");
  useTranslatedDescription("A small store.");
  return null;
}

const titleHash = hashKey("Home", HEAD_DESCRIPTOR);
const descHash = hashKey("A small store.", HEAD_DESCRIPTOR);

beforeEach(() => {
  document.head.innerHTML = "";
  document.title = "";
  setTranslations("", {});
  __resetHeadRegistry();
});

describe("head hooks", () => {
  it("sets the title and meta description, and registers them for review", () => {
    const container = document.createElement("div");
    const root = createRoot(container);
    act(() => root.render(<Head />));

    // No dict loaded yet → source text.
    expect(document.title).toBe("Home");
    const meta = document.head.querySelector('meta[name="description"]');
    expect(meta?.getAttribute("content")).toBe("A small store.");

    // Registered with the descriptor-derived hash the review store keys on.
    expect(headTranslatables()).toEqual([
      { kind: "title", hash: titleHash, source: "Home" },
      { kind: "description", hash: descHash, source: "A small store." },
    ]);

    act(() => root.unmount());
    // Unmount clears the registry.
    expect(headTranslatables()).toEqual([]);
  });

  it("repaints the head when the locale switches at runtime", () => {
    const container = document.createElement("div");
    const root = createRoot(container);
    act(() => root.render(<Head />));
    expect(document.title).toBe("Home");

    act(() => {
      setTranslations("de", { [titleHash]: "Startseite", [descHash]: "Ein kleiner Laden." });
    });

    expect(document.title).toBe("Startseite");
    expect(document.head.querySelector('meta[name="description"]')?.getAttribute("content")).toBe(
      "Ein kleiner Laden.",
    );

    act(() => root.unmount());
  });
});
