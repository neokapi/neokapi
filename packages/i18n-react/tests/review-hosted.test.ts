// @vitest-environment jsdom
/**
 * Hosted (read-only) review overlay: whole-page review mode lists every stamped
 * string on the page grouped by source file, and each row opens its block in
 * context. Also covers the toolbar toggle and untranslated highlighting.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ReviewManifest } from "../src/review/manifest.ts";

const MANIFEST: ReviewManifest = {
  h1: {
    source: "Welcome back to KapiMart",
    targets: { ja: "KapiMart へようこそ" },
    properties: { file: "App.tsx", line: 3, element: "h1", component: "App" },
    annotations: [],
  },
  h2: {
    source: "KapiMart",
    targets: { ja: "カピマート" },
    properties: { file: "App.tsx", line: 8, element: "a", component: "Nav" },
    annotations: [],
  },
  h3: {
    source: "Checkout",
    targets: {}, // untranslated for the active locale
    properties: { file: "Cart.tsx", line: 2, element: "button", component: "Cart" },
    annotations: [],
  },
};

async function freshInit(url: string, opts = {}): Promise<void> {
  window.history.replaceState({}, "", url);
  vi.resetModules();
  const mod = await import("../src/review/hosted.ts");
  mod.initKapiReviewHosted(opts);
  // Flush the manifest fetch chain (fetch → json → enterReviewMode).
  await new Promise((r) => setTimeout(r, 10));
}

beforeEach(() => {
  document.documentElement.setAttribute("lang", "ja");
  document.body.innerHTML = `
    <h1 data-kapi-id="h1">KapiMart へようこそ</h1>
    <a data-kapi-id="h2">カピマート</a>
    <button data-kapi-id="h3">Checkout</button>
  `;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({ ok: true, json: async () => MANIFEST }) as unknown as Response),
  );
  Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  for (const id of [
    "kapi-review-index",
    "kapi-review-panel",
    "kapi-review-toolbar",
    "kapi-review-hosted-styles",
    "kapi-review-toast",
  ]) {
    document.getElementById(id)?.remove();
  }
  document.documentElement.removeAttribute("data-kapi-review-mode");
  document.documentElement.removeAttribute("lang");
  document.body.innerHTML = "";
  window.history.replaceState({}, "", "/");
  vi.unstubAllGlobals();
});

describe("hosted review — whole-page mode", () => {
  it("?kapi-review opens the index, grouped by file with translated stats", async () => {
    await freshInit("/?kapi-review");

    const index = document.getElementById("kapi-review-index");
    expect(index).toBeTruthy();

    const files = [...index!.querySelectorAll(".kapi-idx-file")].map((f) => f.textContent);
    expect(files).toEqual(["App.tsx2", "Cart.tsx1"]); // name + per-file count

    expect(index!.querySelector(".kapi-idx-sub")!.textContent).toBe("3 strings · 2 translated");

    // One row per distinct stamped block, in document order.
    const rows = index!.querySelectorAll(".kapi-idx-row");
    expect(rows).toHaveLength(3);
    expect(rows[0].textContent).toContain("Welcome back to KapiMart");

    // Status dots: h2 translated (ok), h3 untranslated (untr).
    expect(rows[1].querySelector(".kapi-idx-dot")!.classList.contains("kapi-ok")).toBe(true);
    expect(rows[2].querySelector(".kapi-idx-dot")!.classList.contains("kapi-untr")).toBe(true);

    // The page is put in review mode and the untranslated element is flagged.
    expect(document.documentElement.hasAttribute("data-kapi-review-mode")).toBe(true);
    expect(
      document
        .querySelector("button[data-kapi-id='h3']")!
        .hasAttribute("data-kapi-review-untranslated"),
    ).toBe(true);
    expect(
      document.querySelector("a[data-kapi-id='h2']")!.hasAttribute("data-kapi-review-untranslated"),
    ).toBe(false);
  });

  it("clicking an index row opens that block in context", async () => {
    await freshInit("/?kapi-review");

    const row = [...document.querySelectorAll<HTMLElement>(".kapi-idx-row")].find(
      (r) => r.textContent?.includes("KapiMart") && !r.textContent?.includes("Welcome"),
    )!;
    row.click();
    await new Promise((r) => setTimeout(r, 10));

    const el = document.querySelector("a[data-kapi-id='h2']")!;
    expect((el as HTMLElement & { scrollIntoView: () => void }).scrollIntoView).toHaveBeenCalled;
    const panel = document.getElementById("kapi-review-panel");
    expect(panel).toBeTruthy();
    expect(panel!.textContent).toContain("KapiMart"); // source shown
    expect(panel!.textContent).toContain("カピマート"); // ja target shown
    expect(row.hasAttribute("data-active")).toBe(true);
  });

  it("the toolbar pill toggles review mode on and off", async () => {
    await freshInit("/"); // no ?kapi-review → index closed initially
    expect(document.getElementById("kapi-review-index")).toBeNull();

    const toolbar = document.getElementById("kapi-review-toolbar")!;
    toolbar.click();
    expect(document.getElementById("kapi-review-index")).toBeTruthy();
    expect(toolbar.hasAttribute("data-active")).toBe(true);

    toolbar.click();
    expect(document.getElementById("kapi-review-index")).toBeNull();
    expect(toolbar.hasAttribute("data-active")).toBe(false);
    expect(document.documentElement.hasAttribute("data-kapi-review-mode")).toBe(false);
  });

  it("the search box filters rows and empty file groups", async () => {
    await freshInit("/?kapi-review");
    const index = document.getElementById("kapi-review-index")!;
    const search = index.querySelector<HTMLInputElement>(".kapi-idx-search")!;

    search.value = "checkout";
    search.dispatchEvent(new Event("input"));

    const visibleRows = [...index.querySelectorAll<HTMLElement>(".kapi-idx-row")].filter(
      (r) => r.style.display !== "none",
    );
    expect(visibleRows).toHaveLength(1);
    expect(visibleRows[0].textContent).toContain("Checkout");

    // App.tsx group (no match) is hidden; Cart.tsx (has the match) stays.
    const visibleFiles = [...index.querySelectorAll<HTMLElement>(".kapi-idx-file")].filter(
      (f) => f.style.display !== "none",
    );
    expect(visibleFiles.map((f) => f.textContent)).toEqual(["Cart.tsx1"]);
  });
});
