// @vitest-environment jsdom
/**
 * Review overlay client: ALT+click opens the panel from the marked
 * element, saving PUTs to the middleware and repaints in place.
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import { initKapiReview } from "../src/review/index.ts";
import { __t, setTranslations } from "../src/runtime/index.ts";

const PAYLOAD = {
  hash: "h123",
  sourceText: "Welcome back",
  targets: { de: { text: "Willkommen zurück" } },
  properties: { file: "src/Page.tsx", line: 4, component: "Page", element: "h1" },
  placeholders: [],
  annotations: [{ id: "a1", annotationType: "@neokapi/term-detector", data: { term: "Willkommen" } }],
};

function altClick(el: Element): void {
  el.dispatchEvent(new MouseEvent("click", { bubbles: true, altKey: true }));
}

afterEach(() => {
  document.getElementById("kapi-review-panel")?.remove();
  vi.unstubAllGlobals();
  setTranslations("", {});
});

describe("review overlay", () => {
  it("opens the panel on ALT+click and saves edits via PUT", async () => {
    document.body.innerHTML = `<h1 data-kapi-id="h123" data-kapi-loc="src/Page.tsx:4">Willkommen zurück</h1>`;
    // The app has activated the de locale (sets <html lang> too) —
    // the overlay's optimistic merge only applies to the active locale.
    setTranslations("de", { h123: "Willkommen zurück" });

    const calls: Array<{ url: string; init?: RequestInit }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        return { ok: true, status: 200, json: async () => PAYLOAD } as Response;
      }),
    );
    // jsdom has no EventSource — the client must tolerate that.
    initKapiReview({ endpoint: "/__kapi/review" });

    altClick(document.querySelector("h1")!);
    await new Promise((r) => setTimeout(r, 10));

    const panel = document.getElementById("kapi-review-panel")!;
    expect(panel).toBeTruthy();
    expect(panel.textContent).toContain("Welcome back");
    expect(panel.textContent).toContain("src/Page.tsx:4");
    expect(panel.textContent).toContain("term-detector");

    const textarea = panel.querySelector("#kapi-review-target") as HTMLTextAreaElement;
    expect(textarea.value).toBe("Willkommen zurück");
    textarea.value = "Willkommen zurück!";

    (panel.querySelector("#kapi-review-save") as HTMLElement).click();
    await new Promise((r) => setTimeout(r, 10));

    const put = calls.find((c) => c.init?.method === "PUT");
    expect(put).toBeTruthy();
    expect(put!.url).toBe("/__kapi/review/h123");
    expect(JSON.parse(put!.init!.body as string)).toEqual({
      locale: "de",
      text: "Willkommen zurück!",
    });
    // Optimistic in-place repaint through the runtime store.
    expect(__t("h123", "src")).toBe("Willkommen zurück!");
  });

  it("does nothing on plain clicks", async () => {
    document.body.innerHTML = `<h1 data-kapi-id="h999">Hi</h1>`;
    vi.stubGlobal("fetch", vi.fn());
    initKapiReview();
    document.querySelector("h1")!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await new Promise((r) => setTimeout(r, 5));
    expect(document.getElementById("kapi-review-panel")).toBeNull();
  });
});
