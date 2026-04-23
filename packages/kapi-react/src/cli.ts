#!/usr/bin/env node

/**
 * kapi-react CLI.
 *
 *   kapi-react extract   Walk JSX/TSX source and produce one .klf per
 *                        source file under --out (default: i18n/).
 *
 *   kapi-react compile   Consume a translated .klf directory (kapi or
 *                        another tool filled in block.targets[locale]),
 *                        flatten each block's target runs into the
 *                        {hash: text} shape the runtime loader reads
 *                        via fetch() + setTranslations().
 *
 *   kapi-react split     Slice per-locale master dicts into per-chunk
 *                        subsets paired with a translations-manifest.json
 *                        (produced by the Vite/Rollup plugin). Feeds
 *                        lazy loading alongside code-split JS (#406).
 *
 * The boundary: kapi-react extracts and compiles; everything in between
 * (pseudo-translate, AI translate, TM, QA, …) goes through `kapi`.
 */

import { runExtract } from "./commands/extract.ts";
import { runCompile } from "./commands/compile.ts";
import { runSplit } from "./commands/split.ts";

const [, , command, ...rest] = process.argv;

async function main() {
  switch (command) {
    case "extract":
      await runExtract(rest);
      return;
    case "compile":
      await runCompile(rest);
      return;
    case "split":
      await runSplit(rest);
      return;
    case undefined:
    case "--help":
    case "-h":
      usage();
      process.exit(command ? 0 : 1);
      return;
    default:
      console.error(`unknown command: ${command}\n`);
      usage();
      process.exit(1);
  }
}

function usage() {
  console.log(`
kapi-react — zero-config i18n for React

Commands:
  extract    Extract translatable strings from JSX/TSX source files
  compile    Flatten a translated .klf directory into runtime dictionaries
  split      Slice per-locale dicts into per-chunk subsets for lazy loading

Run \`kapi-react <command> --help\` for per-command options.
`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
