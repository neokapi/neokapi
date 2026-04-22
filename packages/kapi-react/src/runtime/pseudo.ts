/**
 * Runtime pseudo-translation — accent + expand every string that
 * flows through the kapi-react runtime, without touching a catalog.
 *
 * Unlike the CLI tool (`kapi pseudo-translate`) which builds a .klf
 * ahead of time, this module hooks directly into the runtime and
 * transforms strings on the fly. No build step; toggle on/off from
 * the browser console; tune the algorithm (markers, expansion,
 * alphabet) without rebuilding.
 *
 * Usage:
 *   import { setPseudoMode } from "@neokapi/kapi-react/runtime/pseudo";
 *
 *   setPseudoMode({});                                 // on, defaults
 *   setPseudoMode({ expansion: 30 });                  // +30% length
 *   setPseudoMode({ alphabet: "wobbly" });             // different accent set
 *   setPseudoMode({ expansion: 50, expansionChar: "·" });
 *   setPseudoMode(null);                               // off
 *
 * Stacks ON TOP of whatever the dict returns, so you can apply
 * pseudo to a real translation to see accent + expansion on the
 * actual target-language strings.
 */

import { setStringTransform } from "./index.js";

export type AlphabetName = "accented" | "wobbly" | "none";

export interface PseudoConfig {
  /** Characters prepended to every translated string. Default `"▒ "`. */
  prefix?: string;
  /** Characters appended to every translated string. Default `" ▒"`. */
  suffix?: string;
  /**
   * Extra expansion relative to the text length, 0..100. Fillers
   * (`expansionChar`) are distributed *between* the characters of the
   * source, so the expansion is visible mid-word — handy for layout
   * QA at any scale. 0 = off.
   */
  expansion?: number;
  /** Character inserted between letters for expansion. Default `"·"` (middle dot). */
  expansionChar?: string;
  /**
   * ASCII → replacement letter mapping. Named alphabets:
   *   - `"accented"` — uniform diacritic pass, close to the classic
   *     pseudo-localisation look (default).
   *   - `"wobbly"` — varied diacritics per letter, visually uneven.
   *   - `"none"` — pass letters through untouched (useful for
   *     padding-only / wrapping-only modes).
   * Or pass a custom `{ a: "ɐ", b: "q", ... }` map for full control.
   */
  alphabet?: AlphabetName | Record<string, string>;
}

const DEFAULT_PREFIX = "\u2592 "; // ▒ + space
const DEFAULT_SUFFIX = " \u2592";
const DEFAULT_EXPANSION_CHAR = "\u00b7"; // ·

/**
 * Well-known expansion characters consumers can surface in a picker
 * UI. Exported for reuse; the transform itself accepts any string.
 */
export const EXPANSION_CHAR_PRESETS: ReadonlyArray<{ value: string; label: string }> = [
  { value: "\u00b7", label: "Middle dot · (U+00B7)" },
  { value: "\u2022", label: "Bullet • (U+2022)" },
  { value: "\u2219", label: "Bullet operator ∙ (U+2219)" },
  { value: "\u2003", label: "Em space ␣ (U+2003)" },
  { value: "\u2009", label: "Thin space (U+2009)" },
  { value: "\u2500", label: "Box line ─ (U+2500)" },
  { value: "~", label: "Tilde ~" },
  { value: "-", label: "Hyphen -" },
  { value: "_", label: "Underscore _" },
];

// ── Alphabets ────────────────────────────────────────────────────

// Classic uniform accent pass — same set the Go CLI tool uses, so
// runtime-built pseudo matches catalog-built pseudo byte-for-byte.
const ACCENTED: Record<string, string> = {
  a: "\u00e0", b: "\u0183", c: "\u00e7", d: "\u0111", e: "\u00e9",
  f: "\u0192", g: "\u011d", h: "\u0125", i: "\u00ee", j: "\u0135",
  k: "\u0137", l: "\u013c", m: "\u1e3f", n: "\u00f1", o: "\u00f6",
  p: "\u00fe", q: "\u01eb", r: "\u0155", s: "\u0161", t: "\u0163",
  u: "\u00fc", v: "\u1e7d", w: "\u0175", x: "\u1e8b", y: "\u00fd",
  z: "\u017e",
  A: "\u00c0", B: "\u0182", C: "\u00c7", D: "\u0110", E: "\u00c9",
  F: "\u0191", G: "\u011c", H: "\u0124", I: "\u00ce", J: "\u0134",
  K: "\u0136", L: "\u013b", M: "\u1e3e", N: "\u00d1", O: "\u00d6",
  P: "\u00de", Q: "\u01ea", R: "\u0154", S: "\u0160", T: "\u0162",
  U: "\u00dc", V: "\u1e7c", W: "\u0174", X: "\u1e8a", Y: "\u00dd",
  Z: "\u017d",
};

// Varied diacritics per letter — macron, caron, acute, grave,
// circumflex, dot-above, diaeresis cycled across the alphabet so
// the result looks deliberately uneven (great for noticing layout
// assumptions that depend on consistent x-height or ascender runs).
const WOBBLY: Record<string, string> = {
  a: "\u0101", b: "\u1e03", c: "\u0109", d: "\u010f", e: "\u011b",
  f: "\u1e1f", g: "\u01e7", h: "\u1e27", i: "\u01d0", j: "\u0135",
  k: "\u01e9", l: "\u013a", m: "\u1e41", n: "\u01f9", o: "\u01d2",
  p: "\u1e55", q: "\u024b", r: "\u0159", s: "\u0161", t: "\u0165",
  u: "\u01d4", v: "\u1e7d", w: "\u0175", x: "\u1e8d", y: "\u1ef3",
  z: "\u017e",
  A: "\u0100", B: "\u1e02", C: "\u0108", D: "\u010e", E: "\u011a",
  F: "\u1e1e", G: "\u01e6", H: "\u1e26", I: "\u01cf", J: "\u0134",
  K: "\u01e8", L: "\u0139", M: "\u1e40", N: "\u01f8", O: "\u01d1",
  P: "\u1e54", Q: "\u024a", R: "\u0158", S: "\u0160", T: "\u0164",
  U: "\u01d3", V: "\u1e7c", W: "\u0174", X: "\u1e8c", Y: "\u1ef2",
  Z: "\u017d",
};

/**
 * Built-in alphabet map registry. Exported so UIs can list the
 * available presets without hard-coding the names.
 */
export const BUILT_IN_ALPHABETS: Record<AlphabetName, Record<string, string>> = {
  accented: ACCENTED,
  wobbly: WOBBLY,
  none: {},
};

function resolveAlphabet(name: PseudoConfig["alphabet"]): Record<string, string> {
  if (!name) return ACCENTED;
  if (typeof name === "string") return BUILT_IN_ALPHABETS[name] ?? ACCENTED;
  return name;
}

// ── Transform ────────────────────────────────────────────────────

/**
 * Transform a single string per the supplied config. Exported for
 * testing and for callers that want to apply the transform outside
 * the runtime hook.
 *
 * Placeholders in `{…}` are preserved verbatim — accent / wrap /
 * expansion skips brace contents so `{count}` (plain param) and
 * `{=m0}` (JSX element token) stay literal for downstream
 * substitution.
 */
export function pseudoTransform(text: string, config: PseudoConfig = {}): string {
  const prefix = config.prefix ?? DEFAULT_PREFIX;
  const suffix = config.suffix ?? DEFAULT_SUFFIX;
  const expansion = Math.max(0, Math.min(100, config.expansion ?? 0));
  const expansionChar = config.expansionChar ?? DEFAULT_EXPANSION_CHAR;
  const alphabet = resolveAlphabet(config.alphabet);

  let out = "";
  let depth = 0;
  // Running accumulator — once it crosses 1, insert a filler and
  // subtract 1. Spreads the expansion evenly across the string
  // instead of lumping it at the end.
  const rate = expansion / 100;
  let accum = 0;

  for (const ch of text) {
    if (ch === "{") {
      depth++;
      out += ch;
      continue;
    }
    if (ch === "}") {
      if (depth > 0) depth--;
      out += ch;
      continue;
    }
    if (depth > 0) {
      out += ch;
      continue;
    }
    // Letter: accent-replace (if mapped) and count for expansion.
    out += alphabet[ch] ?? ch;
    if (expansion > 0 && expansionChar.length > 0) {
      accum += rate;
      while (accum >= 1) {
        out += expansionChar;
        accum -= 1;
      }
    }
  }

  return prefix + out + suffix;
}

// ── Runtime hook wiring ──────────────────────────────────────────

let active: PseudoConfig | null = null;

/**
 * Install the pseudo transform into the runtime. Pass `null` to turn
 * it off. Pass `{}` to activate with default markers.
 *
 * Composes with real dicts: if a real translation is loaded, the
 * pseudo transform runs on THAT — handy for QAing a specific locale
 * with accents + expansion without touching the catalog.
 */
export function setPseudoMode(config: PseudoConfig | null): void {
  active = config;
  if (config === null) {
    setStringTransform(null);
    return;
  }
  setStringTransform((text) => pseudoTransform(text, config));
}

/** Current config, or null when pseudo mode is off. */
export function getPseudoMode(): PseudoConfig | null {
  return active;
}
