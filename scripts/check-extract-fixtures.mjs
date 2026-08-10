#!/usr/bin/env node
//
// Guard: test and story files are not product copy, and must reach neither the
// extractor's scan set nor a committed runtime catalog.
//
// Stage 1 of the dogfood loop scans React source and writes a source catalog;
// stage 3 hands every string in it to the translate stage and stage 4 compiles
// it into the dictionary each SPA loads at runtime. A fixture caught in stage 1
// therefore ships to users and is presented for translation as if a human had
// written it for them.
//
// Two checks, because either alone leaves a way back in:
//
//   1. Scan set. Each surface's real `--src`/`--ignore` flags are expanded
//      exactly as `neokapi-i18n extract` expands them (`fs/promises.glob` with
//      `{ exclude }`), and no yielded path may be a fixture. This catches a new
//      `--src` root added without the rooted ignores beside it — the shape of
//      the defect it exists for.
//
//      That defect: an --ignore is matched against the path glob yields, `../`
//      prefix and all, and `**` does not match a leading `..` segment. So a
//      bare `**/*.test.tsx` is dead for a `../`-prefixed root, silently, and
//      265 of 544 scanned files were fixtures (#1789).
//
//   2. Sentinel key. `App.test.tsx`'s JSX text "app" hashes to a key that was
//      present in the committed bowrain catalogs while the ignores were dead.
//      A key is FNV-1a-64 over text|description, so the sentinel is stable
//      against renames and vocabulary sweeps, and its reappearance means the
//      leak is back however it got back.
//
// `demo/` trees are deliberately not universal here: bowrain excludes its own,
// while kapi-desktop's src/demo is a real UI harness whose strings are seen.
// Each surface's ignore set says which it is; this guard checks the part that
// holds everywhere.
//
// Usage:
//     node scripts/check-extract-fixtures.mjs
//
// Wired into `make lint`.

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { glob } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// A fixture by name. Mirrors what every surface's ignore set excludes.
const FIXTURE = /(\.(test|stories)\.[jt]sx?$)|((^|\/)(__tests__|stories)\/)/;

// Committed runtime catalogs, and the key that must not be in one.
// "2Lo0Nu5k0DP" = hash of the JSX text "app" in
// bowrain/apps/bowrain/frontend/src/App.test.tsx.
const SENTINEL_KEY = "2Lo0Nu5k0DP";
const SENTINEL_CATALOGS = [
  "bowrain/apps/bowrain/frontend/public/translations/qps.json",
  "bowrain/apps/web/public/translations/qps.json",
];

// splitFlags tokenizes a make-expanded flag string, honouring the double
// quotes that keep a glob away from the shell.
function splitFlags(line) {
  const out = [];
  const re = /"([^"]*)"|(\S+)/g;
  let m;
  while ((m = re.exec(line)) !== null) out.push(m[1] ?? m[2]);
  return out;
}

function surfaces() {
  const raw = execFileSync("make", ["-s", "l10n-extract-globs"], {
    cwd: root,
    encoding: "utf8",
  });
  return raw
    .split("\n")
    .filter((l) => l.trim() !== "")
    .map((line) => {
      const [dir, flags] = line.split("\t");
      const tokens = splitFlags(flags ?? "");
      const src = [];
      const exclude = [];
      for (let i = 0; i < tokens.length; i++) {
        if (tokens[i] === "--src") src.push(tokens[++i]);
        else if (tokens[i] === "--ignore") exclude.push(tokens[++i]);
      }
      return { dir, src, exclude };
    });
}

let failed = false;

for (const { dir, src, exclude } of surfaces()) {
  if (src.length === 0) {
    console.error(`${dir}: no --src patterns — l10n-extract-globs is out of step`);
    failed = true;
    continue;
  }
  const seen = new Set();
  for (const pattern of src) {
    for await (const file of glob(pattern, { cwd: resolve(root, dir), exclude: [...exclude] })) {
      seen.add(file);
    }
  }
  const leaked = [...seen].filter((f) => FIXTURE.test(f)).sort();
  if (leaked.length > 0) {
    failed = true;
    console.error(
      `${dir}: ${leaked.length} fixture file(s) in the extract scan set ` +
        `(of ${seen.size} scanned):`,
    );
    for (const f of leaked.slice(0, 10)) console.error(`    ${f}`);
    if (leaked.length > 10) console.error(`    … and ${leaked.length - 10} more`);
    console.error(
      `  Each --src root needs the exclude set rooted at its own prefix — ` +
        `see src-ignores in the Makefile.`,
    );
  }
}

for (const rel of SENTINEL_CATALOGS) {
  const catalog = JSON.parse(readFileSync(resolve(root, rel), "utf8"));
  const messages = catalog.messages ?? catalog;
  if (Object.hasOwn(messages, SENTINEL_KEY)) {
    failed = true;
    console.error(
      `${rel}: carries the fixture sentinel key ${SENTINEL_KEY} ` +
        `(App.test.tsx's JSX text "app"). A test file is being extracted again.`,
    );
  }
}

if (failed) {
  console.error("\ncheck-extract-fixtures: fixtures are reaching the shipped catalogs");
  process.exit(1);
}

console.log("✓ no test or story file is extracted into a shipped catalog");
