// Regenerate src/demo/wails-id-map.generated.json from the Wails-generated
// backend bindings.
//
// The wbridge recording entry (src/demo/real-main.tsx) drives the REAL desktop
// app in a browser by forwarding each Wails binding call to the wbridge HTTP
// server BY METHOD NAME. A Wails binding call carries only a numeric method id
// ($Call.ByID(<id>, …)), so real-main.tsx needs an id→name map to translate it.
// That map is this file — a pure projection of the generated bindings.
//
// It must be regenerated whenever the backend's bound method set changes (new
// App methods, e.g. the composite adapter's ProxyRequest/ProxyMultipart), i.e.
// after `wails3 generate bindings`. A stale map makes real-main.tsx throw
// "wbridge: unknown method id <id>" on the first unmapped call, blanking the
// recorded/parity page.
//
//   node scripts/gen-wails-id-map.mjs
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const FRONTEND = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const APP_JS = path.join(
  FRONTEND,
  "bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js",
);
const OUT = path.join(FRONTEND, "src/demo/wails-id-map.generated.json");

const js = readFileSync(APP_JS, "utf8");
// Each bound method is `export function Name(args) { return $Call.ByID(<id>, …`.
const re = /export function (\w+)\([^)]*\)\s*\{\s*return \$Call\.ByID\((\d+)/g;
const byId = {};
for (let m; (m = re.exec(js)); ) byId[m[2]] = m[1];

const ids = Object.keys(byId)
  .map(Number)
  .sort((a, b) => a - b);
if (ids.length === 0) throw new Error(`no $Call.ByID bindings found in ${APP_JS}`);
const sorted = {};
for (const id of ids) sorted[String(id)] = byId[String(id)];

writeFileSync(OUT, JSON.stringify(sorted, null, 2) + "\n");
console.log(`wrote ${ids.length} id→name entries → ${path.relative(FRONTEND, OUT)}`);
