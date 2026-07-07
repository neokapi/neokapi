#!/usr/bin/env node
// ---------------------------------------------------------------------------
// Storybook coverage report (non-blocking).
//
// Scans the frontend app/package trees for component `.tsx` files above a line
// threshold (default 200) that have no matching `*.stories.tsx`. Prints a
// per-tree report and a summary. Exits 0 by default so it can run as an
// informational CI step; pass `--strict` to exit non-zero when any tracked
// tree has uncovered components (for flipping the gate on once a backlog
// clears).
//
// Usage:
//   node scripts/story-coverage.mjs [--min-lines N] [--strict] [--json]
//
// "Covered" = a `<Component>.stories.tsx` exists anywhere in the same tree, OR
// some `*.stories.tsx` in the tree imports the component by basename. This is a
// heuristic, deliberately generous, matching the two conventions in this repo
// (stories colocated with components, and stories under a `src/stories/` dir).
// ---------------------------------------------------------------------------

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, basename, relative } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("..", import.meta.url));

// Trees scanned, with the src subdir that holds their components.
const TREES = [
  "packages/ui/src",
  "packages/flow-editor/src",
  "packages/docs-shared/src",
  "apps/kapi-desktop/frontend/src",
  "bowrain/packages/ui/src",
  "bowrain/apps/web/src",
  "bowrain/apps/bowrain/frontend/src",
  "bowrain/apps/ctrl/src",
  "bowrain/apps/pulse/src",
];

// Files that are not "components" for coverage purposes.
const SKIP_BASENAMES = new Set([
  "main.tsx",
  "App.tsx",
  "vite-env.d.ts",
  "index.tsx",
]);

function parseArgs(argv) {
  const opts = { minLines: 200, strict: false, json: false };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--strict") opts.strict = true;
    else if (a === "--json") opts.json = true;
    else if (a === "--min-lines") opts.minLines = Number(argv[++i]);
    else if (a.startsWith("--min-lines=")) opts.minLines = Number(a.split("=")[1]);
  }
  return opts;
}

function walk(dir, acc = []) {
  let entries;
  try {
    entries = readdirSync(dir, { withFileTypes: true });
  } catch {
    return acc;
  }
  for (const e of entries) {
    if (e.name === "node_modules" || e.name === "dist" || e.name.startsWith(".")) continue;
    const full = join(dir, e.name);
    if (e.isDirectory()) walk(full, acc);
    else if (e.isFile() && e.name.endsWith(".tsx")) acc.push(full);
  }
  return acc;
}

function isStory(file) {
  return file.endsWith(".stories.tsx");
}

function isTestOrSkipped(file) {
  const b = basename(file);
  return (
    b.endsWith(".test.tsx") ||
    b.endsWith(".spec.tsx") ||
    b.endsWith(".d.tsx") ||
    SKIP_BASENAMES.has(b)
  );
}

function lineCount(file) {
  const text = readFileSync(file, "utf8");
  // Count non-empty logical lines to be a touch more meaningful than raw \n.
  return text.split("\n").length;
}

function analyzeTree(treeSrc, minLines) {
  const abs = join(repoRoot, treeSrc);
  let files;
  try {
    statSync(abs);
    files = walk(abs);
  } catch {
    return null; // tree absent
  }

  const stories = files.filter(isStory);
  const storyBasenames = new Set(
    stories.map((f) => basename(f).replace(/\.stories\.tsx$/, "")),
  );
  const storyText = stories.map((f) => readFileSync(f, "utf8")).join("\n");

  const components = files.filter((f) => !isStory(f) && !isTestOrSkipped(f));

  const uncovered = [];
  let coveredCount = 0;
  let componentCount = 0;

  for (const f of components) {
    const lc = lineCount(f);
    const base = basename(f).replace(/\.tsx$/, "");
    componentCount++;
    const covered =
      storyBasenames.has(base) ||
      new RegExp(`/${base}["']`).test(storyText) ||
      new RegExp(`\\b${base}\\.stories\\b`).test(storyText);
    if (covered) coveredCount++;
    else if (lc >= minLines) {
      uncovered.push({ file: relative(repoRoot, f), lines: lc });
    }
  }

  uncovered.sort((a, b) => b.lines - a.lines);
  return {
    tree: treeSrc,
    componentCount,
    coveredCount,
    storyCount: stories.length,
    uncovered,
  };
}

function main() {
  const opts = parseArgs(process.argv);
  const results = TREES.map((t) => analyzeTree(t, opts.minLines)).filter(Boolean);

  if (opts.json) {
    process.stdout.write(JSON.stringify({ minLines: opts.minLines, results }, null, 2) + "\n");
  } else {
    console.log(`\nStorybook coverage report (components >= ${opts.minLines} lines without a story)\n`);
    for (const r of results) {
      const pct =
        r.componentCount === 0 ? 100 : Math.round((r.coveredCount / r.componentCount) * 100);
      console.log(
        `  ${r.tree}` +
          `  —  ${r.coveredCount}/${r.componentCount} components storied (${pct}%), ${r.storyCount} story files`,
      );
      for (const u of r.uncovered) {
        console.log(`      • ${u.file}  (${u.lines} lines)`);
      }
    }
    const totalBig = results.reduce((n, r) => n + r.uncovered.length, 0);
    console.log(
      `\n  ${totalBig} component(s) >= ${opts.minLines} lines still lack a story.` +
        (totalBig === 0 ? "  ✓" : ""),
    );
    console.log(
      "  (informational — this report does not fail the build unless run with --strict)\n",
    );
  }

  const totalBig = results.reduce((n, r) => n + r.uncovered.length, 0);
  process.exit(opts.strict && totalBig > 0 ? 1 : 0);
}

main();
