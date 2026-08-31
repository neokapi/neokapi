// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { createElement, act, type ReactElement } from "react";
import { createRoot } from "react-dom/client";

import {
  DirectionalText,
  directionAttrs,
  isRTLLocale,
  localeDirection,
  localeOfVariant,
  needsIsolation,
} from "../lib/text-direction";

function render(el: ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

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

describe("localeOfVariant", () => {
  it("strips a tone and/or channel suffix, keeping the bare locale", () => {
    expect(localeOfVariant("ar-EG")).toBe("ar-EG");
    expect(localeOfVariant("ar-EG#formal")).toBe("ar-EG");
    expect(localeOfVariant("ar-EG|social")).toBe("ar-EG");
    expect(localeOfVariant("ar-EG#formal|social")).toBe("ar-EG");
  });
});

describe("DirectionalText", () => {
  it("renders a <span> by default, with dir/lang derived from locale", () => {
    const c = render(createElement(DirectionalText, { locale: "ar" }, "مرحباً"));
    const el = c.firstElementChild!;
    expect(el.tagName).toBe("SPAN");
    expect(el.getAttribute("dir")).toBe("rtl");
    expect(el.getAttribute("lang")).toBe("ar");
  });

  it("renders as whatever element `as` names, carrying the same attributes", () => {
    for (const [as, tag] of [
      ["div", "DIV"],
      ["li", "LI"],
      ["td", "TD"],
      ["p", "P"],
      ["ul", "UL"],
    ] as const) {
      const c = render(createElement(DirectionalText, { as, locale: "he" }, "שלום"));
      const el = c.firstElementChild!;
      expect(el.tagName, as).toBe(tag);
      expect(el.getAttribute("dir"), as).toBe("rtl");
      expect(el.getAttribute("lang"), as).toBe("he");
    }
  });

  it("resolves ltr and omits lang for an unset locale, same as directionAttrs", () => {
    const c = render(createElement(DirectionalText, {}, "hello"));
    const el = c.firstElementChild!;
    expect(el.getAttribute("dir")).toBe("ltr");
    expect(el.getAttribute("lang")).toBeNull();
  });

  it("lets an explicit `dir` override the locale-derived direction", () => {
    // The one legitimate reason: content that stays ltr regardless of the
    // surrounding document's locale (e.g. source code).
    const c = render(createElement(DirectionalText, { as: "pre", locale: "ar", dir: "ltr" }, "x"));
    const el = c.firstElementChild!;
    expect(el.getAttribute("dir")).toBe("ltr");
    expect(el.getAttribute("lang")).toBeNull();
  });

  it("passes through every other prop unchanged (className, data-*, title, …)", () => {
    const c = render(
      createElement(
        DirectionalText,
        { locale: "ar", className: "foo", title: "a title", "data-block-id": "b1" },
        "x",
      ),
    );
    const el = c.firstElementChild!;
    expect(el.className).toBe("foo");
    expect(el.getAttribute("title")).toBe("a title");
    expect(el.getAttribute("data-block-id")).toBe("b1");
  });
});
