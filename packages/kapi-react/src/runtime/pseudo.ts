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
  a: "\u00e0",
  b: "\u0183",
  c: "\u00e7",
  d: "\u0111",
  e: "\u00e9",
  f: "\u0192",
  g: "\u011d",
  h: "\u0125",
  i: "\u00ee",
  j: "\u0135",
  k: "\u0137",
  l: "\u013c",
  m: "\u1e3f",
  n: "\u00f1",
  o: "\u00f6",
  p: "\u00fe",
  q: "\u01eb",
  r: "\u0155",
  s: "\u0161",
  t: "\u0163",
  u: "\u00fc",
  v: "\u1e7d",
  w: "\u0175",
  x: "\u1e8b",
  y: "\u00fd",
  z: "\u017e",
  A: "\u00c0",
  B: "\u0182",
  C: "\u00c7",
  D: "\u0110",
  E: "\u00c9",
  F: "\u0191",
  G: "\u011c",
  H: "\u0124",
  I: "\u00ce",
  J: "\u0134",
  K: "\u0136",
  L: "\u013b",
  M: "\u1e3e",
  N: "\u00d1",
  O: "\u00d6",
  P: "\u00de",
  Q: "\u01ea",
  R: "\u0154",
  S: "\u0160",
  T: "\u0162",
  U: "\u00dc",
  V: "\u1e7c",
  W: "\u0174",
  X: "\u1e8a",
  Y: "\u00dd",
  Z: "\u017d",
};

// "Wobbly" = letters that still read as their English source but
// look visibly bent off the upright baseline. The previous version
// cycled three italic variants — every letter slanted the same way,
// which defeats the point. Unicode has no reverse-italic Latin
// block, so true left-slant is unavailable; the next best thing is
// a cycle that mixes slant-right with no-slant and with ornate
// hand-drawn forms so adjacent letters sit at visibly different
// angles and weights.
//
// Cycle (letter index mod 3):
//   0 → Mathematical Italic      — uniform ~12° right slant
//   1 → Mathematical Sans-Serif  — fully upright, breaks the italic
//                                  cadence ("the straight one")
//   2 → Mathematical Script      — flowy hand-drawn forms whose
//                                  per-letter angle varies, often
//                                  apparently back-leaning on
//                                  ascenders like `𝒽` / `𝓁` / `ℯ`
//
// Reserved codepoints fall back to their Letterlike Symbols
// substitutes (U+210x–U+213x), noted per line.
//
// Each value is a string with a single codepoint — legal in JS
// source and iterated correctly by `for...of`.
const WOBBLY: Record<string, string> = {
  // Lowercase a–z, cycle italic → sans-serif → script.
  a: "\u{1D44E}", b: "\u{1D5BB}", c: "\u{1D4B8}",
  d: "\u{1D451}", e: "\u{1D5BE}", f: "\u{1D4BB}",
  g: "\u{1D454}", h: "\u{1D5C1}", i: "\u{1D4BE}",
  j: "\u{1D457}", k: "\u{1D5C4}", l: "\u{1D4C1}",
  m: "\u{1D45A}", n: "\u{1D5C7}", o: "\u{2134}", // script o reserved → ℴ
  p: "\u{1D45D}", q: "\u{1D5CA}", r: "\u{1D4C7}",
  s: "\u{1D460}", t: "\u{1D5CD}", u: "\u{1D4CA}",
  v: "\u{1D463}", w: "\u{1D5D0}", x: "\u{1D4CD}",
  y: "\u{1D466}", z: "\u{1D5D3}",
  // Uppercase A–Z, same cycle.
  A: "\u{1D434}", B: "\u{1D5A1}", C: "\u{1D49E}",
  D: "\u{1D437}", E: "\u{1D5A4}", F: "\u{2131}", // script F reserved → ℱ
  G: "\u{1D43A}", H: "\u{1D5A7}", I: "\u{2110}", // script I reserved → ℐ
  J: "\u{1D43D}", K: "\u{1D5AA}", L: "\u{2112}", // script L reserved → ℒ
  M: "\u{1D440}", N: "\u{1D5AD}", O: "\u{1D4AA}",
  P: "\u{1D443}", Q: "\u{1D5B0}", R: "\u{211B}", // script R reserved → ℛ
  S: "\u{1D446}", T: "\u{1D5B3}", U: "\u{1D4B0}",
  V: "\u{1D449}", W: "\u{1D5B6}", X: "\u{1D4B3}",
  Y: "\u{1D44C}", Z: "\u{1D5B9}",
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
 *
 * Expansion placement prioritises readability:
 *   - `expansion >= 100` → between every character (full interleave).
 *     Needed to match the target length when it exceeds the source.
 *   - `expansion < 100` with spaces available → at word boundaries
 *     only, distributed evenly. Mid-word filler is avoided so the
 *     text stays scannable.
 *   - `expansion < 100` with no spaces → split between start and end.
 *     Last-resort pad for single-word strings.
 */
export function pseudoTransform(text: string, config: PseudoConfig = {}): string {
  const prefix = config.prefix ?? DEFAULT_PREFIX;
  const suffix = config.suffix ?? DEFAULT_SUFFIX;
  const expansion = Math.max(0, Math.min(100, config.expansion ?? 0));
  const expansionChar = config.expansionChar ?? DEFAULT_EXPANSION_CHAR;
  const alphabet = resolveAlphabet(config.alphabet);

  // Pass 1: walk the source, building per-position metadata. Keeps
  // brace content literal (letter=false) so filler never lands
  // inside a placeholder.
  const chars: string[] = [];
  const isLetter: boolean[] = [];
  const isSpace: boolean[] = [];
  let depth = 0;
  for (const ch of text) {
    if (ch === "{") {
      depth++;
      chars.push(ch);
      isLetter.push(false);
      isSpace.push(false);
      continue;
    }
    if (ch === "}") {
      if (depth > 0) depth--;
      chars.push(ch);
      isLetter.push(false);
      isSpace.push(false);
      continue;
    }
    if (depth > 0) {
      chars.push(ch);
      isLetter.push(false);
      isSpace.push(false);
      continue;
    }
    const whitespace = ch === " " || ch === "\t" || ch === "\n";
    chars.push(alphabet[ch] ?? ch);
    isLetter.push(!whitespace);
    isSpace.push(whitespace);
  }

  // Pass 2: decide filler placement.
  let body = "";
  if (expansion === 0 || !expansionChar) {
    body = chars.join("");
  } else if (expansion >= 100) {
    // Full interleave — between every letter, at the configured rate.
    const rate = expansion / 100;
    let accum = 0;
    for (let i = 0; i < chars.length; i++) {
      body += chars[i];
      if (!isLetter[i]) continue;
      accum += rate;
      while (accum >= 1) {
        body += expansionChar;
        accum -= 1;
      }
    }
  } else {
    // Sub-100%: keep words intact. Count letters + space positions,
    // place the target number of fillers at word boundaries first;
    // spill remainder to start + end if we run out of spaces.
    const letterCount = isLetter.reduce((n, b) => n + (b ? 1 : 0), 0);
    let wanted = Math.round((letterCount * expansion) / 100);
    const spaceIndices: number[] = [];
    for (let i = 0; i < isSpace.length; i++) {
      if (isSpace[i]) spaceIndices.push(i);
    }

    const insertAfter: number[] = Array.from({ length: chars.length }, () => 0);
    let leading = 0;
    let trailing = 0;

    if (spaceIndices.length === 0) {
      // No word boundaries — pad the outside.
      leading = Math.ceil(wanted / 2);
      trailing = wanted - leading;
    } else if (wanted <= spaceIndices.length) {
      // Pick `wanted` evenly-spaced space positions.
      for (let k = 0; k < wanted; k++) {
        const idx = spaceIndices[Math.floor((k * spaceIndices.length) / wanted)];
        insertAfter[idx] += 1;
      }
    } else {
      // More fillers than spaces: put one at every space, then
      // distribute the remainder split across start and end so the
      // padding stays visible without cramming mid-word.
      for (const idx of spaceIndices) insertAfter[idx] += 1;
      const remaining = wanted - spaceIndices.length;
      leading = Math.ceil(remaining / 2);
      trailing = remaining - leading;
    }

    body += expansionChar.repeat(leading);
    for (let i = 0; i < chars.length; i++) {
      body += chars[i];
      if (insertAfter[i] > 0) body += expansionChar.repeat(insertAfter[i]);
    }
    body += expansionChar.repeat(trailing);
  }

  return prefix + body + suffix;
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
