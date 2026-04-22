/**
 * Client-side pseudo-translation for the visual editor preview.
 *
 * Mirrors the accent mapping from framework/core/tools/pseudo.go so that the
 * "Pseudo" preview mode can render source text as pseudo-translated on the fly
 * without a server round-trip.
 */

const accentMap: Record<string, string> = {
  a: "\u00e0", // à
  b: "\u0183", // ƃ
  c: "\u00e7", // ç
  d: "\u0111", // đ
  e: "\u00e9", // é
  f: "\u0192", // ƒ
  g: "\u011d", // ĝ
  h: "\u0125", // ĥ
  i: "\u00ee", // î
  j: "\u0135", // ĵ
  k: "\u0137", // ķ
  l: "\u013c", // ļ
  m: "\u1e3f", // ḿ
  n: "\u00f1", // ñ
  o: "\u00f6", // ö
  p: "\u00fe", // þ
  q: "\u01eb", // ǫ
  r: "\u0155", // ŕ
  s: "\u0161", // š
  t: "\u0163", // ţ
  u: "\u00fc", // ü
  v: "\u1e7d", // ṽ
  w: "\u0175", // ŵ
  x: "\u1e8b", // ẋ
  y: "\u00fd", // ý
  z: "\u017e", // ž
  A: "\u00c0", // À
  B: "\u0182", // Ƃ
  C: "\u00c7", // Ç
  D: "\u0110", // Đ
  E: "\u00c9", // É
  F: "\u0191", // Ƒ
  G: "\u011c", // Ĝ
  H: "\u0124", // Ĥ
  I: "\u00ce", // Î
  J: "\u0134", // Ĵ
  K: "\u0136", // Ķ
  L: "\u013b", // Ļ
  M: "\u1e3e", // Ḿ
  N: "\u00d1", // Ñ
  O: "\u00d6", // Ö
  P: "\u00de", // Þ
  Q: "\u01ea", // Ǫ
  R: "\u0154", // Ŕ
  S: "\u0160", // Š
  T: "\u0162", // Ţ
  U: "\u00dc", // Ü
  V: "\u1e7c", // Ṽ
  W: "\u0174", // Ŵ
  X: "\u1e8a", // Ẋ
  Y: "\u00dd", // Ý
  Z: "\u017d", // Ž
};

/** Apply pseudo-translation accent mapping to plain text. */
export function pseudoTranslate(text: string): string {
  let result = "";
  for (const ch of text) {
    result += accentMap[ch] ?? ch;
  }
  return "\u2592 " + result + " \u2592";
}

/**
 * Apply pseudo-translation to coded text, preserving inline span markers
 * (Unicode private use area characters U+E001–U+E003).
 */
export function pseudoTranslateCoded(coded: string): string {
  let result = "";
  for (const ch of coded) {
    const code = ch.codePointAt(0) ?? 0;
    if (code >= 0xe001 && code <= 0xe003) {
      result += ch;
    } else {
      result += accentMap[ch] ?? ch;
    }
  }
  return "\u2592 " + result + " \u2592";
}
