#!/usr/bin/env node
//
// Guard: no extractor warning beyond the committed baseline.
//
// The React extractor warns about text it extracts without a stable key or
// cannot extract at all: an unmapped component, a label spliced into JSX text,
// a conditional attribute holding a string it cannot read. The warnings already
// in the tree are recorded in scripts/extract-warnings-baseline.json. A warning
// past that record fails, and the recorded ones do not, so existing debt never
// blocks a pull request.
//
// Each extract target writes its warnings with `--warnings-json` to one file
// per surface under .extract-warnings/. A warning is counted by surface, file,
// kind and tag, never by line, so an edit above a recorded warning does not
// read as a new one. A surface with fewer warnings than its record passes, and
// the report says the baseline can be lowered.
//
// Usage:
//     node scripts/check-extract-warnings.mjs [--dir DIR] [--baseline FILE]
//     node scripts/check-extract-warnings.mjs --update     # rewrite the baseline
//     node scripts/check-extract-warnings.mjs --self-test
//
// Run it through `make l10n-extract-warnings-check` after `make l10n-extract`,
// and `make l10n-extract-warnings-baseline` to rewrite the record.

import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");
const DEFAULT_DIR = join(ROOT, ".extract-warnings");
const DEFAULT_BASELINE = join(ROOT, "scripts", "extract-warnings-baseline.json");

const byCodeUnit = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

/** The count key of one warning. The line is left out on purpose. */
function keyOf(w) {
  return `${w.file} | ${w.kind} | ${w.tag}`;
}

/** Every surface's warnings in `dir`, keyed by the file's basename. */
function readWarnings(dir) {
  if (!existsSync(dir)) {
    throw new Error(`${dir} does not exist: run \`make l10n-extract\` first`);
  }
  const surfaces = new Map();
  for (const name of readdirSync(dir).sort(byCodeUnit)) {
    if (!name.endsWith(".json")) continue;
    const parsed = JSON.parse(readFileSync(join(dir, name), "utf8"));
    if (!Array.isArray(parsed.warnings)) {
      throw new Error(`${join(dir, name)} holds no "warnings" array`);
    }
    surfaces.set(basename(name, ".json"), parsed.warnings);
  }
  return surfaces;
}

/** Counts per surface and key, both in code-unit order. */
function tally(surfaces) {
  const out = {};
  for (const surface of [...surfaces.keys()].sort(byCodeUnit)) {
    const counts = {};
    for (const w of surfaces.get(surface)) counts[keyOf(w)] = (counts[keyOf(w)] ?? 0) + 1;
    out[surface] = Object.fromEntries(
      Object.entries(counts).sort(([a], [b]) => byCodeUnit(a, b)),
    );
  }
  return out;
}

function total(counts) {
  return Object.values(counts).reduce(
    (n, c) => n + Object.values(c).reduce((a, b) => a + b, 0),
    0,
  );
}

/**
 * What the current warnings add to the baseline, and how many it has shed. A
 * surface the baseline records and the run did not write is missing: the
 * extraction did not run for it, which is an error rather than a pass.
 */
function compare(baseline, current) {
  const added = [];
  const missing = Object.keys(baseline).filter((s) => !(s in current));
  let lowered = 0;
  for (const [surface, counts] of Object.entries(current)) {
    const recorded = baseline[surface] ?? {};
    for (const [key, now] of Object.entries(counts)) {
      const was = recorded[key] ?? 0;
      if (now > was) added.push({ surface, key, was, now });
    }
    for (const [key, was] of Object.entries(recorded)) {
      const now = counts[key] ?? 0;
      if (now < was) lowered += was - now;
    }
  }
  return { added, missing, lowered };
}

function readBaseline(path) {
  if (!existsSync(path)) return {};
  return JSON.parse(readFileSync(path, "utf8"));
}

function writeBaseline(path, counts) {
  writeFileSync(path, JSON.stringify(counts, null, 2) + "\n");
}

/** Prints the report and returns the exit code: 0 pass, 1 new warnings, 2 missing input. */
function check(dir, baselinePath, log = console) {
  const surfaces = readWarnings(dir);
  const current = tally(surfaces);
  const { added, missing, lowered } = compare(readBaseline(baselinePath), current);

  if (missing.length > 0) {
    log.error(
      `check-extract-warnings: no warnings file for ${missing.join(", ")} in ${dir}: run \`make l10n-extract\` first`,
    );
    return 2;
  }
  if (added.length > 0) {
    const count = added.reduce((n, a) => n + a.now - a.was, 0);
    log.error(`check-extract-warnings: ${count} extractor warning(s) beyond the baseline:`);
    for (const { surface, key, was, now } of added) {
      const [file, kind, tag] = key.split(" | ");
      const lines = surfaces
        .get(surface)
        .filter((w) => w.file === file && w.kind === kind && w.tag === tag)
        .map((w) => w.line);
      log.error(
        `  ${surface}: ${file}:${lines.join(",")} ${kind} ${tag} (recorded ${was}, now ${now})`,
      );
    }
    log.error(
      "Fix each warning where it is raised, as the extractor's message describes. A warning that is right as it stands goes into the baseline with `make l10n-extract-warnings-baseline`.",
    );
    return 1;
  }
  log.log(`check-extract-warnings: ${total(current)} warning(s), none beyond the baseline`);
  if (lowered > 0) {
    log.log(
      `check-extract-warnings: ${lowered} fewer than the baseline records; lower it with \`make l10n-extract-warnings-baseline\``,
    );
  }
  return 0;
}

// ── self-test ────────────────────────────────────────────────────────────────

function selfTest() {
  let status = 0;
  const expect = (label, got, want) => {
    const ok = got === want;
    console.log(`${ok ? "✓" : "✖"} self-test: ${label}`);
    if (!ok) {
      console.log(`    expected ${want}, got ${got}`);
      status = 1;
    }
  };

  const root = mkdtempSync(join(tmpdir(), "extract-warnings-"));
  const dir = join(root, "warnings");
  const baseline = join(root, "baseline.json");
  const quiet = { log() {}, error() {} };
  const run = () => {
    try {
      return check(dir, baseline, quiet);
    } catch {
      return 2;
    }
  };
  const plant = (surface, list) => {
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, `${surface}.json`), JSON.stringify({ warnings: list }));
  };
  const tab = (line) => ({ kind: "unknown-component", file: "src/a.tsx", line, tag: "TabsTrigger" });
  const splice = { kind: "dyn-label-splice", file: "src/b.tsx", line: 9, tag: "item.label" };

  try {
    expect("a run that wrote no warnings directory is an error", run(), 2);

    plant("app", [tab(10), tab(20), splice]);
    writeBaseline(baseline, tally(readWarnings(dir)));
    expect("the warnings the baseline records pass", run(), 0);

    plant("app", [tab(14), tab(31), splice]);
    expect("a recorded warning that moved to another line passes", run(), 0);

    plant("app", [tab(10), tab(20), tab(30), splice]);
    expect("one more of a recorded warning fails", run(), 1);

    plant("app", [tab(10), tab(20), splice, { ...splice, tag: "row.title" }]);
    expect("a warning the baseline does not record fails", run(), 1);

    plant("app", [tab(10)]);
    expect("fewer warnings than the baseline records pass", run(), 0);

    plant("app", [tab(10), tab(20), splice]);
    plant("ctrl", [tab(3)]);
    expect("a surface the baseline does not name is held to an empty record", run(), 1);

    rmSync(join(dir, "ctrl.json"));
    rmSync(join(dir, "app.json"));
    plant("pulse", []);
    expect("a surface the baseline names and the run did not write is an error", run(), 2);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }

  expect(
    "the committed baseline parses and names at least one surface",
    Object.keys(readBaseline(DEFAULT_BASELINE)).length > 0,
    true,
  );
  return status;
}

// ── entry point ──────────────────────────────────────────────────────────────

function main(argv) {
  if (argv.includes("--self-test")) return selfTest();
  const valueOf = (flag, fallback) => {
    const i = argv.indexOf(flag);
    return i === -1 ? fallback : argv[i + 1];
  };
  const dir = valueOf("--dir", DEFAULT_DIR);
  const baseline = valueOf("--baseline", DEFAULT_BASELINE);
  try {
    if (argv.includes("--update")) {
      const counts = tally(readWarnings(dir));
      writeBaseline(baseline, counts);
      console.log(`check-extract-warnings: wrote ${total(counts)} warning(s) to ${baseline}`);
      return 0;
    }
    return check(dir, baseline);
  } catch (err) {
    console.error(`check-extract-warnings: ${err.message ?? err}`);
    return 2;
  }
}

process.exit(main(process.argv.slice(2)));
