/**
 * Client-side pseudo-translation for the visual editor preview.
 *
 * Mirrors the accent mapping from framework/core/tools/pseudo.go so that the
 * "Pseudo" preview mode can render source text as pseudo-translated on the fly
 * without a server round-trip.
 */

const accentMap: Record<string, string> = {
  a: "à", // à
  b: "ƃ", // ƃ
  c: "ç", // ç
  d: "đ", // đ
  e: "é", // é
  f: "ƒ", // ƒ
  g: "ĝ", // ĝ
  h: "ĥ", // ĥ
  i: "î", // î
  j: "ĵ", // ĵ
  k: "ķ", // ķ
  l: "ļ", // ļ
  m: "ḿ", // ḿ
  n: "ñ", // ñ
  o: "ö", // ö
  p: "þ", // þ
  q: "ǫ", // ǫ
  r: "ŕ", // ŕ
  s: "š", // š
  t: "ţ", // ţ
  u: "ü", // ü
  v: "ṽ", // ṽ
  w: "ŵ", // ŵ
  x: "ẋ", // ẋ
  y: "ý", // ý
  z: "ž", // ž
  A: "À", // À
  B: "Ƃ", // Ƃ
  C: "Ç", // Ç
  D: "Đ", // Đ
  E: "É", // É
  F: "Ƒ", // Ƒ
  G: "Ĝ", // Ĝ
  H: "Ĥ", // Ĥ
  I: "Î", // Î
  J: "Ĵ", // Ĵ
  K: "Ķ", // Ķ
  L: "Ļ", // Ļ
  M: "Ḿ", // Ḿ
  N: "Ñ", // Ñ
  O: "Ö", // Ö
  P: "Þ", // Þ
  Q: "Ǫ", // Ǫ
  R: "Ŕ", // Ŕ
  S: "Š", // Š
  T: "Ţ", // Ţ
  U: "Ü", // Ü
  V: "Ṽ", // Ṽ
  W: "Ŵ", // Ŵ
  X: "Ẋ", // Ẋ
  Y: "Ý", // Ý
  Z: "Ž", // Ž
};

/** Accent the letters of one text run; the shade markers wrap the whole block. */
export function pseudoAccent(text: string): string {
  let result = "";
  for (const ch of text) {
    result += accentMap[ch] ?? ch;
  }
  return result;
}

/** The shade markers a pseudo-translated block is wrapped in. */
export function pseudoWrap(text: string): string {
  return "▒ " + text + " ▒";
}

/** Apply pseudo-translation accent mapping to plain text. */
export function pseudoTranslate(text: string): string {
  return pseudoWrap(pseudoAccent(text));
}
