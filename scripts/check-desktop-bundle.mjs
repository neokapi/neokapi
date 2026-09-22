#!/usr/bin/env node
// Guard: a desktop app's production frontend bundle loads its Wails bindings.
//
// The desktop reaches its Go backend through generated bindings that the
// frontend imports lazily and treats a failed import as "not running under
// Wails": every backend call then returns null and the app renders with empty
// lists and no error. Nothing in a typecheck or a jsdom test sees that; only
// the chunks the production build emits do. Two things are checked here:
//
//   1. No cycle among the chunks' static imports. A cycle evaluates one chunk
//      before a binding it reads at load time has been assigned.
//   2. The chunk that holds the bindings evaluates outside a Wails window and
//      exposes the backend method named on the command line.
//
// Usage: check-desktop-bundle.mjs <dist dir> <backend method name>

import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const [distDir, method] = process.argv.slice(2);
if (!distDir || !method) {
  console.error("usage: check-desktop-bundle.mjs <dist dir> <backend method name>");
  process.exit(2);
}

const assets = resolve(distDir, "assets");
const chunks = readdirSync(assets).filter((name) => name.endsWith(".js"));
if (chunks.length === 0) {
  console.error(`check-desktop-bundle: no chunks under ${assets}; build the frontend first`);
  process.exit(1);
}

// Static imports only: `import ... from "./x.js"`. A dynamic import evaluates
// its target when it runs, after the importer has finished, so it cannot bind
// a chunk to an uninitialised import the way a static one does.
const staticImport = /from\s*["'`]\.\/([A-Za-z0-9_.-]+\.js)["'`]/g;
const graph = new Map();
for (const chunk of chunks) {
  const source = readFileSync(join(assets, chunk), "utf8");
  const targets = new Set();
  for (const match of source.matchAll(staticImport)) targets.add(match[1]);
  graph.set(chunk, targets);
}

function findCycle() {
  const state = new Map();
  const stack = [];
  const visit = (node) => {
    state.set(node, "active");
    stack.push(node);
    for (const next of graph.get(node) ?? []) {
      if (!graph.has(next)) continue;
      const s = state.get(next);
      if (s === "active") return [...stack.slice(stack.indexOf(next)), next];
      if (!s) {
        const cycle = visit(next);
        if (cycle) return cycle;
      }
    }
    stack.pop();
    state.set(node, "done");
    return null;
  };
  for (const node of graph.keys()) {
    if (!state.has(node)) {
      const cycle = visit(node);
      if (cycle) return cycle;
    }
  }
  return null;
}

const cycle = findCycle();
if (cycle) {
  console.error(`check-desktop-bundle: chunks import each other: ${cycle.join(" -> ")}`);
  process.exit(1);
}

// The chunk that defines the binding exports it under its own name (`xm as
// ListWorkspaceProjects`); a chunk that only calls it holds the name as a
// string. The former is the one to load.
const defines = new RegExp(`\\bas ${method}\\b|\\b${method}\\s*:`);
const holder = chunks.find((chunk) => defines.test(readFileSync(join(assets, chunk), "utf8")));
if (!holder) {
  console.error(`check-desktop-bundle: no chunk defines ${method}; the bindings are missing from the bundle`);
  process.exit(1);
}

// The entry chunk renders the app and needs a document, so it cannot be
// evaluated here. When the bindings sit in it, a failed load takes the whole
// page down rather than returning null from every call, which the cycle check
// above already covers; only a lazily imported bindings chunk fails silently.
const html = readFileSync(join(resolve(distDir), "index.html"), "utf8");
const entries = [...html.matchAll(/<script[^>]+src="[^"]*\/assets\/([A-Za-z0-9_.-]+\.js)"/g)].map((m) => m[1]);
if (entries.includes(holder)) {
  console.log(`check-desktop-bundle: ${chunks.length} chunks, no import cycle; ${method}() is in the entry chunk ${holder}`);
  process.exit(0);
}

// The runtime prints a banner when it finds no Wails window; that is expected here.
const quiet = console.warn;
console.warn = () => {};
let mod;
try {
  mod = await import(pathToFileURL(join(assets, holder)).href);
} catch (err) {
  console.warn = quiet;
  console.error(`check-desktop-bundle: ${holder} failed to load: ${err && err.message ? err.message : err}`);
  process.exit(1);
}
console.warn = quiet;

// The bindings namespace is one of the chunk's exports, under a minified name.
const found = Object.values(mod).some(
  (value) => value && typeof value === "object" && typeof value[method] === "function",
);
if (!found) {
  console.error(`check-desktop-bundle: ${holder} loaded but exposes no namespace with ${method}()`);
  process.exit(1);
}
console.log(`check-desktop-bundle: ${chunks.length} chunks, no import cycle; ${holder} exposes ${method}()`);
