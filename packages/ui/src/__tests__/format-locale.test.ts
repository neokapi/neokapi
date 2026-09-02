import { describe, it, expect } from "vitest";
import { formatLocale, localeLabel, resolveLocaleName } from "../lib/locale-name";

// CLDR names every well-formed tag, so these expectations are ICU's own output
// rather than a table this repo maintains.
describe("formatLocale", () => {
  it("names a regional tag and keeps the code beside it", () => {
    expect(formatLocale("fr-FR")).toEqual({
      name: "French (France)",
      code: "fr-FR",
      text: "French (France)",
      title: "French (France) (fr-FR)",
    });
  });

  it("uses CLDR's own name for a tag that has one", () => {
    expect(formatLocale("pt-BR").name).toBe("Brazilian Portuguese");
  });

  it("names a script subtag", () => {
    expect(formatLocale("zh-Hant").name).toBe("Traditional Chinese");
  });

  it("names a language, script and region together", () => {
    expect(formatLocale("sr-Latn-RS").name).toBe("Serbian (Latin, Serbia)");
  });

  it("falls back to the code when CLDR has no name", () => {
    expect(formatLocale("qps")).toEqual({
      name: "qps",
      code: "qps",
      text: "qps",
      title: "qps",
    });
  });

  it("falls back to the code for a malformed tag rather than throwing", () => {
    expect(formatLocale("not a locale").name).toBe("not a locale");
    expect(formatLocale("").code).toBe("");
  });

  it("names the locale in the UI language", () => {
    expect(formatLocale("fr-FR", { uiLocale: "fr" }).name).toBe("français (France)");
    expect(formatLocale("fr-FR", { uiLocale: "nb" }).name).toBe("fransk (Frankrike)");
    expect(formatLocale("de", { uiLocale: "nb" }).name).toBe("tysk");
  });

  it("draws the code in a compact context and keeps the name in the title", () => {
    const compact = formatLocale("fr-FR", { compact: true });
    expect(compact.text).toBe("fr-FR");
    expect(compact.title).toBe("French (France) (fr-FR)");
    expect(compact.name).toBe("French (France)");
  });

  it("drops the region for the short variant", () => {
    expect(formatLocale("fr-FR", { variant: "short" }).name).toBe("French");
    expect(formatLocale("pt-BR", { variant: "short" }).name).toBe("Portuguese");
    expect(formatLocale("sr-Latn-RS", { variant: "short" }).name).toBe("Serbian");
  });

  // A BCP 47 tag carries meaning in its casing, so the helper hands back what it
  // was given and every renderer draws that.
  it("returns the tag exactly as given", () => {
    for (const tag of ["zh-Hant", "sr-Latn-RS", "pt-br", "FR-fr"]) {
      expect(formatLocale(tag).code).toBe(tag);
    }
  });
});

describe("resolveLocaleName", () => {
  it("keeps naming in English by default", () => {
    expect(resolveLocaleName("fr")).toBe("French");
    expect(resolveLocaleName("pt-BR")).toBe("Brazilian Portuguese");
  });

  it("takes the UI language as its second argument", () => {
    expect(resolveLocaleName("fr", "nb")).toBe("fransk");
  });

  it("names the language alone for the short variant", () => {
    expect(resolveLocaleName("fr-FR", "en", "short")).toBe("French");
  });
});

describe("localeLabel", () => {
  it("pairs the name with the code for a single-string context", () => {
    expect(localeLabel("fr")).toBe("French (fr)");
  });

  it("returns an unnamed code alone rather than doubling it", () => {
    expect(localeLabel("qps")).toBe("qps");
  });

  it("names in the UI language when one is given", () => {
    expect(localeLabel("fr", "nb")).toBe("fransk (fr)");
  });
});
