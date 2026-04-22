/**
 * Runtime pseudo-translation — accent + wrap every string that
 * flows through the kapi-react runtime, without touching a catalog.
 *
 * Unlike the CLI tool (`kapi pseudo-translate`) which builds a .klf
 * ahead of time, this module hooks directly into the runtime and
 * transforms strings on the fly. No build step; toggle on/off from
 * the browser console; tune the algorithm (markers, expansion,
 * accent pass) without rebuilding.
 *
 * Usage:
 *   import { setPseudoMode } from "@neokapi/kapi-react/runtime/pseudo";
 *
 *   // Turn on with defaults (▒ prefix/suffix, no expansion, accents on)
 *   setPseudoMode({});
 *
 *   // Tune
 *   setPseudoMode({ prefix: "« ", suffix: " »", expansion: 30 });
 *
 *   // Off
 *   setPseudoMode(null);
 *
 * Stacks ON TOP of whatever the dict returns — so you can apply
 * pseudo to a real translation to see how a specific locale's
 * strings look accented and expanded.
 */

import { setStringTransform } from "./index.js";

export interface PseudoConfig {
  /** Characters prepended to every translated string. Default `"▒ "`. */
  prefix?: string;
  /** Characters appended to every translated string. Default `" ▒"`. */
  suffix?: string;
  /**
   * Extra padding percent (relative to the accented length). Simulates
   * translation expansion for layout QA. 0 = no padding. Default 0.
   */
  expansion?: number;
  /**
   * Whether to replace ASCII letters with accented equivalents.
   * Default `true`. Turn off for a padding-only mode.
   */
  accent?: boolean;
}

const DEFAULT_PREFIX = "\u2592 "; // ▒ + space
const DEFAULT_SUFFIX = " \u2592";

// ASCII → accented letter map. Matches the kapi CLI pseudo tool so
// catalog-built and runtime-built pseudo strings look the same.
const ACCENT_MAP: Record<string, string> = {
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

/**
 * Transform a single string per the supplied config. Exported for
 * testing and for callers that want to apply the transform outside
 * the runtime hook.
 *
 * Placeholders in `{…}` are preserved verbatim — accent / wrap pass
 * skips brace contents so `{count}` (plain param) and `{=m0}` (JSX
 * element token) stay literal for downstream substitution.
 */
export function pseudoTransform(text: string, config: PseudoConfig = {}): string {
  const prefix = config.prefix ?? DEFAULT_PREFIX;
  const suffix = config.suffix ?? DEFAULT_SUFFIX;
  const expansion = config.expansion ?? 0;
  const accent = config.accent ?? true;

  let out = "";
  let depth = 0;
  let textLen = 0; // count of transformed runes for expansion sizing

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
    if (accent) {
      const replacement = ACCENT_MAP[ch];
      out += replacement ?? ch;
    } else {
      out += ch;
    }
    textLen++;
  }

  if (expansion > 0 && textLen > 0) {
    const padLen = Math.floor((textLen * expansion) / 100);
    if (padLen > 0) out += " " + "~".repeat(padLen);
  }

  return prefix + out + suffix;
}

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
