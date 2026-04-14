#!/usr/bin/env node
/**
 * Generate pseudo-English (qps) translations from an extracted strings.json.
 *
 * Wraps each segment in [~...~] with padding proportional to expansion
 * percent, but leaves `{name}` placeholders untouched so the runtime can
 * substitute real values at render time.
 *
 * Usage: node gen-qps.mjs <strings.json> <out.qps.json> [expansion-percent]
 */
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname } from "node:path";

const [, , stringsPath, outPath, expandArg] = process.argv;
if (!stringsPath || !outPath) {
  console.error("usage: gen-qps.mjs <strings.json> <out.qps.json> [expansion-percent]");
  process.exit(2);
}
const expansion = Number(expandArg ?? "20");

// Lowercase/uppercase accent maps matching `kapi pseudo-translate`.
const MAP = {
  a: "à", b: "ƃ", c: "ç", d: "đ", e: "é", f: "ƒ", g: "ĝ", h: "ĥ", i: "î",
  j: "ĵ", k: "ķ", l: "ļ", m: "ḿ", n: "ñ", o: "ö", p: "þ", q: "ǫ", r: "ŕ",
  s: "š", t: "ţ", u: "ü", v: "ṽ", w: "ŵ", x: "ẋ", y: "ý", z: "ž",
  A: "À", B: "Ƃ", C: "Ç", D: "Đ", E: "É", F: "Ƒ", G: "Ĝ", H: "Ĥ", I: "Î",
  J: "Ĵ", K: "Ķ", L: "Ļ", M: "Ḿ", N: "Ñ", O: "Ö", P: "Þ", Q: "Ǫ", R: "Ŕ",
  S: "Š", T: "Ţ", U: "Ü", V: "Ṽ", W: "Ŵ", X: "Ẋ", Y: "Ý", Z: "Ž",
};

const PLACEHOLDER = /\{[^}]+\}/g;

function accent(text) {
  let out = "";
  for (const ch of text) out += MAP[ch] ?? ch;
  return out;
}

function pad(text) {
  const n = Math.ceil((text.length * expansion) / 100);
  return ` ${"~".repeat(Math.max(1, n))}`;
}

function pseudoTranslate(text) {
  const parts = [];
  let last = 0;
  let match;
  while ((match = PLACEHOLDER.exec(text)) !== null) {
    parts.push(accent(text.slice(last, match.index)));
    parts.push(match[0]); // preserved verbatim
    last = match.index + match[0].length;
  }
  parts.push(accent(text.slice(last)));
  return `[${parts.join("")}${pad(text)}]`;
}

const input = JSON.parse(readFileSync(stringsPath, "utf-8"));
const out = Object.fromEntries(input.strings.map((e) => [e.hash, pseudoTranslate(e.text)]));

mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, JSON.stringify(out, null, 2) + "\n");
console.log(`Wrote ${Object.keys(out).length} strings → ${outPath}`);
