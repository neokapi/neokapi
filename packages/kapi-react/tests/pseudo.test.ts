import { describe, it, expect, beforeEach } from "vitest";

import {
  __t,
  __tx,
  setTranslations,
  t as jsT,
  setStringTransform,
} from "../src/runtime/index.ts";
import {
  pseudoTransform,
  setPseudoMode,
  getPseudoMode,
} from "../src/runtime/pseudo.ts";

describe("pseudoTransform", () => {
  it("accents ASCII letters and wraps with default markers", () => {
    expect(pseudoTransform("Hello")).toBe("\u2592 \u0124\u00e9\u013c\u013c\u00f6 \u2592");
  });

  it("preserves {placeholder} tokens verbatim", () => {
    const out = pseudoTransform("{count} items found");
    expect(out).toContain("{count}"); // name untouched
    expect(out).not.toContain("{\u00e7\u00f6\u00fc\u00f1\u0163}"); // wasn't accented inside braces
  });

  it("preserves {=m0} JSX element tokens verbatim", () => {
    const out = pseudoTransform("Click {=m0} to continue");
    expect(out).toContain("{=m0}");
  });

  it("honours a custom prefix/suffix", () => {
    const out = pseudoTransform("Save", { prefix: "[", suffix: "]" });
    expect(out.startsWith("[")).toBe(true);
    expect(out.endsWith("]")).toBe(true);
  });

  it("adds expansion padding at the configured percent", () => {
    const out = pseudoTransform("Save", { expansion: 100 });
    // accented length is 4 chars → 100% padding ~= 4 tildes
    expect(out).toMatch(/~~~~/);
  });

  it("turns off accents when accent: false", () => {
    const out = pseudoTransform("Hello", { accent: false });
    expect(out).toContain("Hello");
    expect(out).not.toContain("\u0124"); // Ĥ
  });
});

describe("setPseudoMode + runtime integration", () => {
  beforeEach(() => {
    setPseudoMode(null);
    setTranslations("en", {});
  });

  it("wraps __t() output when active", () => {
    setPseudoMode({});
    expect(__t("missingHash", "Save")).toMatch(/^\u2592 .* \u2592$/);
  });

  it("preserves {param} tokens through substitution", () => {
    setPseudoMode({});
    const out = __t("missingHash", "Hello, {name}!", { name: "World" });
    // param was substituted normally; only surrounding text accented
    expect(out).toContain("World");
    expect(out).not.toContain("{name}");
  });

  it("applies on top of a loaded dict entry", () => {
    setTranslations("de", { someHash: "Speichern" });
    setPseudoMode({});
    const out = __t("someHash", "Save");
    expect(out).toContain("\u0160"); // Š for S in "Speichern"
  });

  it("wraps __tx() output too", () => {
    setPseudoMode({});
    const out = __tx("missingHash", "Click {=m0} please", { "=m0": "X" }, {});
    // out is a ReactNode array with wrapped static text
    expect(JSON.stringify(out)).toMatch(/\u2592/);
  });

  it("passes through when disabled (null)", () => {
    setPseudoMode(null);
    expect(__t("missingHash", "Save")).toBe("Save");
  });

  it("getPseudoMode() returns the current config", () => {
    setPseudoMode({ expansion: 30 });
    expect(getPseudoMode()).toEqual({ expansion: 30 });
    setPseudoMode(null);
    expect(getPseudoMode()).toBeNull();
  });

  it("leaves t() escape-hatch text pseudo-wrapped too", () => {
    setPseudoMode({});
    // t() is the dev-mode no-op; just substitutes params. With
    // pseudo on, its output STAYS raw because t() runs independently
    // of __t — the plugin rewrites it in production. Documented.
    // Here we confirm t() itself doesn't try to apply pseudo (by
    // design, since plugin replaces t() with __t() before shipping).
    expect(jsT("Save")).toBe("Save");
  });

  it("custom setStringTransform still works (pseudo uses this hook)", () => {
    setStringTransform((s) => s.toUpperCase());
    expect(__t("missingHash", "save")).toBe("SAVE");
    setStringTransform(null);
  });
});
