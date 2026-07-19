// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";

import { setTranslations, syncDocumentLocale } from "../src/runtime/index.ts";

/**
 * The runtime tests live in a Node environment (no `document`).
 * `setTranslations`' DOM side-effect needs a browser-like environment
 * to exercise, so this file is kept separate and pinned to jsdom via
 * the @vitest-environment pragma.
 */
describe("setTranslations — document locale sync", () => {
  beforeEach(() => {
    document.documentElement.removeAttribute("lang");
    document.documentElement.removeAttribute("dir");
  });

  it("pushes the locale onto <html lang>", () => {
    setTranslations("fr", {});
    expect(document.documentElement.lang).toBe("fr");
  });

  it("defaults dir to ltr for most locales", () => {
    setTranslations("fr-CA", {});
    expect(document.documentElement.dir).toBe("ltr");
  });

  it("sets dir=rtl for the common RTL scripts", () => {
    for (const l of ["ar", "ar-EG", "he", "fa-IR", "ur", "yi", "ps", "sd", "dv"]) {
      setTranslations(l, {});
      expect(document.documentElement.dir, `dir for ${l}`).toBe("rtl");
    }
  });

  it("accepts underscore-separated locales (POSIX style)", () => {
    setTranslations("de_AT", {});
    expect(document.documentElement.lang).toBe("de_AT");
    expect(document.documentElement.dir).toBe("ltr");
  });

  it("honours the syncDocumentLocale opt-out", () => {
    document.documentElement.setAttribute("lang", "en");
    setTranslations("de", {}, { syncDocumentLocale: false });
    expect(document.documentElement.lang).toBe("en");
  });

  it("exposes syncDocumentLocale() for callers that want to sync without swapping dict", () => {
    syncDocumentLocale("ja-JP");
    expect(document.documentElement.lang).toBe("ja-JP");
    expect(document.documentElement.dir).toBe("ltr");
  });
});
