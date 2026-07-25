// Semantic gate over the committed Wails bindings of both desktop apps.
//
// The generated bindings are committed (see .gitignore's `!.../bindings/`
// re-includes) *and* regenerated during release.yml's build. Two copies of a
// generated artifact drift, and the frontend's dispatch layer cannot detect it:
//
//   type Backend = Record<string, (...args: unknown[]) => Promise<unknown>>;
//   if (typeof b[method] !== "function") return null;   // silent null
//
// An open index signature means `tsc` never sees a wrong method name, and the
// runtime turns one into a silent `null` rather than an error. So nothing in the
// normal build catches a bindings/frontend name mismatch. This script does.
//
// Three independent assertions, all derived from the generated bindings — no
// Go toolchain required, so this runs in the cheap node-only CI job:
//
//   1. WRAPPER/ID PAIRING. Wails derives a bound method's numeric id as
//      fnv32a(pkgPath + "." + TypeName + "." + MethodName)
//      (internal/generator/collect/service.go: `hash.Fnv(fqn)`, internal/hash/fnv.go).
//      Since the JS wrapper is emitted under the Go method's own name and Wails
//      has no name-alias directive, recomputing the hash from the *wrapper name*
//      must reproduce the id the wrapper passes to $Call.ByID. If a rename ever
//      touches the JS name without moving the id (the #1462 hazard), this fails.
//
//   2. ID-MAP FRESHNESS. src/demo/wails-id-map.generated.json is a pure
//      projection of app.js used by the wbridge recorder to map ids back to
//      names. Assert it matches rather than trusting someone ran the generator.
//
//   3. CALL-NAME REACHABILITY. Every method name the frontend passes to
//      `call("Name", …)` must be exported by the bindings. This is what actually
//      broke: after #1462 renamed 36 TM/termbase methods to memory/terms, the
//      frontend called the new names while the committed bindings still exported
//      the old ones — every one of those calls silently returned null.
//
//      This assertion is kapi-desktop-specific by construction: bowrain-desktop
//      re-exports the bindings statically (`export * as Backend from ".../app.js"`
//      in src/api/backend.ts), so `tsc` already rejects a name that no longer
//      exists there. A reported count of 0 called names for bowrain is therefore
//      expected, not a silently vacuous check. It is exactly that static typing
//      which shaped bowrain's failure: #1462 renamed the wrappers to satisfy the
//      typecheck and left the ids behind, so the names compiled while 7 calls
//      dispatched to hashes of the old names — caught here by assertion 1.
//
// Run: node scripts/check-wails-bindings.mjs
import { readFileSync, existsSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

/** Wails' method-id hash: fnv32a over "<pkgPath>.<TypeName>.<MethodName>". */
function fnv1a32(s) {
  let h = 0x811c9dc5;
  const bytes = Buffer.from(s, "utf8");
  for (const b of bytes) {
    h ^= b;
    // FNV prime 16777619, kept in uint32 via Math.imul.
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

const APPS = [
  {
    label: "kapi-desktop",
    frontend: "apps/kapi-desktop/frontend",
    appJS:
      "apps/kapi-desktop/frontend/bindings/github.com/neokapi/neokapi/kapi-desktop/backend/app.js",
    // Go package path + bound service type, the two inputs to the id hash.
    pkgPath: "github.com/neokapi/neokapi/kapi-desktop/backend",
    typeName: "App",
    idMap: "apps/kapi-desktop/frontend/src/demo/wails-id-map.generated.json",
  },
  {
    label: "bowrain-desktop",
    frontend: "bowrain/apps/bowrain/frontend",
    appJS:
      "bowrain/apps/bowrain/frontend/bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js",
    pkgPath: "github.com/neokapi/neokapi/bowrain/apps/bowrain/backend",
    typeName: "App",
    idMap: "bowrain/apps/bowrain/frontend/src/demo/wails-id-map.generated.json",
  },
];

/** Extract wrapper name -> $Call.ByID id from a generated app.js. */
function callSurface(file) {
  const js = readFileSync(file, "utf8");
  const re = /export function (\w+)\([^)]*\)\s*\{\s*return \$Call\.ByID\((\d+)/g;
  const surface = new Map();
  for (let m; (m = re.exec(js)); ) surface.set(m[1], Number(m[2]));
  return surface;
}

/** Every distinct name passed to call("Name", …) under a frontend's src/. */
function calledNames(frontendDir) {
  const srcDir = path.join(ROOT, frontendDir, "src");
  const names = new Map(); // name -> first file that calls it
  if (!existsSync(srcDir)) return names;
  const walk = (dir) => {
    for (const entry of readdirSync(dir)) {
      const p = path.join(dir, entry);
      if (statSync(p).isDirectory()) {
        if (entry === "__tests__" || entry === "__mocks__" || entry === "stories") continue;
        walk(p);
        continue;
      }
      if (!/\.(ts|tsx)$/.test(entry) || /\.d\.ts$/.test(entry)) continue;
      // Tests and stories deliberately dispatch on names that do not exist
      // (e.g. useApi.test.ts asserts call("NonexistentMethod") resolves null).
      if (/\.(test|spec)\.tsx?$/.test(entry) || /\.stories\.tsx?$/.test(entry)) continue;
      const src = readFileSync(p, "utf8");
      // call<T>("Name", …) / call("Name", …) — the single dispatch helper.
      const re = /\bcall\s*(?:<[^>]*>)?\s*\(\s*"(\w+)"/g;
      for (let m; (m = re.exec(src)); ) {
        if (!names.has(m[1])) names.set(m[1], path.relative(ROOT, p));
      }
    }
  };
  walk(srcDir);
  return names;
}

const failures = [];
let checked = 0;

for (const app of APPS) {
  const appJS = path.join(ROOT, app.appJS);
  if (!existsSync(appJS)) {
    failures.push(`${app.label}: missing generated bindings at ${app.appJS}`);
    continue;
  }

  const surface = callSurface(appJS);
  if (surface.size === 0) {
    failures.push(`${app.label}: no $Call.ByID wrappers found in ${app.appJS}`);
    continue;
  }

  // 1. Wrapper name and id must be a consistent pair.
  let mismatched = 0;
  for (const [name, id] of surface) {
    const fqn = `${app.pkgPath}.${app.typeName}.${name}`;
    const want = fnv1a32(fqn);
    if (want !== id) {
      mismatched++;
      failures.push(
        `${app.label}: ${name} calls $Call.ByID(${id}) but fnv32a("${fqn}") = ${want}` +
          ` — the JS wrapper name and the Go method id disagree, so this call goes nowhere at runtime`,
      );
    }
  }

  // 2. The committed id map must be app.js's own projection.
  const idMapPath = path.join(ROOT, app.idMap);
  if (!existsSync(idMapPath)) {
    failures.push(`${app.label}: missing id map ${app.idMap}`);
  } else {
    const committed = JSON.parse(readFileSync(idMapPath, "utf8"));
    // Same shape gen-wails-id-map.mjs writes: id -> name, ascending by id.
    const expected = {};
    for (const [name, id] of [...surface].sort((x, y) => x[1] - y[1])) {
      expected[String(id)] = name;
    }
    if (JSON.stringify(committed) !== JSON.stringify(expected)) {
      const missing = Object.keys(expected).filter((k) => committed[k] !== expected[k]);
      failures.push(
        `${app.label}: ${app.idMap} is stale vs app.js` +
          ` (${missing.length} id(s) wrong/absent, e.g. ${missing.slice(0, 5).join(", ")})` +
          ` — regenerate with: cd ${app.frontend} && node scripts/gen-wails-id-map.mjs`,
      );
    }
  }

  // 3. Every name the frontend dispatches on must exist in the bindings.
  const called = calledNames(app.frontend);
  const unreachable = [...called].filter(([name]) => !surface.has(name));
  if (unreachable.length > 0) {
    failures.push(
      `${app.label}: ${unreachable.length} call() name(s) are not exported by the bindings` +
        ` — each silently returns null at runtime:\n` +
        unreachable.map(([n, f]) => `      ${n}  (${f})`).join("\n"),
    );
  }

  checked++;
  console.log(
    `${app.label}: ${surface.size} bound methods, ` +
      `${surface.size - mismatched} id(s) verified against fnv32a(FQN), ` +
      `${called.size} call() name(s) resolved`,
  );
}

if (failures.length > 0) {
  console.error(`\ncheck-wails-bindings: ${failures.length} problem(s)\n`);
  for (const f of failures) console.error(`  ✗ ${f}`);
  console.error(
    `\nIf the Go backend changed, regenerate the committed bindings:\n` +
      `  make kapi-desktop-bindings\n`,
  );
  process.exit(1);
}

console.log(`✓ wails bindings consistent across ${checked} app(s)`);
