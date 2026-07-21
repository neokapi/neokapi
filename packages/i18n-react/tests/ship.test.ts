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

  it("derives each label from its code as the capitalized endonym", () => {
    // Bare codes (string or label-less LocaleInput) → the language named in its
    // own language, via Intl.DisplayNames, first letter capitalized.
    const model = languagePickerModel({}, ["fr", "de", "ja", { locale: "es" }]);
    const by = Object.fromEntries(model.map((m) => [m.locale, m]));
    expect(by.fr.label).toBe("Français"); // endonym "français", capitalized
    expect(by.de.label).toBe("Deutsch");
    expect(by.ja.label).toBe("日本語"); // no case for CJK — unchanged
    expect(by.es.label).toBe("Español");
  });

  it("an explicit LocaleInput label wins over the derived endonym", () => {
    const model = languagePickerModel({}, [
      { locale: "fr", label: "French" },
      { locale: "de" }, // no label → derived
    ]);
    const by = Object.fromEntries(model.map((m) => [m.locale, m]));
    expect(by.fr.label).toBe("French"); // explicit wins
    expect(by.de.label).toBe("Deutsch"); // derived
  });

  it("an override-map label wins over the derived endonym", () => {
    const model = languagePickerModel({}, ["fr", "de"], {
      labels: { fr: "Le Français" },
    });
    const by = Object.fromEntries(model.map((m) => [m.locale, m]));
    expect(by.fr.label).toBe("Le Français"); // override wins over Intl
    expect(by.de.label).toBe("Deutsch"); // no override → derived
  });

  it("an explicit LocaleInput label still wins over an override-map entry", () => {
    const model = languagePickerModel({}, [{ locale: "fr", label: "Explicit" }], {
      labels: { fr: "Override" },
    });
    expect(model[0].label).toBe("Explicit");
  });

  it("names a pseudo-locale from the override map when Intl cannot", () => {
    // qps has no Intl.DisplayNames name (it echoes the code back).
    const model = languagePickerModel({}, ["qps"], {
      labels: { qps: "Pseudo English" },
    });
    expect(model[0].label).toBe("Pseudo English");
  });

  it("falls back to the raw code for an unknown locale with no override", () => {
    const model = languagePickerModel({}, ["qps"]);
    expect(model[0].label).toBe("qps"); // Intl echoes the code → raw code
  });

  it("falls back gracefully when Intl.DisplayNames throws", () => {
    const RealDisplayNames = Intl.DisplayNames;
    // Simulate a runtime whose Intl.DisplayNames is broken/unavailable.
    (Intl as { DisplayNames: unknown }).DisplayNames = class {
      constructor() {
        throw new Error("Intl.DisplayNames unavailable");
      }
    };
    try {
      const model = languagePickerModel({}, ["fr", "qps"], { labels: { qps: "Pseudo" } });
      const by = Object.fromEntries(model.map((m) => [m.locale, m]));
      expect(by.fr.label).toBe("fr"); // no override, Intl throws → raw code
      expect(by.qps.label).toBe("Pseudo"); // override still applies
    } finally {
      (Intl as { DisplayNames: typeof RealDisplayNames }).DisplayNames = RealDisplayNames;
    }
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
