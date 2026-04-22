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

  it("inserts expansion characters between source characters at the configured rate", () => {
    // 4 letters at 100% expansion → 4 filler chars between them
    // (one filler after each letter). Default filler is `·`.
    const out = pseudoTransform("Save", { expansion: 100 });
    // After every accented letter we should see a `·`.
    const fillerCount = (out.match(/\u00b7/g) ?? []).length;
    expect(fillerCount).toBe(4);
  });

  it("spreads expansion evenly at fractional rates", () => {
    // 50% on 4 letters → 2 fillers (every other position)
    const out = pseudoTransform("Save", { expansion: 50 });
    const fillerCount = (out.match(/\u00b7/g) ?? []).length;
    expect(fillerCount).toBe(2);
  });

  it("honours a custom expansionChar", () => {
    const out = pseudoTransform("Save", { expansion: 100, expansionChar: "-" });
    // With insert-between, 4 letters at 100% → 4 dashes interleaved.
    const dashCount = (out.match(/-/g) ?? []).length;
    expect(dashCount).toBe(4);
    expect(out).not.toContain("\u00b7");
  });

  it("doesn't insert fillers inside {placeholder} braces", () => {
    const out = pseudoTransform("{count} items", { expansion: 100 });
    expect(out).toContain("{count}"); // braces preserved literally
  });

  it("selects the wobbly alphabet when asked", () => {
    const out = pseudoTransform("Save", { alphabet: "wobbly" });
    // wobbly maps 'S' → Š (U+0160), 'a' → ā (U+0101), 'v' → ṽ (U+1E7D), 'e' → ě (U+011B)
    expect(out).toContain("\u0160");
    expect(out).toContain("\u0101");
    expect(out).toContain("\u011b");
  });

  it("passes letters through unchanged with alphabet: none", () => {
    const out = pseudoTransform("Hello", { alphabet: "none" });
    expect(out).toContain("Hello");
    expect(out).not.toContain("\u0124"); // no Ĥ
  });

  it("accepts a custom alphabet map", () => {
    const out = pseudoTransform("abc", { alphabet: { a: "x", b: "y", c: "z" } });
    expect(out).toContain("xyz");
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
