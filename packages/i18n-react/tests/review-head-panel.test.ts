// @vitest-environment jsdom
/**
 * The review overlay's head/SEO panel: lists the page's head translatables (from
 * the head registry), and edits save through the SAME review-store PUT path the
 * inline block affordance uses. This is how a reviewer reaches a `<title>` or
 * meta string that has no body DOM text node to ALT+click.
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import { initKapiReview } from "../src/review/index.ts";
import { __t, setTranslations } from "../src/runtime/index.ts";
import { registerHeadTranslatable, __resetHeadRegistry } from "../src/runtime/head.ts";

afterEach(() => {
  document.getElementById("kapi-review-head-panel")?.remove();
  vi.unstubAllGlobals();
  setTranslations("", {});
  __resetHeadRegistry();
});

describe("review overlay — head/SEO panel", () => {
  it("lists head translatables and saves an edit via PUT through the store", async () => {
    // The app's head hooks have registered the page title, and the de locale
    // is active with a translation loaded.
    registerHeadTranslatable({ kind: "title", hash: "h1", source: "Home" });
    setTranslations("de", { h1: "Startseite" });

    const calls: Array<{ url: string; init?: RequestInit }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        return { ok: true, status: 200, json: async () => ({}) } as Response;
      }),
    );

    initKapiReview({ endpoint: "/__kapi/review" });

    // The toolbar surfaces a head/SEO button because a head string exists.
    const button = document.getElementById("kapi-review-head-btn") as HTMLButtonElement;
    expect(button).toBeTruthy();
    expect(button.hidden).toBe(false);
    expect(button.textContent).toContain("(1)");

    button.click();

    const panel = document.getElementById("kapi-review-head-panel")!;
    expect(panel).toBeTruthy();
    expect(panel.textContent).toContain("Home"); // source
    expect(panel.textContent).toContain("title"); // kind label

    const textarea = panel.querySelector(".kapi-head-target") as HTMLTextAreaElement;
    // Target column shows the active-locale value the head hook applied.
    expect(textarea.value).toBe("Startseite");
    textarea.value = "Startseite!";

    (panel.querySelector(".kapi-head-save") as HTMLElement).click();
    await new Promise((r) => setTimeout(r, 10));

    const put = calls.find((c) => c.init?.method === "PUT");
    expect(put).toBeTruthy();
    expect(put!.url).toBe("/__kapi/review/h1");
    expect(JSON.parse(put!.init!.body as string)).toEqual({ locale: "de", text: "Startseite!" });

    // Optimistic in-place repaint through the runtime store.
    expect(__t("h1", "Home")).toBe("Startseite!");
  });

  it("keeps the head button hidden when there are no head translatables", () => {
    vi.stubGlobal("fetch", vi.fn());
    initKapiReview({ endpoint: "/__kapi/review" });
    const button = document.getElementById("kapi-review-head-btn") as HTMLButtonElement;
    expect(button.hidden).toBe(true);
    // No panel until a head string registers.
    button.click();
    expect(document.getElementById("kapi-review-head-panel")).toBeNull();
  });
});
