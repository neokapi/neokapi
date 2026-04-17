/**
 * kapi-react extract — walk every matched JSX/TSX file and write a
 * single `.klz` archive carrying one `Document` per source file.
 *
 * The archive is what `kapi` (pseudo-translate, ai-translate, QA, …)
 * consumes for further processing and what `kapi-react compile` reads
 * back to produce the per-locale runtime dictionaries.
 */

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, relative } from 'node:path';
import { glob } from 'node:fs/promises';

import type { Document, File } from '@neokapi/kapi-format';
import { newFile } from '@neokapi/kapi-format';
import { KlzWriter } from '@neokapi/kapi-format/klz';

import {
  createWarningCollector,
  extractDocument,
  formatWarning,
} from '../extract/index.ts';
import type { PluginOptions } from '../types.ts';

type ExtractConfig = Pick<PluginOptions, 'componentMap' | 'rules'>;

export async function runExtract(args: string[]): Promise<void> {
  const opts = parseArgs(args);
  if (opts.help) {
    console.log(usage);
    return;
  }

  const config = loadConfig(opts.configPath);

  const files: string[] = [];
  for await (const file of glob(opts.srcGlob)) files.push(file);
  files.sort();

  if (files.length === 0) {
    console.warn(`No files found matching "${opts.srcGlob}"`);
    return;
  }

  console.log(`Scanning ${files.length} files...`);

  const documents = extractAllDocuments(files, config);
  if (documents.length === 0) {
    console.warn('No translatable content found.');
    return;
  }

  const archive = buildArchive(documents, opts);
  mkdirSync(dirname(opts.outFile), { recursive: true });
  writeFileSync(opts.outFile, archive);

  const blockCount = documents.reduce((n, d) => n + d.blocks.length, 0);
  console.log(`Extracted ${blockCount} blocks from ${documents.length} files → ${opts.outFile}`);
}

// ─── Internals ────────────────────────────────────────────────────

interface ExtractArgs {
  srcGlob: string;
  outFile: string;
  configPath: string | null;
  projectId: string;
  sourceLocale: string;
  targetLocales: string[];
  help: boolean;
}

function parseArgs(args: string[]): ExtractArgs {
  const parsed: ExtractArgs = {
    srcGlob: 'src/**/*.{tsx,jsx}',
    outFile: 'i18n/extracted.klz',
    configPath: null,
    projectId: 'app',
    sourceLocale: 'en',
    targetLocales: [],
    help: false,
  };

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    const value = args[i + 1];
    switch (flag) {
      case '--help':
      case '-h':
        parsed.help = true;
        return parsed;
      case '--src':
        if (value) parsed.srcGlob = args[++i];
        break;
      case '--out':
        if (value) parsed.outFile = args[++i];
        break;
      case '--config':
        if (value) parsed.configPath = args[++i];
        break;
      case '--project':
        if (value) parsed.projectId = args[++i];
        break;
      case '--source-locale':
        if (value) parsed.sourceLocale = args[++i];
        break;
      case '--target-locale':
        if (value) parsed.targetLocales.push(args[++i]);
        break;
      default:
        console.warn(`unknown flag: ${flag}`);
    }
  }

  return parsed;
}

function loadConfig(path: string | null): ExtractConfig {
  if (!path) return {};
  try {
    return JSON.parse(readFileSync(path, 'utf-8')) as ExtractConfig;
  } catch (e) {
    console.error(`Failed to load config from ${path}:`, e);
    process.exit(1);
  }
}

function extractAllDocuments(files: readonly string[], config: ExtractConfig): Document[] {
  const out: Document[] = [];
  const warnings = createWarningCollector();
  for (const file of files) {
    const code = readFileSync(file, 'utf-8');
    const filename = relative(process.cwd(), file);
    const doc = extractDocument(code, { filename, warnings, ...config });
    if (doc) out.push(doc);
  }
  for (const w of warnings.list()) {
    console.warn(formatWarning(w));
  }
  return out;
}

function buildArchive(documents: readonly Document[], opts: ExtractArgs): Uint8Array {
  const file: File = newFile({
    generator: { id: '@neokapi/kapi-react', version: readPackageVersion() },
    project: {
      id: opts.projectId,
      sourceLocale: opts.sourceLocale,
    },
    documents: [...documents],
  });

  const writer = new KlzWriter({
    generator: file.generator,
    project: {
      id: opts.projectId,
      sourceLocale: opts.sourceLocale,
      ...(opts.targetLocales.length > 0 ? { targetLocales: opts.targetLocales } : {}),
    },
    created: new Date().toISOString(),
  });

  // One `documents/<slug>.klf` per source file so consumers can stream
  // per-document without re-decoding the whole archive.
  for (const doc of documents) {
    const slug = slugify(doc.path);
    writer.addDocument(`documents/${slug}.klf`, {
      ...file,
      documents: [doc],
    });
  }
  return writer.build();
}

function slugify(path: string): string {
  return path.replace(/[^\w./-]+/g, '_').replace(/\//g, '-');
}

function readPackageVersion(): string {
  try {
    const url = new URL('../../package.json', import.meta.url);
    const pkg = JSON.parse(readFileSync(url, 'utf-8')) as { version?: string };
    return pkg.version ?? '0.0.0';
  } catch {
    return '0.0.0';
  }
}

const usage = `
kapi-react extract — scan JSX/TSX files and write a .klz archive.

Usage:
  kapi-react extract [options]

Options:
  --src <glob>            Source files to scan (default: "src/**/*.{tsx,jsx}")
  --out <path>            Output archive path (default: "i18n/extracted.klz")
  --config <path>         Config file with componentMap, rules, …
  --project <id>          Project id stamped into manifest.project (default: "app")
  --source-locale <bcp>   Manifest source locale (default: "en")
  --target-locale <bcp>   Declared target locale (repeatable, informational)
`;
