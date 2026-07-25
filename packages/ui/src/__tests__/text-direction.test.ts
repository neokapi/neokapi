import { describe, it, expect } from "vitest";

import {
  directionAttrs,
  isRTLLocale,
  localeDirection,
  needsIsolation,
} from "../lib/text-direction";

describe("localeDirection", () => {
  it("resolves the RTL language subtags to rtl", () => {
    for (const tag of ["ar", "he", "fa", "ur", "ps", "sd", "ug", "yi", "dv", "ckb", "nqo", "syr"]) {
      expect(localeDirection(tag), tag).toBe("rtl");
    }
  });

  it("keeps region and variant subtags out of the decision", () => {
    expect(localeDirection("ar-EG")).toBe("rtl");
    expect(localeDirection("ar-SA-u-nu-arab")).toBe("rtl");
    expect(localeDirection("he-IL")).toBe("rtl");
    expect(localeDirection("fa-AF")).toBe("rtl");
  });

  it("does not mistake a Unicode extension value for a script subtag", () => {
    // `-u-nu-arab` asks for Arabic *numerals*; the language is still English.
    // BCP-47 puts the script immediately after the language, so only that
    // position may be read as one.
    expect(localeDirection("en-US-u-nu-arab")).toBe("ltr");
    expect(localeDirection("de-u-ca-hebr")).toBe("ltr");
    // …while a real script subtag in its proper position still decides.
    expect(localeDirection("az-Arab-IR-u-nu-latn")).toBe("rtl");
  });

  it("lets an explicit script subtag decide", () => {
    expect(localeDirection("az-Arab")).toBe("rtl");
    expect(localeDirection("az-Latn")).toBe("ltr");
    expect(localeDirection("pa-Arab-PK")).toBe("rtl");
    expect(localeDirection("pa-Guru-IN")).toBe("ltr");
    // An LTR script on an otherwise-RTL language still wins.
    expect(localeDirection("ku-Latn")).toBe("ltr");
    expect(localeDirection("ku-Arab")).toBe("rtl");
  });

  it("resolves ordinary LTR locales to ltr", () => {
    for (const tag of ["en", "en-US", "nb", "pt-BR", "pt-PT", "zh-Hans", "zh-Hant", "ja", "de"]) {
      expect(localeDirection(tag), tag).toBe("ltr");
    }
  });

  it("accepts POSIX-style separators", () => {
    expect(localeDirection("ar_EG")).toBe("rtl");
    expect(localeDirection("he_IL.UTF-8")).toBe("rtl");
    expect(localeDirection("en_US.UTF-8")).toBe("ltr");
  });

  it("is case-insensitive", () => {
    expect(localeDirection("AR-eg")).toBe("rtl");
    expect(localeDirection("az-ARAB")).toBe("rtl");
  });

  it("defaults unknown, empty, and missing tags to ltr", () => {
    expect(localeDirection("")).toBe("ltr");
    expect(localeDirection("   ")).toBe("ltr");
    expect(localeDirection(undefined)).toBe("ltr");
    expect(localeDirection(null)).toBe("ltr");
    expect(localeDirection("zz-ZZ")).toBe("ltr");
  });

  it("does not mistake a bare `ku` for Sorani", () => {
    // CLDR's default for the bare tag is Kurmanji (Latin script).
    expect(localeDirection("ku")).toBe("ltr");
  });
});

describe("isRTLLocale", () => {
  it("mirrors localeDirection", () => {
    expect(isRTLLocale("ar")).toBe(true);
    expect(isRTLLocale("en")).toBe(false);
    expect(isRTLLocale(undefined)).toBe(false);
  });
});

describe("directionAttrs", () => {
  it("always states dir, and lang when the tag is known", () => {
    expect(directionAttrs("ar-EG")).toEqual({ dir: "rtl", lang: "ar-EG" });
    expect(directionAttrs("en-US")).toEqual({ dir: "ltr", lang: "en-US" });
  });

  it("omits lang for an absent tag so no bogus language is asserted", () => {
    expect(directionAttrs(undefined)).toEqual({ dir: "ltr" });
    expect(directionAttrs("")).toEqual({ dir: "ltr" });
    expect(directionAttrs("  ")).toEqual({ dir: "ltr" });
  });
});

describe("needsIsolation", () => {
  it("is true exactly across a direction boundary", () => {
    expect(needsIsolation("en", "ar")).toBe(true);
    expect(needsIsolation("ar", "en")).toBe(true);
    expect(needsIsolation("ar", "he")).toBe(false);
    expect(needsIsolation("en", "nb")).toBe(false);
  });
});
