import { describe, expect, it } from "vitest";

import {
  AnnotationExt,
  Ext,
  ExtI18nReact,
  isAnnotationPath,
  isKbfPath,
  isOverlaySetPath,
  OverlaySetExt,
  ReadableExts,
  trimKbfExt,
} from "../src/index.ts";

// All three suffixes are *compound* and all three end in a JSON-family
// extension, so `path.extname()` cannot tell them apart and a naive
// `endsWith(".json")` would conflate a bundle with an overlay set. The
// predicates must partition the space: every path matches at most one.

const cases: Array<{ path: string; kbf: boolean; annotation: boolean; overlaySet: boolean }> = [
  { path: "i18n/src/App.kbf.json", kbf: true, annotation: false, overlaySet: false },
  // What @neokapi/i18n-react 1.2.3 wrote. Still a bundle, and still only one.
  { path: "i18n/src/App.klf", kbf: true, annotation: false, overlaySet: false },
  { path: "i18n/src/App.overlays.json", kbf: false, annotation: false, overlaySet: true },
  { path: "i18n/src/App.overlays.jsonl", kbf: false, annotation: true, overlaySet: false },
  // A plain JSON document (e.g. a compiled runtime dictionary) is none of them.
  { path: "public/translations/fr.json", kbf: false, annotation: false, overlaySet: false },
  // Bare legacy/near-miss suffixes must not match either.
  { path: "i18n/src/App.kbf", kbf: false, annotation: false, overlaySet: false },
  { path: "i18n/src/App.jsonl", kbf: false, annotation: false, overlaySet: false },
  // An overlay set *for* a bundle: overlay-set wins, it is not a bundle.
  { path: "i18n/src/App.kbf.overlays.json", kbf: false, annotation: false, overlaySet: true },
];

describe("path predicates partition the compound suffixes", () => {
  for (const { path, kbf, annotation, overlaySet } of cases) {
    it(`classifies ${path}`, () => {
      const actual = [isKbfPath(path), isAnnotationPath(path), isOverlaySetPath(path)];
      expect(actual).toEqual([kbf, annotation, overlaySet]);
      // Mutual exclusion: no path may satisfy two predicates at once.
      expect(actual.filter(Boolean).length).toBeLessThanOrEqual(1);
    });
  }

  it("matches case-insensitively", () => {
    expect(isKbfPath("I18N/SRC/APP.KBF.JSON")).toBe(true);
    expect(isKbfPath("I18N/SRC/APP.KLF")).toBe(true);
    expect(isAnnotationPath("APP.OVERLAYS.JSONL")).toBe(true);
    expect(isOverlaySetPath("APP.OVERLAYS.JSON")).toBe(true);
  });

  it("pins the suffix constants the predicates are built from", () => {
    expect(Ext).toBe(".kbf.json");
    expect(ExtI18nReact).toBe(".klf");
    expect([...ReadableExts]).toEqual([Ext, ExtI18nReact]);
    expect(AnnotationExt).toBe(".overlays.jsonl");
    expect(OverlaySetExt).toBe(".overlays.json");
    // No suffix may be a suffix of another, or endsWith() would alias them.
    for (const [a, b] of [
      [Ext, AnnotationExt],
      [Ext, OverlaySetExt],
      [AnnotationExt, OverlaySetExt],
    ] as const) {
      expect(a.endsWith(b)).toBe(false);
      expect(b.endsWith(a)).toBe(false);
    }
  });
});

// One document, two spellings. Trimming is how the extractor tells that a
// catalog it wrote under the older suffix records the same source as the one
// it is writing now.
describe("trimKbfExt reduces both spellings to one stem", () => {
  it("strips either bundle suffix", () => {
    expect(trimKbfExt("i18n/src/App.kbf.json")).toBe("i18n/src/App");
    expect(trimKbfExt("i18n/src/App.klf")).toBe("i18n/src/App");
    expect(trimKbfExt("I18N/SRC/App.KLF")).toBe("I18N/SRC/App");
  });

  it("leaves a path carrying neither suffix alone", () => {
    expect(trimKbfExt("public/translations/fr.json")).toBe("public/translations/fr.json");
    expect(trimKbfExt("i18n/src/App.overlays.json")).toBe("i18n/src/App.overlays.json");
  });
});
