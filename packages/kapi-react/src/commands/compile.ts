/**
 * kapi-react compile — flatten a translated .klz into runtime dictionaries.
 *
 * Given a `.klz` that kapi (or another tool) has filled in with target
 * runs per locale, produces one `<locale>.json` dictionary per locale
 * in the `{hash: renderedText}` shape the kapi-react runtime fetches
 * via `loadTranslations()` and consumes via `t()` / `tx()`.
 *
 * Input: a translated .klz (source blocks + block.targets populated).
 * Output: one JSON file per target locale with
 *         `{ "<block.hash>": "<flattened target>" }`.
 */

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';

import { flattenRuns } from '@neokapi/kapi-format';
import { KlzReader } from '@neokapi/kapi-format/klz';

export async function runCompile(args: string[]) {
  let input: string | null = null;
  const locales: string[] = [];
  let outDir = 'public/translations';

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    const value = args[i + 1];
    if (flag === '--help' || flag === '-h') {
      console.log(usage);
      return;
    }
    if (flag === '--out' && value) outDir = args[++i];
    else if (flag === '--locale' && value) locales.push(args[++i]);
    else if (!flag.startsWith('--')) input = flag;
  }

  if (!input) {
    console.error('error: missing input .klz path\n');
    console.log(usage);
    process.exit(1);
  }

  const archive = readFileSync(input);
  const reader = new KlzReader(new Uint8Array(archive));

  // If no --locale flags provided, derive the set from targets seen
  // on every block plus the manifest's declared target locales.
  const targetLocales = new Set<string>(locales);
  if (targetLocales.size === 0) {
    for (const l of reader.manifest.project.targetLocales ?? []) {
      targetLocales.add(l);
    }
    for (const { block } of reader.blocks()) {
      for (const l of Object.keys(block.targets ?? {})) targetLocales.add(l);
    }
  }

  if (targetLocales.size === 0) {
    console.error('error: archive has no target locales; pass --locale explicitly');
    process.exit(1);
  }

  mkdirSync(outDir, { recursive: true });

  let totalCompiled = 0;
  for (const locale of targetLocales) {
    const dict: Record<string, string> = {};
    for (const { block } of reader.blocks()) {
      const runs = block.targets?.[locale];
      if (!runs || runs.length === 0) continue;
      dict[block.hash] = flattenRuns(runs);
    }
    const outPath = join(outDir, `${locale}.json`);
    mkdirSync(dirname(outPath), { recursive: true });
    writeFileSync(outPath, `${JSON.stringify(dict, null, 2)}\n`);
    console.log(`Compiled ${Object.keys(dict).length} entries → ${outPath}`);
    totalCompiled += Object.keys(dict).length;
  }

  if (totalCompiled === 0) {
    console.warn('warning: no translated blocks found for any target locale');
  }
}

const usage = `
kapi-react compile — flatten a translated .klz into runtime dictionaries.

Usage:
  kapi-react compile <input.klz> [--locale <lang>]... [--out <dir>]

Options:
  --locale <lang>   Emit a dictionary for this locale (repeat for multiple).
                    If omitted, every locale present in block.targets or the
                    manifest's targetLocales is emitted.
  --out <dir>       Output directory (default: public/translations)
`;
