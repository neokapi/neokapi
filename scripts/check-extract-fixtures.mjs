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
// Three checks, because no two of them add up to the third:
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
//   3. Invocation parity. A surface is extracted from two places — a make
//      target and, where one exists, the package's own `extract` script — and
//      the two must pass the same flags. A `--config` on one side only is the
//      shape that defect takes: the componentMap decides how a JSX path
//      resolves, a key is a hash over that path, and the vite transform reads
//      the config file unconditionally. So the half that omits it extracts,
//      translates and compiles keys the running app never asks for, and the
//      strings fall back to source with nothing failing (#1806). The same check
//      requires every `--config` path to exist, and requires a surface that
//      carries its own config file to pass it.
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
import { existsSync, readFileSync } from "node:fs";
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

// The extract flags that consume the next token. Anything else is a switch.
const VALUED = new Set([
  "--config",
  "--out",
  "--source-root",
  "--target-locale",
  "--src",
  "--ignore",
]);

// splitFlags tokenizes a make-expanded flag string, honouring the double
// quotes that keep a glob away from the shell.
function splitFlags(line) {
  const out = [];
  const re = /"([^"]*)"|(\S+)/g;
  let m;
  while ((m = re.exec(line)) !== null) out.push(m[1] ?? m[2]);
  return out;
}

// normalize renders a flag list as a sorted "flag=value" multiset, so the two
// definitions of a surface may order their flags differently and still agree.
function normalize(tokens) {
  const out = [];
  for (let i = 0; i < tokens.length; i++) {
    out.push(VALUED.has(tokens[i]) ? `${tokens[i]}=${tokens[++i]}` : tokens[i]);
  }
  return out.sort();
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
      const config = [];
      for (let i = 0; i < tokens.length; i++) {
        if (tokens[i] === "--src") src.push(tokens[++i]);
        else if (tokens[i] === "--ignore") exclude.push(tokens[++i]);
        else if (tokens[i] === "--config") config.push(tokens[++i]);
      }
      return { dir, src, exclude, config, tokens };
    });
}

// packageFlags returns the flag tokens a surface's own `extract` script passes,
// or null when the package defines no such script (the Makefile is then its
// only definition, and there is nothing to disagree with).
function packageFlags(dir) {
  const manifest = resolve(root, dir, "package.json");
  if (!existsSync(manifest)) return null;
  const script = JSON.parse(readFileSync(manifest, "utf8")).scripts?.extract;
  if (script === undefined) return null;
  const at = script.indexOf(" extract ");
  if (at === -1) {
    console.error(`${dir}: package.json "extract" script does not invoke neokapi-i18n extract`);
    return null;
  }
  return splitFlags(script.slice(at + " extract ".length));
}

let failed = false;

for (const { dir, src, exclude, config, tokens } of surfaces()) {
  for (const path of config) {
    if (!existsSync(resolve(root, dir, path))) {
      failed = true;
      console.error(`${dir}: --config ${path} does not exist`);
    }
  }
  if (config.length === 0 && existsSync(resolve(root, dir, "neokapi-i18n.config.json"))) {
    failed = true;
    console.error(
      `${dir}: carries a neokapi-i18n.config.json the extract flags never pass. ` +
        `The vite transform reads it unconditionally, so an extract without ` +
        `--config hashes those strings differently and the app cannot resolve them.`,
    );
  }

  const pkg = packageFlags(dir);
  if (pkg !== null) {
    const fromMake = normalize(tokens);
    const fromPackage = normalize(pkg);
    if (fromMake.join("\n") !== fromPackage.join("\n")) {
      failed = true;
      const only = (a, b) => a.filter((f) => !b.includes(f));
      console.error(`${dir}: the make target and the package \`extract\` script disagree:`);
      for (const f of only(fromMake, fromPackage)) console.error(`    make only:    ${f}`);
      for (const f of only(fromPackage, fromMake)) console.error(`    package only: ${f}`);
    }
  }

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
  console.error("\ncheck-extract-fixtures: an extract surface is not what it claims to be");
  process.exit(1);
}

console.log(
  "✓ no test or story file is extracted into a shipped catalog, " +
    "and every surface is invoked the same way from make and from its package",
);
