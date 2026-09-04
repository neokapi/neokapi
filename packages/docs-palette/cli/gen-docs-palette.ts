/**
 * Write the documentation palette bridges from the canonical brand tokens.
 *
 * Run `make generate-docs-palette` to refresh, `make check-docs-palette` to gate
 * on drift. See packages/docs-palette/src/derive.ts for what is computed.
 */

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { check, generate } from "../src/generate.ts";
import { MAKE_TARGET } from "../src/targets.ts";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(HERE, "..", "..", "..");

if (process.argv.includes("-check")) {
  const stale = check(REPO_ROOT).filter((r) => !r.current);
  if (stale.length > 0) {
    console.error("docs-palette: these bridges are stale relative to their brand tokens:");
    for (const result of stale) console.error(`  ${result.out}`);
    console.error(`Run 'make ${MAKE_TARGET}' and commit the result.`);
    process.exit(1);
  }
  console.log("docs-palette: every bridge matches its brand tokens");
} else {
  for (const result of generate(REPO_ROOT)) {
    console.log(`docs-palette: ${result.current ? "unchanged" : "wrote"} ${result.out}`);
  }
}
