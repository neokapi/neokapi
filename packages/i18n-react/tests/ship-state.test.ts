/**
 * The ship state a manifest carries (shippable, withheld or not_gated), and how
 * the picker treats a locale no ship gate matches: offered by default with its
 * state on the model entry, and dropped when the caller offers gated locales
 * only.
 */

import { afterEach, describe, expect, it, vi } from "vitest";

import { languagePickerModel, loadShipStatus, type ShipStatus } from "../src/ship/index.ts";

const MANIFEST: ShipStatus = {
  fr: { shippable: true, state: "established" },
  nb: { shippable: true, state: "not_gated" },
  ja: { shippable: false, state: "withheld" },
};

describe("languagePickerModel and ship states", () => {
  it("offers a not-gated locale by default and says it is not gated", () => {
    const model = languagePickerModel(MANIFEST, ["fr", "nb", "ja"]);
    expect(model.map((m) => m.locale)).toEqual(["fr", "nb"]);

    const by = Object.fromEntries(model.map((m) => [m.locale, m]));
    expect(by.fr.state).toBe("established");
    expect(by.nb.state).toBe("not_gated");
  });

  it("drops a not-gated locale when the caller offers gated locales only", () => {
    const model = languagePickerModel(MANIFEST, ["fr", "nb", "ja"], { includeNotGated: false });
    expect(model.map((m) => m.locale)).toEqual(["fr"]);
  });
});

describe("loadShipStatus and ship states", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps a known state and drops an unknown one", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              nb: { shippable: true, state: "not_gated" },
              fr: { shippable: true, state: "ready" },
            }),
            { status: 200 },
          ),
      ),
    );
    const status = await loadShipStatus();
    expect(status.nb).toEqual({ shippable: true, state: "not_gated" });
    expect(status.fr).toStrictEqual({ shippable: true });
  });
});
