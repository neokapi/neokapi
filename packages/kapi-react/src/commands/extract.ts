/**
 * kapi-react extract — walk JSX/TSX source and write i18n/strings.json.
 *
 * Still emits the flat ExtractedString[] shape. Rewriting to produce
 * structured Runs + .klz is the next major step (tracked in the issue
 * referenced from AD-045).
 */

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, relative } from 'node:path';
import { glob } from 'node:fs/promises';
import { extractStrings, type ExtractedString } from '../extract.ts';

interface Config {
  componentMap?: Record<string, string>;
  // The config shape is still under negotiation in the extractor.
  // Everything else passes through untyped for now.
  [key: string]: unknown;
}

export async function runExtract(args: string[]) {
  let srcGlob = 'src/**/*.{tsx,jsx}';
  let outFile = 'i18n/strings.json';
  let configPath: string | null = null;

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    const value = args[i + 1];
    if (flag === '--help' || flag === '-h') {
      console.log(`
kapi-react extract — scan JSX/TSX files for translatable strings.

Options:
  --src <glob>     Source files to scan (default: "src/**/*.{tsx,jsx}")
  --out <path>     Output file (default: "i18n/strings.json")
  --config <path>  Config file with componentMap, rules, etc.
`);
      return;
    }
    if (flag === '--src' && value) srcGlob = args[++i];
    else if (flag === '--out' && value) outFile = args[++i];
    else if (flag === '--config' && value) configPath = args[++i];
  }

  let config: Config = {};
  if (configPath) {
    try {
      config = JSON.parse(readFileSync(configPath, 'utf-8')) as Config;
    } catch (e) {
      console.error(`Failed to load config from ${configPath}:`, e);
      process.exit(1);
    }
  }

  const files: string[] = [];
  for await (const file of glob(srcGlob)) files.push(file);

  if (files.length === 0) {
    console.warn(`No files found matching "${srcGlob}"`);
    return;
  }

  console.log(`Scanning ${files.length} files...`);

  const seen = new Set<string>();
  const allStrings: ExtractedString[] = [];
  for (const file of files) {
    const code = readFileSync(file, 'utf-8');
    const relPath = relative(process.cwd(), file);
    for (const s of extractStrings(code, relPath, config)) {
      if (!seen.has(s.hash)) {
        seen.add(s.hash);
        allStrings.push(s);
      }
    }
  }

  mkdirSync(dirname(outFile), { recursive: true });
  writeFileSync(
    outFile,
    `${JSON.stringify({ sourceLocale: 'en', strings: allStrings }, null, 2)}\n`,
  );
  console.log(`Extracted ${allStrings.length} strings → ${outFile}`);
}
