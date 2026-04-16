#!/usr/bin/env node

/**
 * kapi-react CLI.
 *
 *   kapi-react extract   Walk JSX/TSX source and produce i18n/strings.json
 *                        (also .klz once the AST walker learns to emit
 *                        structured Runs; tracked separately).
 *
 *   kapi-react compile   Consume a translated .klz (kapi or another tool
 *                        filled in block.targets[locale]), flatten each
 *                        block's target runs into the {hash: text} shape
 *                        the runtime loader reads via fetch() + setTranslations().
 *
 * The boundary: kapi-react extracts and compiles; everything in between
 * (pseudo-translate, AI translate, TM, QA, …) goes through `kapi`.
 */

import { runExtract } from './commands/extract.ts';
import { runCompile } from './commands/compile.ts';

const [, , command, ...rest] = process.argv;

async function main() {
  switch (command) {
    case 'extract':
      await runExtract(rest);
      return;
    case 'compile':
      await runCompile(rest);
      return;
    case undefined:
    case '--help':
    case '-h':
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
  compile    Flatten a translated .klz into runtime dictionaries

Run \`kapi-react <command> --help\` for per-command options.
`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
