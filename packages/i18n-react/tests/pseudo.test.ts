import { describe, it, expect, beforeEach } from "vitest";

import { __t, __tx, setTranslations, t as jsT, setStringTransform } from "../src/runtime/index.ts";
import { pseudoTransform, setPseudoMode, getPseudoMode } from "../src/runtime/pseudo.ts";

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

  it("at 100% expansion, interleaves a filler after every letter", () => {
    // 4 letters at 100% → 4 fillers, one per letter.
    const out = pseudoTransform("Save", { expansion: 100 });
    expect((out.match(/\u00b7/g) ?? []).length).toBe(4);
  });

  it("at sub-100% with spaces, places fillers only at word boundaries", () => {
    // "Hello world" has 1 space. 20% × 10 letters = 2 fillers wanted.
    // 2 > 1 space → one at the space, one remaining split start/end.
    const out = pseudoTransform("Hello world", { expansion: 20 });
    // The word "Hello" should NOT have any mid-word middle-dot.
    const body = out.slice("\u2592 ".length, -" \u2592".length);
    // Letters of "Hello" in accented form — no filler between them.
    const helloAccented = "\u0124\u00e9\u013c\u013c\u00f6"; // Ĥéļļö
    expect(body).toContain(helloAccented);
    // Two fillers in total.
    expect((out.match(/\u00b7/g) ?? []).length).toBe(2);
  });

  it("at sub-100% with enough spaces, distributes among spaces only (no start/end pad)", () => {
    // "a b c d e" — 4 spaces; 5 letters at 40% → 2 fillers wanted.
    // Both land at spaces, none at start/end.
    const out = pseudoTransform("a b c d e", { expansion: 40 });
    expect(out.startsWith("\u2592 \u00e0")).toBe(true); // starts with accented 'a'
    expect(out.endsWith("\u00e9 \u2592")).toBe(true); // ends with accented 'e'
    expect((out.match(/\u00b7/g) ?? []).length).toBe(2);
  });

  it("at sub-100% with no spaces, pads start and end", () => {
    // Single word, no space boundaries → fillers go outside the word.
    const out = pseudoTransform("Save", { expansion: 50 });
    // body between the prefix/suffix markers
    const body = out.slice("\u2592 ".length, -" \u2592".length);
    // Accented "Save" = "Šàṽé"; it should appear contiguous (no
    // mid-word filler).
    expect(body).toContain("\u0160\u00e0\u1e7d\u00e9"); // Šàṽé
    expect(body.includes("\u0160\u00b7")).toBe(false); // no dot after Š
  });

  it("honours a custom expansionChar", () => {
    const out = pseudoTransform("Save", { expansion: 100, expansionChar: "-" });
    expect((out.match(/-/g) ?? []).length).toBe(4);
    expect(out).not.toContain("\u00b7");
  });

  it("doesn't insert fillers inside {placeholder} braces", () => {
    const out = pseudoTransform("{count} items", { expansion: 100 });
    expect(out).toContain("{count}"); // braces preserved literally
  });

  it("selects the wobbly alphabet when asked", () => {
    const out = pseudoTransform("Save", { alphabet: "wobbly" });
    // Wobbly cycles Mathematical Italic → Sans-Serif → Script per
    // letter position, so S/a/v (positions 0 mod 3) come out italic
    // while e (position 4 → 1 mod 3) lands in the sans-serif block.
    expect(out).toContain("\u{1D446}"); // S italic → 𝑆
    expect(out).toContain("\u{1D44E}"); // a italic → 𝑎
    expect(out).toContain("\u{1D463}"); // v italic → 𝑣
    expect(out).toContain("\u{1D5BE}"); // e sans-serif → 𝖾
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
