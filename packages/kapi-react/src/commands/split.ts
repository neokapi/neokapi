/**
 * kapi-react split — slice per-locale catalogs along chunk lines.
 *
 * Takes a `translations-manifest.json` (emitted by the Vite/Rollup
 * plugin at build time) plus a directory of master `{locale}.json`
 * dicts (from `kapi-react compile`), and emits one subset per
 * (locale, chunk) pair:
 *
 *     dist/translations/
 *     ├── manifest.json             ← copy of the input manifest
 *     └── {locale}/
 *         ├── index.json            ← hashes used by the index chunk
 *         ├── SettingsPage.json
 *         └── FlowEditor.json
 *
 * Hashes used by multiple chunks are duplicated across the subset
 * files — keeps each chunk independently loadable. The total on-wire
 * cost after gzip is close to the original master dict.
 *
 * See issue #406.
 */

import { readFileSync, writeFileSync, mkdirSync, readdirSync, existsSync } from "node:fs";
import { join, basename, extname } from "node:path";

import type { TranslationsManifest } from "../plugin/chunk-manifest.ts";

const usage = `
Usage: kapi-react split [options]

Options:
  --manifest <path>   Path to translations-manifest.json emitted by
                      the plugin's generateBundle hook (required).
  --locales <dir>     Directory containing master {locale}.json dicts
                      produced by \`kapi-react compile\` (required).
  --out <dir>         Output directory. Per-locale subfolders are
                      created under it (default: dist/translations).
  --help, -h          Show this help message.

Example:
  kapi-react split \\
    --manifest dist/translations-manifest.json \\
    --locales public/translations \\
    --out dist/translations
`;

interface SplitArgs {
  manifestPath: string;
  localesDir: string;
  outDir: string;
}

export async function runSplit(args: string[]) {
  const parsed = parseArgs(args);
  if (!parsed) return;

  const manifest = loadManifest(parsed.manifestPath);
  const locales = discoverLocales(parsed.localesDir);
  if (locales.length === 0) {
    console.error(`error: no {locale}.json files found in ${parsed.localesDir}`);
    process.exit(1);
  }

  mkdirSync(parsed.outDir, { recursive: true });
  // Echo the manifest into the output tree so consumers have a single
  // well-known path for both the chunk map and the chunk dicts.
  writeFileSync(join(parsed.outDir, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);

  let totalChunks = 0;
  for (const locale of locales) {
    const dict = loadLocaleDict(parsed.localesDir, locale);
    const localeOutDir = join(parsed.outDir, locale);
    mkdirSync(localeOutDir, { recursive: true });

    for (const [chunkName, hashes] of Object.entries(manifest.chunks)) {
      const subset: Record<string, string> = {};
      for (const h of hashes) {
        const translated = dict[h];
        if (translated !== undefined) subset[h] = translated;
      }
      const outPath = join(localeOutDir, `${chunkName}.json`);
      writeFileSync(outPath, `${JSON.stringify(subset, null, 2)}\n`);
      totalChunks++;
    }
  }

  console.log(
    `split: ${locales.length} locales × ${Object.keys(manifest.chunks).length} chunks ` +
      `= ${totalChunks} files → ${parsed.outDir}`,
  );
}

function parseArgs(args: string[]): SplitArgs | null {
  let manifestPath = "";
  let localesDir = "";
  let outDir = "dist/translations";

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    if (flag === "--help" || flag === "-h") {
      console.log(usage);
      return null;
    }
    if (flag === "--manifest") manifestPath = args[++i] ?? "";
    else if (flag === "--locales") localesDir = args[++i] ?? "";
    else if (flag === "--out") outDir = args[++i] ?? outDir;
    else {
      console.error(`unknown argument: ${flag}`);
      console.log(usage);
      process.exit(1);
    }
  }

  if (!manifestPath) {
    console.error("error: --manifest is required\n");
    console.log(usage);
    process.exit(1);
  }
  if (!localesDir) {
    console.error("error: --locales is required\n");
    console.log(usage);
    process.exit(1);
  }

  return { manifestPath, localesDir, outDir };
}

function loadManifest(path: string): TranslationsManifest {
  if (!existsSync(path)) {
    console.error(`error: manifest not found: ${path}`);
    process.exit(1);
  }
  const parsed = JSON.parse(readFileSync(path, "utf8")) as TranslationsManifest;
  if (parsed.version !== 1) {
    console.error(`error: unsupported manifest version ${parsed.version} (this CLI supports v1)`);
    process.exit(1);
  }
  if (!parsed.chunks || typeof parsed.chunks !== "object") {
    console.error("error: manifest has no `chunks` object");
    process.exit(1);
  }
  return parsed;
}

function discoverLocales(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir)
    .filter((f) => extname(f) === ".json")
    .map((f) => basename(f, ".json"))
    .sort();
}

function loadLocaleDict(dir: string, locale: string): Record<string, string> {
  const path = join(dir, `${locale}.json`);
  return JSON.parse(readFileSync(path, "utf8")) as Record<string, string>;
}
