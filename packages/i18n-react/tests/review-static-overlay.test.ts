// @vitest-environment jsdom
/**
 * The hosted overlay on a statically-rendered (inline) page:
 *
 *   - resolves an inline-built element by its content-hash `data-kapi-id`
 *     against the build-emitted review.json (Part 1), and
 *   - (Part 2) with `edit: true`, edits a translation, PUTs it through the
 *     review-store `{endpoint}/{hash}` protocol, and patches the DOM in place —
 *     independent of any app i18n runtime.
 */

import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

import { transform } from "../src/plugin/transform.ts";
import type { ReviewManifest } from "../src/review/manifest.ts";

const tmp: string[] = [];

function translationsDir(dict: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-static-overlay-"));
  tmp.push(dir);
  const tdir = join(dir, "translations");
  mkdirSync(tdir, { recursive: true });
  writeFileSync(join(tdir, "de.json"), JSON.stringify(dict));
  return tdir;
}

const flush = () => new Promise((r) => setTimeout(r, 15));

async function freshInit(opts: Record<string, unknown> = {}): Promise<void> {
  vi.resetModules();
  const mod = await import("../src/review/hosted.ts");
  mod.initKapiReviewHosted(opts);
  await flush();
}

function altClick(el: Element): void {
  el.dispatchEvent(new MouseEvent("click", { bubbles: true, altKey: true }));
}

afterEach(() => {
  for (const d of tmp.splice(0)) rmSync(d, { recursive: true, force: true });
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

describe("hosted overlay on an inline/SSG page", () => {
  it("resolves an inline-built element by content hash via the build-emitted review.json", async () => {
    // Produce the exact static HTML + manifest an inline `review: true`
    // build emits — no runtime, no dev server.
    const hashOf = (await import("../src/plugin/hash.ts")).hashKey;
    const hash = hashOf("Welcome back", "h1");
    const tdir = translationsDir({ [hash]: "Willkommen zurück" });
    const out = transform("<h1>Welcome back</h1>", "src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
      review: true,
    })!;

    document.documentElement.setAttribute("lang", "de");
    document.body.innerHTML = out.code; // the baked, stamped <h1>
    expect(document.querySelector(`[data-kapi-id="${hash}"]`)).toBeTruthy();

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: true, json: async () => out.review }) as unknown as Response),
    );
    Element.prototype.scrollIntoView = vi.fn();

    window.history.replaceState({}, "", `/?kapi-focus=${hash}`);
    await freshInit();

    const panel = document.getElementById("kapi-review-panel");
    expect(panel).toBeTruthy();
    expect(panel!.textContent).toContain("Welcome back"); // source
    expect(panel!.textContent).toContain("Willkommen zurück"); // de target
  });
});

describe("hosted overlay edit (Part 2): DOM patch + PUT round-trip", () => {
  const MANIFEST: ReviewManifest = {
    h123: {
      source: "Welcome back",
      targets: { de: "Willkommen zurück" },
      properties: { file: "src/Page.tsx", line: 1, element: "h1" },
      annotations: [],
    },
    p9: {
      source: "Click {=m0}here{/=m0}",
      targets: { de: "Klicke {=m0}hier{/=m0}" },
      properties: { file: "src/Page.tsx", line: 2, element: "p" },
      annotations: [],
    },
  };

  function stubFetch(): Array<{ url: string; init?: RequestInit }> {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        if (init?.method === "PUT") {
          return { ok: true, status: 200, json: async () => ({}) } as Response;
        }
        return { ok: true, json: async () => MANIFEST } as Response;
      }),
    );
    return calls;
  }

  it("plain-text block: edits, PUTs, and repaints the DOM in place", async () => {
    document.documentElement.setAttribute("lang", "de");
    document.body.innerHTML = `<h1 data-kapi-id="h123" data-kapi-loc="src/Page.tsx:1">Willkommen zurück</h1>`;
    const calls = stubFetch();
    await freshInit({ edit: true, endpoint: "/__kapi/review" });

    altClick(document.querySelector("h1")!);
    await flush();

    const panel = document.getElementById("kapi-review-panel")!;
    const textarea = panel.querySelector("#kapi-review-edit") as HTMLTextAreaElement;
    expect(textarea).toBeTruthy();
    expect(textarea.value).toBe("Willkommen zurück");
    textarea.value = "Willkommen zurück!";

    (panel.querySelector("#kapi-review-save") as HTMLElement).click();
    await flush();

    const put = calls.find((c) => c.init?.method === "PUT");
    expect(put).toBeTruthy();
    expect(put!.url).toBe("/__kapi/review/h123");
    expect(JSON.parse(put!.init!.body as string)).toEqual({
      locale: "de",
      text: "Willkommen zurück!",
    });
    // In-place repaint, no app i18n runtime involved.
    expect(document.querySelector("h1")!.textContent).toBe("Willkommen zurück!");
  });

  it("block with inline structure: saves to the store but does not patch text-only", async () => {
    document.documentElement.setAttribute("lang", "de");
    document.body.innerHTML = `<p data-kapi-id="p9" data-kapi-loc="src/Page.tsx:2">Klicke <a href="/x">hier</a></p>`;
    const calls = stubFetch();
    await freshInit({ edit: true });

    altClick(document.querySelector("p")!);
    await flush();

    const panel = document.getElementById("kapi-review-panel")!;
    (panel.querySelector("#kapi-review-edit") as HTMLTextAreaElement).value = "Tippe hier";
    (panel.querySelector("#kapi-review-save") as HTMLElement).click();
    await flush();

    // Still PUTs the edit to the store…
    expect(calls.find((c) => c.init?.method === "PUT")).toBeTruthy();
    // …but the inline anchor is preserved (no destructive text swap).
    expect(document.querySelector("p a")).toBeTruthy();
    expect(panel.querySelector("#kapi-review-status")!.textContent).toContain("not repainted");
  });

  it("stays read-only without edit:true (no textarea, no save button)", async () => {
    document.documentElement.setAttribute("lang", "de");
    document.body.innerHTML = `<h1 data-kapi-id="h123">Willkommen zurück</h1>`;
    stubFetch();
    await freshInit(); // edit defaults false

    altClick(document.querySelector("h1")!);
    await flush();

    const panel = document.getElementById("kapi-review-panel")!;
    expect(panel.querySelector("#kapi-review-edit")).toBeNull();
    expect(panel.querySelector("#kapi-review-save")).toBeNull();
    expect(panel.textContent).toContain("read-only");
  });
});
