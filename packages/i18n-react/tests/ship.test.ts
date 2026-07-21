/**
 * Ship-aware language picker helpers: the pure render-model transform (shippable
 * filter + AI-only badge rule) and the manifest loader (missing/malformed
 * tolerance).
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import { languagePickerModel, loadShipStatus, type ShipStatus } from "../src/ship/index.ts";

const MANIFEST: ShipStatus = {
  fr: { shippable: true, verified: true }, // ships + verified → no badge
  de: { shippable: true, verified: false }, // ships, unverified → AI badge
  ja: { shippable: false, verified: false }, // not shippable → dropped
};

describe("languagePickerModel", () => {
  it("drops non-shippable locales and badges the shippable-but-unverified ones AI", () => {
    const model = languagePickerModel(MANIFEST, ["en", "fr", "de", "ja"]);
    // ja is filtered out; en has no entry (source language) so it passes through.
    expect(model.map((m) => m.locale)).toEqual(["en", "fr", "de"]);

    const by = Object.fromEntries(model.map((m) => [m.locale, m]));
    expect(by.fr.badge).toBeNull(); // verified → no badge
    expect(by.de.badge).toBe("ai"); // shippable but unverified → the only badge
    expect(by.en.badge).toBeNull(); // no entry → unbadged passthrough
    // every entry in the model is shippable
    expect(model.every((m) => m.shippable)).toBe(true);
  });

  it("'ai' is the only badge — a verified locale never gets one", () => {
    const model = languagePickerModel(MANIFEST, ["fr", "de"]);
    const badges = model.map((m) => m.badge);
    expect(badges).toContain("ai");
    expect(badges).not.toContain("verified");
    expect(badges.filter((b) => b !== null)).toEqual(["ai"]);
  });

  it("falls back to showing all locales unbadged when the manifest is empty", () => {
    const model = languagePickerModel({}, ["fr", "de", "ja"]);
    expect(model.map((m) => m.locale)).toEqual(["fr", "de", "ja"]);
    expect(model.every((m) => m.badge === null)).toBe(true);
    expect(model.every((m) => m.shippable)).toBe(true);
  });

  it("uses caller labels, falling back to the locale code", () => {
    const model = languagePickerModel(MANIFEST, [
      { locale: "fr", label: "Français" },
      { locale: "de" },
    ]);
    const by = Object.fromEntries(model.map((m) => [m.locale, m]));
    expect(by.fr.label).toBe("Français");
    expect(by.de.label).toBe("de"); // no label → code
  });
});

describe("loadShipStatus", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("parses a served manifest", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify(MANIFEST), { status: 200 })),
    );
    const status = await loadShipStatus("/ship.json");
    expect(status).toEqual(MANIFEST);
  });

  it("returns {} on a missing manifest (non-2xx)", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("not found", { status: 404 })),
    );
    expect(await loadShipStatus()).toEqual({});
  });

  it("returns {} when fetch throws", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("network down");
      }),
    );
    expect(await loadShipStatus()).toEqual({});
  });

  it("coerces malformed entries, keeping only booleans", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ fr: { shippable: true, verified: "yes" }, bad: 3, ja: null }),
            { status: 200 },
          ),
      ),
    );
    const status = await loadShipStatus();
    expect(status).toEqual({ fr: { shippable: true, verified: false } });
  });
});
