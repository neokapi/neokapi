/**
 * Audit captured sessions for kapi/tool errors.
 *
 *   npm run audit            # report; exits non-zero if any demo has errors
 *
 * Reads public/<id>/capture.json. Uses the recorded meta.errors when present,
 * otherwise recomputes from the timeline (so older captures are covered too).
 */
import fs from "node:fs";
import path from "node:path";
import { PUBLIC_DIR } from "../lib/paths.ts";
import { detectErrors } from "../driver/normalize.ts";
import type { CaptureError, DemoCapture } from "../types.ts";

function main() {
  if (!fs.existsSync(PUBLIC_DIR)) {
    console.log("no public/ captures to audit");
    return;
  }
  const ids = fs
    .readdirSync(PUBLIC_DIR, { withFileTypes: true })
    .filter((d) => d.isDirectory() && fs.existsSync(path.join(PUBLIC_DIR, d.name, "capture.json")))
    .map((d) => d.name)
    .sort();

  let totalErrors = 0;
  let dirty = 0;
  console.log(`Auditing ${ids.length} capture(s) for kapi/tool errors:\n`);
  for (const id of ids) {
    const cap: DemoCapture = JSON.parse(fs.readFileSync(path.join(PUBLIC_DIR, id, "capture.json"), "utf8"));
    const errs: CaptureError[] = cap.meta?.errors ?? detectErrors(cap.events);
    if (errs.length === 0) {
      console.log(`  ✓ ${id}`);
      continue;
    }
    dirty++;
    totalErrors += errs.length;
    console.log(`  ✗ ${id} — ${errs.length} error(s):`);
    for (const e of errs) {
      console.log(`      [${e.hardError ? "error" : "pattern"}] ${e.command.slice(0, 100)}`);
      console.log(`         ↳ ${e.snippet}`);
    }
  }
  console.log(`\n${dirty ? "✗" : "✓"} ${ids.length - dirty}/${ids.length} clean; ${totalErrors} error(s) total`);
  if (dirty) process.exitCode = 1;
}

main();
