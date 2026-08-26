// Lift the landing shell's head strings out of a translated index.html into
// the small JSON the Vite build swaps in per locale.
//
// The head is prose: the browser tab and the social card. It was the one part
// of the landing that stayed English in every locale, because locale-meta.json
// carried head strings only if someone typed them, and typing a translation by
// hand is the thing this repository does not do. kapi reads the shell like any
// other content, so the strings can come from the same pass as the rest.
//
// Usage: node scripts/landing-head.mjs <translated.html> <out.json>

import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

const [input, output] = process.argv.slice(2);
if (!input || !output) {
  console.error("usage: landing-head.mjs <translated.html> <out.json>");
  process.exit(2);
}

const html = readFileSync(input, "utf8");

// Each field, and the pattern that finds it. The attribute order and the
// whitespace are the shell's own, so these mirror what vite.config.ts swaps
// rather than a general HTML parse: if the shell is reformatted, both sides
// notice, and an empty result here is reported below rather than written.
const FIELDS = [
  ["title", /<title>([^<]*)<\/title>/],
  ["description", /<meta\s+name="description"\s+content="([^"]*)"/],
  ["ogTitle", /<meta\s+property="og:title"\s+content="([^"]*)"/],
  ["ogDescription", /<meta\s+property="og:description"\s+content="([^"]*)"/],
];

const head = {};
const missing = [];
for (const [key, pattern] of FIELDS) {
  const m = html.match(pattern);
  if (m && m[1].trim()) {
    head[key] = m[1];
  } else {
    missing.push(key);
  }
}

if (missing.length) {
  // Not fatal: a field the shell no longer carries should not fail the loop.
  // Silence would be worse — the locale would quietly fall back to English.
  console.warn(
    `[landing-head] ${input}: no value found for ${missing.join(", ")}; ` +
      `those fields will fall back to the source language`,
  );
}

if (Object.keys(head).length === 0) {
  console.error(`[landing-head] ${input}: nothing extracted; refusing to write an empty head`);
  process.exit(1);
}

mkdirSync(dirname(output), { recursive: true });
writeFileSync(output, `${JSON.stringify(head, null, 2)}\n`);
console.log(`[landing-head] ${Object.keys(head).length} fields → ${output}`);
