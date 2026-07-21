/**
 * Render-mode-independent review (Part 1): an inline/SSG build with
 * `review: true` stamps `data-kapi-id` AND emits `translations/review.json`,
 * exactly like a runtime build — the review ID is a content hash (AD-035), so
 * the hosted overlay works on statically-rendered pages with no app runtime.
 */

import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { transform } from "../src/plugin/transform.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { unpluginFactory } from "../src/plugin/index.ts";
import type { ReviewManifest } from "../src/review/manifest.ts";

const tmp: string[] = [];
afterEach(() => {
  for (const d of tmp.splice(0)) rmSync(d, { recursive: true, force: true });
});

/** A translations dir seeded with a `de.json` dict. */
function translationsDir(dict: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-static-review-"));
  tmp.push(dir);
  const tdir = join(dir, "translations");
  mkdirSync(tdir, { recursive: true });
  writeFileSync(join(tdir, "de.json"), JSON.stringify(dict));
  return tdir;
}

const H1 = hashKey("Welcome back", "h1");
const PH = hashKey("Search...", "input[placeholder]");

describe("inline build: stamps + review manifest", () => {
  it("stamps data-kapi-id and records source/target/properties in inline mode", () => {
    const tdir = translationsDir({ [H1]: "Willkommen zurück" });
    const out = transform("<h1>Welcome back</h1>", "src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
      review: true,
    })!;

    // Baked target + a content-hash stamp on the static HTML.
    expect(out.code).toContain("Willkommen zurück");
    expect(out.code).toContain(`data-kapi-id="${H1}"`);

    // The review manifest entry the plugin will emit to review.json.
    expect(out.review[H1]).toMatchObject({
      source: "Welcome back",
      targets: { de: "Willkommen zurück" },
      properties: { file: "src/Page.tsx", line: 1, element: "h1" },
    });
  });

  it("records attribute blocks too (placeholder/aria strings)", () => {
    const tdir = translationsDir({ [PH]: "Suchen..." });
    const out = transform('<input placeholder="Search..." />', "src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
      review: true,
    })!;
    expect(out.code).toContain(`data-kapi-attr="placeholder:${PH}"`);
    expect(out.review[PH]).toMatchObject({
      source: "Search...",
      targets: { de: "Suchen..." },
      properties: { element: "input" },
    });
  });

  it("emits nothing extra when review is off (no bundle-size regression)", () => {
    const tdir = translationsDir({ [H1]: "Willkommen zurück" });
    const out = transform("<h1>Welcome back</h1>", "src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
    })!;
    expect(out.code).not.toContain("data-kapi-");
    expect(Object.keys(out.review)).toHaveLength(0);
  });
});

describe("hash parity: inline === runtime", () => {
  it("the same string hashes identically across render modes", () => {
    const tdir = translationsDir({ [H1]: "Willkommen zurück" });
    const inline = transform("<h1>Welcome back</h1>", "src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
      review: true,
    })!;
    const runtime = transform("<h1>Welcome back</h1>", "src/Page.tsx", {
      mode: "runtime",
      review: true,
    })!;
    const inlineHash = Object.keys(inline.review)[0];
    const runtimeHash = Object.keys(runtime.review)[0];
    expect(inlineHash).toBe(H1);
    expect(runtimeHash).toBe(H1);
    // Both stamp the same id, so one manifest interoperates across builds.
    expect(inline.code).toContain(`data-kapi-id="${H1}"`);
    expect(runtime.code).toContain(`data-kapi-id="${H1}"`);
  });
});

// ─── Plugin asset emission ───────────────────────────────────

interface EmittedAsset {
  type: "asset";
  fileName: string;
  source: string;
}

/** Drive the unplugin factory's transform + vite generateBundle hooks. */
function runInlinePlugin(
  code: string,
  id: string,
  opts: { locale?: string; translationsDir?: string; review?: boolean; mode?: "runtime" },
): EmittedAsset[] {
  const plugin = unpluginFactory(opts);
  plugin.buildStart();
  plugin.transform(code, id);
  const emitted: EmittedAsset[] = [];
  const ctx = { emitFile: (a: EmittedAsset) => emitted.push(a) };
  plugin.vite.generateBundle.call(ctx, {}, {});
  return emitted;
}

describe("plugin emits translations/review.json — gated on review, not mode", () => {
  it("an inline build with review:true emits review.json at the hosted-overlay default path", () => {
    const tdir = translationsDir({ [H1]: "Willkommen zurück" });
    const emitted = runInlinePlugin("<h1>Welcome back</h1>", "/proj/src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
      review: true,
    });
    const review = emitted.find((a) => a.fileName === "translations/review.json");
    expect(review).toBeTruthy();
    const manifest = JSON.parse(review!.source) as ReviewManifest;
    expect(manifest[H1]).toMatchObject({
      source: "Welcome back",
      targets: { de: "Willkommen zurück" },
    });
    // Inline mode never emits the OTA chunk manifest.
    expect(emitted.find((a) => a.fileName === "translations-manifest.json")).toBeUndefined();
  });

  it("a non-review inline build emits no review artifacts", () => {
    const tdir = translationsDir({ [H1]: "Willkommen zurück" });
    const emitted = runInlinePlugin("<h1>Welcome back</h1>", "/proj/src/Page.tsx", {
      locale: "de",
      translationsDir: tdir,
    });
    expect(emitted).toHaveLength(0);
  });

  it("a runtime review build also emits review.json (both modes)", () => {
    const emitted = runInlinePlugin("<h1>Welcome back</h1>", "/proj/src/Page.tsx", {
      mode: "runtime",
      review: true,
    });
    expect(emitted.find((a) => a.fileName === "translations/review.json")).toBeTruthy();
    // Runtime keeps emitting the OTA chunk manifest unchanged.
    expect(emitted.find((a) => a.fileName === "translations-manifest.json")).toBeTruthy();
  });
});
