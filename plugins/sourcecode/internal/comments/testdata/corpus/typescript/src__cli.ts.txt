#!/usr/bin/env node

/**
 * neokapi-i18n CLI.
 *
 *   neokapi-i18n extract   Walk JSX/TSX source and produce one .kbf.json per
 *                        source file under --out (default: i18n/),
 *                        mirroring the source tree — so `src/**` lands
 *                        catalogs under i18n/src/, leaving i18n/{lang}/
 *                        for kapi's per-locale targets.
 *
 *   neokapi-i18n compile   Consume a translated .kbf.json directory (kapi or
 *                        another tool filled in block.targets[locale]),
 *                        flatten each block's target runs into the
 *                        {hash: text} shape the runtime loader reads
 *                        via fetch() + setTranslations().
 *
 *   neokapi-i18n split     Slice per-locale master dicts into per-chunk
 *                        subsets paired with a translations-manifest.json
 *                        (produced by the Vite/Rollup plugin). Feeds
 *                        lazy loading alongside code-split JS (#406).
 *
 * The boundary: neokapi-i18n extracts and compiles; everything in between
 * (pseudo-translate, AI translate, content memory, checks, …) goes through `kapi`.
 */

import { runExtract } from "./commands/extract.ts";
import { runCompile } from "./commands/compile.ts";
import { runSplit } from "./commands/split.ts";
import { runExplain } from "./commands/explain.ts";

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
    case "explain":
      await runExplain(rest);
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
neokapi-i18n — zero-config i18n for React

Commands:
  extract       Extract translatable strings from JSX/TSX source files
  compile       Flatten a translated .kbf.json directory into runtime dictionaries
  split         Slice per-locale dicts into per-chunk subsets for lazy loading
  explain       Print each element's translatability decision (ITS audit)

Run \`neokapi-i18n <command> --help\` for per-command options.
`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
