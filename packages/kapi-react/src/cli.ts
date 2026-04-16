#!/usr/bin/env node

/**
 * kapi-react extract — extract translatable strings from JSX/TSX source files.
 *
 * Usage:
 *   npx kapi-react extract [--src <glob>] [--out <path>] [--config <path>]
 *
 * Output: JSON file with { sourceLocale, strings: [...] }
 */

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, relative } from 'node:path';
import { glob } from 'node:fs/promises';
import { extractStrings, type ExtractedString } from './extract.ts';

const args = process.argv.slice(2);
const command = args[0];

if (command !== 'extract') {
  console.log(`
kapi-react — zero-config i18n for React

Commands:
  extract    Extract translatable strings from JSX/TSX source files

Usage:
  npx kapi-react extract [options]

Options:
  --src <glob>     Source files to scan (default: "src/**/*.{tsx,jsx}")
  --out <path>     Output file (default: "i18n/strings.json")
  --config <path>  Config file with componentMap, rules, etc.
`);
  process.exit(command === '--help' || command === '-h' ? 0 : 1);
}

// Parse CLI arguments
let srcGlob = 'src/**/*.{tsx,jsx}';
let outFile = 'i18n/strings.json';
let configPath: string | null = null;

for (let i = 1; i < args.length; i++) {
  if (args[i] === '--src' && args[i + 1]) { srcGlob = args[++i]; }
  else if (args[i] === '--out' && args[i + 1]) { outFile = args[++i]; }
  else if (args[i] === '--config' && args[i + 1]) { configPath = args[++i]; }
}

// Load config
let config: { componentMap?: Record<string, string>; rules?: any[] } = {};
if (configPath) {
  try {
    config = JSON.parse(readFileSync(configPath, 'utf-8'));
  } catch (e) {
    console.error(`Failed to load config from ${configPath}:`, e);
    process.exit(1);
  }
}

// Collect files
async function run() {
  const files: string[] = [];
  for await (const file of glob(srcGlob)) {
    files.push(file);
  }

  if (files.length === 0) {
    console.warn(`No files found matching "${srcGlob}"`);
    process.exit(0);
  }

  console.log(`Scanning ${files.length} files...`);

  // Extract strings from each file
  const allStrings: ExtractedString[] = [];
  const seen = new Set<string>();

  for (const file of files) {
    const code = readFileSync(file, 'utf-8');
    const relPath = relative(process.cwd(), file);
    const strings = extractStrings(code, relPath, config);

    for (const s of strings) {
      if (!seen.has(s.hash)) {
        seen.add(s.hash);
        allStrings.push(s);
      }
    }
  }

  // Write output
  mkdirSync(dirname(outFile), { recursive: true });
  const output = {
    sourceLocale: 'en',
    strings: allStrings,
  };
  writeFileSync(outFile, JSON.stringify(output, null, 2) + '\n');

  console.log(`Extracted ${allStrings.length} strings → ${outFile}`);
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
